package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRecordsRouteTemplateAndReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)
	metrics.SetReady(true)

	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusCreated) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/items/secret-value", nil))

	metricResponse := httptest.NewRecorder()
	Handler(registry).ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricResponse.Body.String()
	for _, expected := range []string{
		`beon_bin_api_http_requests_total{method="GET",route="/items/:id",status="201"} 1`,
		`beon_bin_api_http_request_duration_seconds_count{method="GET",route="/items/:id"} 1`,
		`beon_bin_api_ready 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "secret-value") {
		t.Fatalf("raw route value leaked into metrics: %s", body)
	}

	metrics.SetReady(false)
	metricResponse = httptest.NewRecorder()
	Handler(registry).ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricResponse.Body.String(), `beon_bin_api_ready 0`) {
		t.Fatalf("readiness gauge did not become zero: %s", metricResponse.Body.String())
	}
}

func TestMetricsUsesBoundedUnmatchedRouteLabel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)
	router := gin.New()
	router.Use(metrics.Middleware())

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/not-found/123", nil))
	metricResponse := httptest.NewRecorder()
	Handler(registry).ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricResponse.Body.String(), `route="unmatched"`) {
		t.Fatalf("unmatched route label missing: %s", metricResponse.Body.String())
	}
}
