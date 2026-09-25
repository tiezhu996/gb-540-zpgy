package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerLandParcelRoutes(api *gin.RouterGroup, deps Dependencies) {
	h := deps.CadastralHandler
	parcels := api.Group("/parcels")
	parcels.GET("", h.ListParcels)
	parcels.GET("/:id", h.GetParcel)
	parcels.POST("", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.CreateParcel)
	parcels.PATCH("/:id", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.UpdateParcel)
}
