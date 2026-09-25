package handler

import (
	"net/http"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"github.com/gin-gonic/gin"
)

func (h *CadastralHandler) ListObservations(c *gin.Context) {
	items, meta, err := h.service.ListObservations(dto.ObservationQuery{ParcelID: queryUint(c, "parcel_id"), State: c.Query("state"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 50)})
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) ImportObservation(c *gin.Context) {
	var req dto.ImportObservationRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.ImportObservation(req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, item, nil)
}

func (h *CadastralHandler) GetObservation(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := h.service.GetObservation(id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}

func (h *CadastralHandler) TransitionObservation(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.ObservationTransitionRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.TransitionObservation(id, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}
