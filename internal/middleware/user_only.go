package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/consts"
)

func RequireUserToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get(consts.IsService)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		isService, ok := value.(bool)
		if !ok || isService {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
