package router

import (
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerBoundaryProposalRoutes(api *gin.RouterGroup, deps Dependencies) {
	h := deps.CadastralHandler
	proposals := api.Group("/proposals")
	proposals.GET("", h.ListProposals)
	proposals.GET("/:id", h.GetProposal)
	proposals.POST("", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleAdmin), h.CreateProposal)
	proposals.POST("/:id/transition", appmw.RBACMiddleware(constants.RoleSurveyor, constants.RoleGISAnalyst, constants.RoleReviewer, constants.RoleAdmin), h.TransitionProposal)
}
