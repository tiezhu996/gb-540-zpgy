package middleware

import (
	"log/slog"
	"net/http"

	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func RecoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic_recovered", "request_id", c.GetString("request_id"), "panic", recovered)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": service.CodeInternal, "message": "unexpected server error", "request_id": c.GetString("request_id")}})
	})
}
