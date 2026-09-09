package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAccessLoggerRedactsSensitiveRequestData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const pan = "4111111111111111"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := gin.New()
	router.Use(RequestID(), AccessLogger(logger))
	router.GET("/api/v1/bin/:number", func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/bin/"+pan+"?api_key=secret", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	logged := output.String()
	for _, sensitive := range []string{pan, "api_key", "secret"} {
		if strings.Contains(logged, sensitive) {
			t.Fatalf("access log leaked %q: %s", sensitive, logged)
		}
	}
	if !strings.Contains(logged, "/api/v1/bin/[REDACTED]") {
		t.Fatalf("redacted path missing from log: %s", logged)
	}
}

func TestAccessLoggerKeepsDiagnosticBIN(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := gin.New()
	router.Use(RequestID(), AccessLogger(logger))
	router.GET("/api/v1/bin/:number", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/api/v1/bin/411111", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if !strings.Contains(output.String(), "/api/v1/bin/411111") {
		t.Fatalf("valid BIN missing from access log: %s", output.String())
	}
}
