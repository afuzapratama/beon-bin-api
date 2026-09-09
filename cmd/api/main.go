package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/beon/bin-api/internal/database"
	"github.com/beon/bin-api/internal/handler"
	"github.com/beon/bin-api/internal/middleware"
	"github.com/beon/bin-api/internal/observability"
	"github.com/beon/bin-api/internal/repository/postgres"
	"github.com/beon/bin-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func main() {
	_ = godotenv.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := sqlx.Open("postgres", cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.dbMaxOpenConns)
	db.SetMaxIdleConns(cfg.dbMaxIdleConns)
	db.SetConnMaxLifetime(cfg.dbConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.dbConnMaxIdleTime)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	if err := database.Migrate(startupCtx, db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	metrics := observability.NewMetrics(registry)
	metrics.SetReady(true)

	repo := postgres.New(db)
	svc := service.New(repo, service.EnrichmentConfig{
		Enabled:           cfg.enrichmentEnabled,
		HandyAPIKey:       cfg.handyAPIKey,
		HandyMonthlyLimit: int64(cfg.handyMonthlyLimit),
	})
	defer svc.Close()
	binHandler := handler.NewBINHandler(svc)
	healthHandler := handler.NewHealthHandler(db, metrics)
	router, err := newRouter(logger, cfg, metrics, registry, binHandler, healthHandler)
	if err != nil {
		return err
	}

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.port),
		Handler:           router,
		ReadHeaderTimeout: cfg.readHeaderTimeout,
		ReadTimeout:       cfg.readTimeout,
		WriteTimeout:      cfg.writeTimeout,
		IdleTimeout:       cfg.idleTimeout,
		MaxHeaderBytes:    cfg.maxHeaderBytes,
		BaseContext: func(net.Listener) context.Context {
			return appCtx
		},
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("API listening", "address", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-appCtx.Done():
		metrics.SetReady(false)
		logger.Info("shutdown started", "timeout", cfg.shutdownTimeout.String())
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr := server.Close()
			return errors.Join(fmt.Errorf("graceful shutdown: %w", err), closeErr)
		}
		logger.Info("shutdown complete")
		return nil
	}
}

func newRouter(
	logger *slog.Logger,
	cfg appConfig,
	metrics *observability.Metrics,
	registry prometheus.Gatherer,
	binHandler *handler.BINHandler,
	healthHandler *handler.HealthHandler,
) (*gin.Engine, error) {
	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.AccessLogger(logger),
		metrics.Middleware(),
		gin.Recovery(),
	)
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Use(corsMiddleware(cfg.corsOrigins))
	router.GET("/metrics", gin.WrapH(observability.Handler(registry)))

	api := router.Group("/api/v1")
	api.GET("/live", healthHandler.Live)
	api.GET("/ready", healthHandler.Ready)
	api.GET("/health", healthHandler.Live) // Backward-compatible alias.

	publicLimiter := middleware.NewRateLimiter(middleware.RateLimitConfig{
		GlobalRPS:      cfg.globalRateLimitRPS,
		GlobalBurst:    cfg.globalRateLimitBurst,
		PerClientRPS:   cfg.rateLimitRPS,
		PerClientBurst: cfg.rateLimitBurst,
		MaxClients:     cfg.rateLimitMaxClients,
		ClientTTL:      10 * time.Minute,
	})
	limited := api.Group("")
	limited.Use(publicLimiter.Middleware())
	limited.GET("/stats", binHandler.Stats)
	limited.GET("/bin/:number", binHandler.Lookup)
	return router, nil
}

func corsMiddleware(allowOrigins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false
		if allowOrigins == "*" {
			allowed = true
			c.Header("Access-Control-Allow-Origin", "*")
		} else if allowOrigins != "" {
			c.Header("Vary", "Origin")
			for _, candidate := range strings.Split(allowOrigins, ",") {
				if strings.TrimSpace(candidate) == origin {
					allowed = true
					break
				}
			}
		}
		if allowed && allowOrigins != "*" {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID, Retry-After")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
