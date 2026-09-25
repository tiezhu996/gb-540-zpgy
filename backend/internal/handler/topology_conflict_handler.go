package handler

import (
	"net/http"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"github.com/gin-gonic/gin"
)

func (h *CadastralHandler) ListConflicts(c *gin.Context) {
	items, meta, err := h.service.ListConflicts(dto.ConflictQuery{ProposalID: queryUint(c, "proposal_id"), State: c.Query("state"), Type: c.Query("conflict_type"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 50)})
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) GetConflict(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := h.service.GetConflict(id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}

func (h *CadastralHandler) DetectConflicts(c *gin.Context) {
	var req dto.DetectConflictRequest
	if !bind(c, h.validate, &req) {
		return
	}
	items, err := h.service.DetectConflicts(req, c.GetHeader("Idempotency-Key"), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, nil)
}

func (h *CadastralHandler) TransitionConflict(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.ConflictTransitionRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.TransitionConflict(id, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}

func (h *CadastralHandler) ApplyConflictSuggestion(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.ApplySuggestionRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.ApplyConflictSuggestion(id, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, item, nil)
}
