package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type appConfig struct {
	databaseURL string
	port        int
	corsOrigins string

	enrichmentEnabled bool
	handyAPIKey       string
	handyMonthlyLimit int

	rateLimitRPS         float64
	rateLimitBurst       int
	globalRateLimitRPS   float64
	globalRateLimitBurst int
	rateLimitMaxClients  int

	dbMaxOpenConns    int
	dbMaxIdleConns    int
	dbConnMaxLifetime time.Duration
	dbConnMaxIdleTime time.Duration

	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	shutdownTimeout   time.Duration
	maxHeaderBytes    int
}

func loadConfig() (appConfig, error) {
	var cfg appConfig
	cfg.databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if cfg.databaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	cfg.corsOrigins = os.Getenv("CORS_ORIGINS")
	cfg.handyAPIKey = strings.TrimSpace(os.Getenv("HANDY_API_KEY"))

	var err error
	if cfg.port, err = intEnv("PORT", 8080); err != nil || cfg.port > 65_535 {
		return cfg, errors.New("PORT must be between 1 and 65535")
	}
	if cfg.enrichmentEnabled, err = boolEnv("ENRICHMENT_ENABLED", false); err != nil {
		return cfg, err
	}
	if cfg.handyMonthlyLimit, err = intEnv("HANDY_API_MONTHLY_LIMIT", 3_000); err != nil {
		return cfg, err
	}
	if cfg.rateLimitRPS, err = floatEnv("RATE_LIMIT_RPS", 5); err != nil {
		return cfg, err
	}
	if cfg.rateLimitBurst, err = intEnv("RATE_LIMIT_BURST", 10); err != nil {
		return cfg, err
	}
	if cfg.globalRateLimitRPS, err = floatEnv("GLOBAL_RATE_LIMIT_RPS", 100); err != nil {
		return cfg, err
	}
	if cfg.globalRateLimitBurst, err = intEnv("GLOBAL_RATE_LIMIT_BURST", 200); err != nil {
		return cfg, err
	}
	if cfg.rateLimitMaxClients, err = intEnv("RATE_LIMIT_MAX_CLIENTS", 10_000); err != nil {
		return cfg, err
	}

	if cfg.dbMaxOpenConns, err = intEnv("DB_MAX_OPEN_CONNS", 20); err != nil {
		return cfg, err
	}
	if cfg.dbMaxIdleConns, err = intEnv("DB_MAX_IDLE_CONNS", 10); err != nil {
		return cfg, err
	}
	if cfg.dbMaxIdleConns > cfg.dbMaxOpenConns {
		return cfg, errors.New("DB_MAX_IDLE_CONNS must not exceed DB_MAX_OPEN_CONNS")
	}
	if cfg.dbConnMaxLifetime, err = durationEnv("DB_CONN_MAX_LIFETIME", 30*time.Minute); err != nil {
		return cfg, err
	}
	if cfg.dbConnMaxIdleTime, err = durationEnv("DB_CONN_MAX_IDLE_TIME", 5*time.Minute); err != nil {
		return cfg, err
	}

	if cfg.readHeaderTimeout, err = durationEnv("HTTP_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		return cfg, err
	}
	if cfg.readTimeout, err = durationEnv("HTTP_READ_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	if cfg.writeTimeout, err = durationEnv("HTTP_WRITE_TIMEOUT", 30*time.Second); err != nil {
		return cfg, err
	}
	if cfg.idleTimeout, err = durationEnv("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return cfg, err
	}
	if cfg.shutdownTimeout, err = durationEnv("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return cfg, err
	}
	if cfg.maxHeaderBytes, err = intEnv("HTTP_MAX_HEADER_BYTES", 1<<20); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}

func intEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func floatEnv(name string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s must be a positive number", name)
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return parsed, nil
}
