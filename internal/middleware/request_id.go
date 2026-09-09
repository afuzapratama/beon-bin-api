package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/beon/bin-api/internal/requestcontext"
	"github.com/gin-gonic/gin"
)

const (
	requestIDHeader = "X-Request-ID"
	requestIDGinKey = "request_id"
)

var (
	validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
	fallbackID     atomic.Uint64
)

// RequestID validates a caller-provided request ID or creates a new one, then
// makes it available in the response, Gin context, and request context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Set(requestIDGinKey, id)
		c.Header(requestIDHeader, id)
		ctx := requestcontext.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequestIDFromContext returns the request ID propagated by the middleware.
func RequestIDFromContext(ctx context.Context) string {
	return requestcontext.RequestID(ctx)
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("fallback-%x-%x", time.Now().UnixNano(), fallbackID.Add(1))
}
