package middleware

import (
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func HubFromContext(c *gin.Context) *sentry.Hub {
	if hub, ok := c.Get("sentry_hub"); ok {
		if h, ok := hub.(*sentry.Hub); ok {
			return h
		}
	}
	return sentry.CurrentHub()
}
