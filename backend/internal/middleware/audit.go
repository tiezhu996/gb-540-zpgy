package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditContext is request-scoped metadata shared by structured request logs
// and service-level immutable audit events.
type AuditContext struct {
	RequestID string
	Method    string
	Path      string
	StartedAt time.Time
}

func AuditContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("audit_context", AuditContext{RequestID: c.GetString("request_id"), Method: c.Request.Method, Path: c.Request.URL.Path, StartedAt: time.Now().UTC()})
		c.Next()
	}
}

func AccessLogMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http_request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "bytes", c.Writer.Size(), "latency_ms", time.Since(started).Milliseconds(), "client_ip", c.ClientIP(), "request_id", c.GetString("request_id"))
	}
}
