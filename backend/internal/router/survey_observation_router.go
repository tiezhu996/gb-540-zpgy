package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerSurveyObservationRoutes(api *gin.RouterGroup, deps Dependencies) {
	h := deps.CadastralHandler
	observations := api.Group("/observations")
	observations.GET("", h.ListObservations)
	observations.GET("/:id", h.GetObservation)
	observations.POST("/import", appmw.RateLimitMiddleware(deps.ImportLimiter, "observation_import"), appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.ImportObservation)
	observations.POST("/:id/transition", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.TransitionObservation)
}
