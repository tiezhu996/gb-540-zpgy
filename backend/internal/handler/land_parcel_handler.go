package handler

import (
	"net/http"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// CadastralHandler supplies request binding shared by entity-specific handlers.
type CadastralHandler struct {
	service  *service.CadastralService
	validate *validator.Validate
}

func NewCadastralHandler(cadastralService *service.CadastralService, validate *validator.Validate) *CadastralHandler {
	return &CadastralHandler{service: cadastralService, validate: validate}
}

func (h *CadastralHandler) ListParcels(c *gin.Context) {
	items, meta, err := h.service.ListParcels(dto.ParcelQuery{Keyword: c.Query("keyword"), State: c.Query("state"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 20)})
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, meta)
}

func (h *CadastralHandler) CreateParcel(c *gin.Context) {
	var req dto.CreateParcelRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.CreateParcel(req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, item, nil)
}

func (h *CadastralHandler) GetParcel(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	item, err := h.service.GetParcel(id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}

func (h *CadastralHandler) UpdateParcel(c *gin.Context) {
	id, valid := idParam(c)
	if !valid {
		return
	}
	var req dto.UpdateParcelRequest
	if !bind(c, h.validate, &req) {
		return
	}
	item, err := h.service.UpdateParcel(id, req, actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, item, nil)
}
