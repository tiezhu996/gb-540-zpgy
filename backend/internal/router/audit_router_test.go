package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/config"
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/handler"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newAuditTestRouter(t *testing.T) (*gin.Engine, *service.CadastralService, *service.AuthService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := repository.MigrateAndSeed(db); err != nil {
		t.Fatalf("migrate and seed SQLite: %v", err)
	}
	store := repository.NewStore(db)
	config := config.Config{JWTSecret: "audit-router-test-secret-must-be-at-least-32-bytes", AccessTokenTTL: time.Hour}
	authService := service.NewAuthService(store, config)
	cadastralService := service.NewCadastralService(store)
	auditService := service.NewAuditService(store)
	validate := validator.New(validator.WithRequiredStructEnabled())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Register(engine, Dependencies{
		AuthHandler:      handler.NewAuthHandler(authService, validate),
		AuditHandler:     handler.NewAuditHandler(auditService),
		AuthService:      authService,
		CadastralHandler: handler.NewCadastralHandler(cadastralService, validate),
		LoginLimiter:     appmw.NewRateLimiter(100, time.Minute),
		ImportLimiter:    appmw.NewRateLimiter(100, time.Minute),
		AnalyzeLimiter:   appmw.NewRateLimiter(100, time.Minute),
	})
	return engine, cadastralService, authService
}

func TestAuditEndpointAllowsAuditorAndFiltersEntity(t *testing.T) {
	engine, cadastralService, authService := newAuditTestRouter(t)
	requestID := "audit-router-entity-filter"
	_, err := cadastralService.CreateParcel(dto.CreateParcelRequest{
		ParcelCode:       "P-AUDIT",
		Name:             "audit route fixture",
		BoundaryGeoJSON:  `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`,
		CoordinateSystem: "EPSG:3857",
	}, service.Actor{ID: 701, Username: "surveyor-fixture", Role: constants.RoleSurveyor, RequestID: requestID})
	if err != nil {
		t.Fatalf("create audit fixture: %v", err)
	}
	login, err := authService.Login(dto.LoginRequest{Username: "auditor", Password: "DemoPass123!"})
	if err != nil {
		t.Fatalf("login seeded auditor: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?entity=LandParcel&request_id="+requestID, nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	res := httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/audit status = %d, body = %s", res.Code, res.Body.String())
	}
	var body struct {
		Data []model.AuditLog `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("filtered audit data = %#v, want exactly one parcel entry", body.Data)
	}
	entry := body.Data[0]
	if entry.ResourceType != "LandParcel" || entry.RequestID != requestID || entry.Action != "parcel.created" {
		t.Fatalf("audit entry = %#v, want filtered parcel creation", entry)
	}
}
