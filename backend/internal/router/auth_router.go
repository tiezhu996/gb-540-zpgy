package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/handler"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthHandler      *handler.AuthHandler
	AuditHandler     *handler.AuditHandler
	AuthService      *service.AuthService
	LoginLimiter     *appmw.RateLimiter
	ImportLimiter    *appmw.RateLimiter
	AnalyzeLimiter   *appmw.RateLimiter
	CadastralHandler *handler.CadastralHandler
}

func Register(engine *gin.Engine, deps Dependencies) {
	engine.GET("/healthz", deps.AuditHandler.Health)
	engine.GET("/readyz", deps.AuditHandler.Ready)
	api := engine.Group("/api/v1")
	api.POST("/auth/login", appmw.RateLimitMiddleware(deps.LoginLimiter, "login"), deps.AuthHandler.Login)
	protected := api.Group("")
	protected.Use(appmw.AuthMiddleware(deps.AuthService))
	registerLandParcelRoutes(protected, deps)
	registerSurveyObservationRoutes(protected, deps)
	registerBoundaryProposalRoutes(protected, deps)
	registerTopologyConflictRoutes(protected, deps)
	audit := protected.Group("/audit")
	audit.GET("", appmw.RBACMiddleware(constants.RoleAuditor, constants.RoleReviewer, constants.RoleAdmin), deps.AuditHandler.List)
}
