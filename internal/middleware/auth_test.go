package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIKeyAuthFailsClosedWithoutConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(APIKeyAuth(""))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIKeyAuthAcceptsOnlyConfiguredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(APIKeyAuth("expected-secret"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	wrong := httptest.NewRequest(http.MethodGet, "/", nil)
	wrong.Header.Set("X-API-Key", "wrong-secret")
	wrongRecorder := httptest.NewRecorder()
	router.ServeHTTP(wrongRecorder, wrong)
	if wrongRecorder.Code != http.StatusForbidden {
		t.Fatalf("wrong key status = %d, want %d", wrongRecorder.Code, http.StatusForbidden)
	}

	valid := httptest.NewRequest(http.MethodGet, "/", nil)
	valid.Header.Set("X-API-Key", "expected-secret")
	validRecorder := httptest.NewRecorder()
	router.ServeHTTP(validRecorder, valid)
	if validRecorder.Code != http.StatusNoContent {
		t.Fatalf("valid key status = %d, want %d", validRecorder.Code, http.StatusNoContent)
	}
}
