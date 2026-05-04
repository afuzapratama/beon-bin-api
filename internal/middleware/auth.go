package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth protects admin routes with a static API key from env
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Missing X-API-Key header",
				"code":    401,
			})
			return
		}
		if key != os.Getenv("ADMIN_API_KEY") {
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
