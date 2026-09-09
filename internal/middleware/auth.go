package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth protects admin routes with a static API key validated at startup.
func APIKeyAuth(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedKey == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   "Admin API is not configured",
				"code":    503,
			})
			return
		}
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Missing X-API-Key header",
				"code":    401,
			})
			return
		}
		if subtle.ConstantTimeCompare([]byte(key), []byte(expectedKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Invalid API key",
				"code":    403,
			})
			return
		}
		c.Next()
	}
}
