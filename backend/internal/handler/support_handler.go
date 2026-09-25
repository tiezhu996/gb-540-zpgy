package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type AuthHandler struct {
	service  *service.AuthService
	validate *validator.Validate
}

func NewAuthHandler(auth *service.AuthService, validate *validator.Validate) *AuthHandler {
	return &AuthHandler{auth, validate}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request dto.LoginRequest
	if !bind(c, h.validate, &request) {
		return
	}
	response, err := h.service.Login(request)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, response, nil)
}

type AuditHandler struct{ service *service.AuditService }

func NewAuditHandler(audit *service.AuditService) *AuditHandler { return &AuditHandler{audit} }

func (h *AuditHandler) List(c *gin.Context) {
	entity := c.Query("entity")
	if entity == "" {
		entity = c.Query("resource_type")
	}
	query := dto.AuditQuery{ResourceType: entity, RequestID: c.Query("request_id"), ActorName: c.Query("actor"), Action: c.Query("action"), Page: queryInt(c, "page", 1), PageSize: queryInt(c, "page_size", 50)}
	if value := c.Query("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			fail(c, &service.AppError{Code: service.CodeInvalidInput, Status: http.StatusBadRequest, Message: "from must be RFC3339", Err: err})
			return
		}
		query.From = &parsed
	}
	if value := c.Query("to"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			fail(c, &service.AppError{Code: service.CodeInvalidInput, Status: http.StatusBadRequest, Message: "to must be RFC3339", Err: err})
			return
		}
		query.To = &parsed
	}
	items, pagination, err := h.service.List(query)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, items, pagination)
}

func (h *AuditHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "cadastral-boundary-topology-resolution"})
}

func (h *AuditHandler) Ready(c *gin.Context) {
	if err := h.service.Ready(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func ok(c *gin.Context, status int, data any, meta any) {
	payload := gin.H{"data": data, "request_id": c.GetString("request_id")}
	if meta != nil {
		payload["meta"] = meta
	}
	c.JSON(status, payload)
}

func fail(c *gin.Context, err error) {
	var appErr *service.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.Status, gin.H{"error": gin.H{"code": appErr.Code, "message": appErr.Message, "request_id": c.GetString("request_id")}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": service.CodeInternal, "message": "unexpected server error", "request_id": c.GetString("request_id")}})
}

func bind(c *gin.Context, validate *validator.Validate, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		fail(c, &service.AppError{Code: service.CodeInvalidInput, Status: http.StatusBadRequest, Message: "request body is not valid JSON", Err: err})
		return false
	}
	if err := validate.Struct(target); err != nil {
		fail(c, &service.AppError{Code: service.CodeInvalidInput, Status: http.StatusBadRequest, Message: "request validation failed: " + err.Error(), Err: err})
		return false
	}
	return true
}

func idParam(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || value == 0 {
		fail(c, &service.AppError{Code: service.CodeInvalidInput, Status: http.StatusBadRequest, Message: "resource id must be a positive integer", Err: err})
		return 0, false
	}
	return uint(value), true
}

func actor(c *gin.Context) service.Actor {
	id, _ := c.Get("user_id")
	role, _ := c.Get("role")
	username, _ := c.Get("username")
	userID, _ := id.(uint)
	roleString, _ := role.(string)
	usernameString, _ := username.(string)
	return service.Actor{ID: userID, Username: usernameString, Role: roleString, RequestID: c.GetString("request_id")}
}

func queryUint(c *gin.Context, key string) *uint {
	if c.Query(key) == "" {
		return nil
	}
	value, err := strconv.ParseUint(c.Query(key), 10, 64)
	if err != nil || value == 0 {
		return nil
	}
	converted := uint(value)
	return &converted
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return value
}
