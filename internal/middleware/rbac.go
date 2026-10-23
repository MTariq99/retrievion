package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/utils"
)

func RequireRoles(roles ...enums.UserRole) gin.HandlerFunc {
	allowed := make(map[enums.UserRole]struct{}, len(roles))

	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		claims, ok := utils.GetUserClaims(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
			return
		}

		if _, ok := allowed[claims.UserRole]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden",
			})
			return
		}

		c.Next()
	}
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := utils.GetUserClaims(c); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
			return
		}

		c.Next()
	}
}
