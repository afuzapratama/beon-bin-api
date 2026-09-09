package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDAcceptsSafeValueAndPropagatesContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, RequestIDFromContext(c.Request.Context()))
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, "client-request_123")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Header().Get(requestIDHeader) != "client-request_123" || recorder.Body.String() != "client-request_123" {
		t.Fatalf("request ID was not propagated: header=%q body=%q",
			recorder.Header().Get(requestIDHeader), recorder.Body.String())
	}
}

func TestRequestIDReplacesUnsafeValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, "unsafe value with spaces")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	id := recorder.Header().Get(requestIDHeader)
	if !validRequestID.MatchString(id) || id == "unsafe value with spaces" {
		t.Fatalf("generated request ID = %q", id)
	}
}
