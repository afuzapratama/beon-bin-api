package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterLimitsIndividualClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter(RateLimitConfig{
		GlobalRPS:      1_000,
		GlobalBurst:    100,
		PerClientRPS:   0.0001,
		PerClientBurst: 1,
		MaxClients:     10,
		ClientTTL:      time.Minute,
	})
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	first := performRequest(router, "192.0.2.1:1234")
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusNoContent)
	}

	second := performRequest(router, "192.0.2.1:5678")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limited response is missing Retry-After")
	}

	otherClient := performRequest(router, "198.51.100.1:1234")
	if otherClient.Code != http.StatusNoContent {
		t.Fatalf("other client status = %d, want %d", otherClient.Code, http.StatusNoContent)
	}
}

func TestRateLimiterLimitsGlobalTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter(RateLimitConfig{
		GlobalRPS:      0.0001,
		GlobalBurst:    1,
		PerClientRPS:   1_000,
		PerClientBurst: 100,
		MaxClients:     10,
		ClientTTL:      time.Minute,
	})
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	if got := performRequest(router, "192.0.2.1:1234").Code; got != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", got, http.StatusNoContent)
	}
	if got := performRequest(router, "198.51.100.1:1234").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", got, http.StatusTooManyRequests)
	}
}

func performRequest(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
