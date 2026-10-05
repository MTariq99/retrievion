package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mtariq99/retrievion/internal/enums"
	"github.com/mtariq99/retrievion/utils"
)

// RequireRoles allows the request through only if the authenticated
// user's role is one of the given roles.
//
// Must run after the authentication middleware because it reads
// user claims from the Gin context.
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

// RequireAuth allows any authenticated user to proceed.
//
// More fine-grained authorization should be handled by the
// service/domain layer where necessary.
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
