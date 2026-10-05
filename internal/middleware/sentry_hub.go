package middleware

import (
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// HubFromContext returns the request-scoped Sentry hub set by SentryMiddleware.
// Falls back to the global hub if the middleware wasn't registered.
func HubFromContext(c *gin.Context) *sentry.Hub {
	if hub, ok := c.Get("sentry_hub"); ok {
		if h, ok := hub.(*sentry.Hub); ok {
			return h
		}
	}
	return sentry.CurrentHub()
}
