package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

func SentryInit(dsn string, env string) error {
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		TracesSampleRate: 1.0, // set to 0.1–0.2 in prod to reduce volume
	})
}

func SentryMiddleware(repanic bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		hub := sentry.CurrentHub().Clone()
		hub.Scope().SetRequest(c.Request)
		c.Set("sentry_hub", hub)

		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		defer func() {
			if err := recover(); err != nil {
				hub.RecoverWithContext(c.Request.Context(), err)
				hub.Flush(2 * time.Second)
				if repanic {
					panic(err)
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()

		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start)

		switch {
		case status >= 500:
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("method", c.Request.Method)
				scope.SetTag("path", path)
				scope.SetTag("query", query)
				scope.SetTag("ip", c.ClientIP())

				scope.SetContext("request_meta", map[string]any{
					"latency_ms": latency.Milliseconds(),
					"user_agent": c.Request.UserAgent(),
				})

				if uid, ok := c.Get("user_id"); ok {
					scope.SetUser(sentry.User{ID: fmt.Sprintf("%v", uid)})
				}

				ginErrors := c.Errors.ByType(gin.ErrorTypePrivate).String()
				if ginErrors != "" {
					scope.SetContext("gin_errors", map[string]any{
						"errors": ginErrors,
					})
				}

				hub.CaptureMessage(fmt.Sprintf("%s %s → %d", c.Request.Method, path, status))
			})

		case status >= 400:
			hub.AddBreadcrumb(&sentry.Breadcrumb{
				Type:     "http",
				Category: "request",
				Message:  fmt.Sprintf("%s %s → %d", c.Request.Method, path, status),
				Data: map[string]any{
					"status":     status,
					"latency_ms": latency.Milliseconds(),
					"ip":         c.ClientIP(),
				},
				Level:     sentry.LevelWarning,
				Timestamp: start,
			}, nil)
		}
	}
}
