package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/beon/bin-api/internal/requestcontext"
	"github.com/gin-gonic/gin"
)

type databasePinger interface {
	PingContext(context.Context) error
}

type readinessMetrics interface {
	SetReady(bool)
}

// HealthHandler provides process liveness and dependency readiness probes.
type HealthHandler struct {
	db      databasePinger
	metrics readinessMetrics
	timeout time.Duration
}

func NewHealthHandler(db databasePinger, metrics readinessMetrics) *HealthHandler {
	return &HealthHandler{db: db, metrics: metrics, timeout: 2 * time.Second}
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	if err := h.db.PingContext(ctx); err != nil {
		h.metrics.SetReady(false)
		slog.WarnContext(ctx, "readiness check failed",
			"request_id", requestcontext.RequestID(ctx),
			"dependency", "postgres",
			"error", err,
		)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"checks": gin.H{"postgres": "down"},
		})
		return
	}
	h.metrics.SetReady(true)
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": gin.H{"postgres": "up"},
	})
}
