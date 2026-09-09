package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics contains bounded-cardinality runtime signals for the API.
type Metrics struct {
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	ready        prometheus.Gauge
}

// NewMetrics registers application metrics with the supplied registry.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	metrics := &Metrics{
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "beon_bin_api",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, route template, and status code.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "beon_bin_api",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration by method and route template.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
		ready: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "beon_bin_api",
			Name:      "ready",
			Help:      "Whether the API can currently reach its required dependencies.",
		}),
	}
	registerer.MustRegister(metrics.httpRequests, metrics.httpDuration, metrics.ready)
	return metrics
}

// Middleware records HTTP traffic using Gin route templates, never raw BINs.
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.httpRequests.WithLabelValues(c.Request.Method, route, status).Inc()
		m.httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(startedAt).Seconds())
	}
}

// SetReady updates the readiness signal exported to Prometheus.
func (m *Metrics) SetReady(ready bool) {
	if ready {
		m.ready.Set(1)
		return
	}
	m.ready.Set(0)
}

// Handler exposes this registry in Prometheus text format.
func Handler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}
