package middleware

import (
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var sensitiveDigitSequence = regexp.MustCompile(`[0-9]{12,}`)

// AccessLogger logs request metadata without query strings or PAN-like digit
// sequences. Valid 6/8 digit BIN values remain useful for diagnostics.
func AccessLogger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		path := c.Request.URL.Path
		if index := strings.IndexByte(path, '?'); index >= 0 {
			path = path[:index]
		}
		path = sensitiveDigitSequence.ReplaceAllString(path, "[REDACTED]")
		errorMessage := sensitiveDigitSequence.ReplaceAllString(c.Errors.String(), "[REDACTED]")
		logger.InfoContext(c.Request.Context(), "http_request",
			slog.String("request_id", RequestIDFromContext(c.Request.Context())),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
			slog.String("error", errorMessage),
		)
	}
}
