package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cadastral-boundary-topology-resolution/backend/internal/config"
	"cadastral-boundary-topology-resolution/backend/internal/constants"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrNotFound = errors.New("record not found")

type Store struct {
	DB            *gorm.DB
	Users         *UserRepository
	Audits        *AuditRepository
	Parcels       *LandParcelRepository
	Observations  *SurveyObservationRepository
	Proposals     *BoundaryProposalRepository
	Conflicts     *TopologyConflictRepository
	DetectionRuns *TopologyDetectionRunRepository
}

func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if cfg.DBDriver == "sqlite" {
		dialector = sqlite.Open(cfg.DBDSN)
	} else {
		dialector = postgres.Open(cfg.DBDSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Warn), TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.DBDriver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain sql database: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	return db, nil
}

func NewStore(db *gorm.DB) *Store {
	return &Store{
		DB:            db,
		Users:         &UserRepository{db},
		Audits:        &AuditRepository{db},
		Parcels:       &LandParcelRepository{db},
		Observations:  &SurveyObservationRepository{db},
		Proposals:     &BoundaryProposalRepository{db},
		Conflicts:     &TopologyConflictRepository{db},
		DetectionRuns: &TopologyDetectionRunRepository{db},
	}
}

func (s *Store) Transaction(fn func(*Store) error) error {
	return s.DB.Transaction(func(tx *gorm.DB) error { return fn(NewStore(tx)) })
}

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return fmt.Errorf("obtain sql database for readiness: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database readiness ping: %w", err)
	}
	return nil
}

func MigrateAndSeed(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.LandParcel{}, &model.SurveyObservation{}, &model.BoundaryProposal{}, &model.TopologyConflict{}, &model.TopologyDetectionRun{}); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	// Existing installations may have been created before the cadastral RBAC roles
	// were introduced. Expand the PostgreSQL check constraint idempotently.
	if db.Dialector.Name() == "postgres" {
		if err := db.Exec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS user_role_allowed`).Error; err != nil {
			return fmt.Errorf("drop legacy role constraint: %w", err)
		}
		if err := db.Exec(`ALTER TABLE users ADD CONSTRAINT user_role_allowed CHECK (role IN ('surveyor','gis_analyst','reviewer','auditor','admin'))`).Error; err != nil {
			return fmt.Errorf("add role constraint: %w", err)
		}
		if err := migratePostGIS(db); err != nil {
			return err
		}
	}
	accounts := []struct{ username, display, role string }{
		{"surveyor", "测量员", constants.RoleSurveyor},
		{"gis_analyst", "GIS分析师", constants.RoleGISAnalyst},
		{"reviewer", "复核员", constants.RoleReviewer},
		{"auditor", "审计员", constants.RoleAuditor},
		{"admin", "系统管理员", constants.RoleAdmin},
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("DemoPass123!"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}
	for _, account := range accounts {
		var existing model.User
		err := db.Where("username = ?", account.username).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find seed user: %w", err)
		}
		user := model.User{Username: account.username, DisplayName: account.display, Role: account.role, PasswordHash: string(hash), Active: true}
		if err := db.Create(&user).Error; err != nil {
			return fmt.Errorf("create seed user %s: %w", account.username, err)
		}
	}
	return nil
}

type UserRepository struct{ db *gorm.DB }

func (r *UserRepository) FindByUsername(username string) (model.User, error) {
	var user model.User
	if err := r.db.Where("username = ? AND active = ?", username, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user, ErrNotFound
		}
		return user, fmt.Errorf("find user by username: %w", err)
	}
	return user, nil
}

func (r *UserRepository) FindByID(id uint) (model.User, error) {
	var user model.User
	if err := r.db.Where("id = ? AND active = ?", id, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user, ErrNotFound
		}
		return user, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

type AuditRepository struct{ db *gorm.DB }

func (r *AuditRepository) Create(entry *model.AuditLog) error {
	if strings.TrimSpace(entry.RequestID) == "" {
		return fmt.Errorf("audit request id is required")
	}
	if err := r.db.Create(entry).Error; err != nil {
		return fmt.Errorf("create immutable audit entry: %w", err)
	}
	return nil
}

func (r *AuditRepository) List(query dto.AuditQuery) ([]model.AuditLog, int64, error) {
	db := r.db.Model(&model.AuditLog{})
	if entity := strings.TrimSpace(query.ResourceType); entity != "" {
		db = db.Where("resource_type = ?", entity)
	}
	if requestID := strings.TrimSpace(query.RequestID); requestID != "" {
		db = db.Where("request_id = ?", requestID)
	}
	if actorName := strings.TrimSpace(query.ActorName); actorName != "" {
		db = db.Where("actor_name LIKE ?", "%"+actorName+"%")
	}
	if action := strings.TrimSpace(query.Action); action != "" {
		db = db.Where("action = ?", action)
	}
	if query.From != nil {
		db = db.Where("created_at >= ?", *query.From)
	}
	if query.To != nil {
		db = db.Where("created_at <= ?", *query.To)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}
	var entries []model.AuditLog
	if err := db.Order("created_at DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&entries).Error; err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	return entries, total, nil
}

// migratePostGIS materializes the validated GeoJSON fields into spatial
// columns. SQLite smoke deliberately relies on the same Go geometry checks;
// production PostgreSQL additionally keeps database-side spatial invariants
// and GiST indexes for operational inspection and future spatial queries.
func migratePostGIS(db *gorm.DB) error {
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS postgis`,
		`ALTER TABLE land_parcels ADD COLUMN IF NOT EXISTS boundary_geom geometry(Polygon)`,
		`ALTER TABLE survey_observations ADD COLUMN IF NOT EXISTS point_geom geometry(Point)`,
		`ALTER TABLE boundary_proposals ADD COLUMN IF NOT EXISTS proposed_geom geometry(Polygon)`,
		`ALTER TABLE topology_conflicts ADD COLUMN IF NOT EXISTS conflict_geom geometry`,
		`CREATE INDEX IF NOT EXISTS idx_land_parcels_boundary_geom ON land_parcels USING GIST (boundary_geom)`,
		`CREATE INDEX IF NOT EXISTS idx_survey_observations_point_geom ON survey_observations USING GIST (point_geom)`,
		`CREATE INDEX IF NOT EXISTS idx_boundary_proposals_proposed_geom ON boundary_proposals USING GIST (proposed_geom)`,
		`CREATE INDEX IF NOT EXISTS idx_topology_conflicts_conflict_geom ON topology_conflicts USING GIST (conflict_geom)`,
		`CREATE OR REPLACE FUNCTION cadastral_polygon_from_geojson(source text) RETURNS geometry AS $$
DECLARE parsed geometry;
BEGIN
  parsed := ST_SetSRID(ST_Force2D(ST_GeomFromGeoJSON(source)), 0);
  IF GeometryType(parsed) <> 'POLYGON' OR NOT ST_IsValid(parsed) THEN
    RAISE EXCEPTION 'cadastral polygon GeoJSON is invalid';
  END IF;
  RETURN parsed;
END;
$$ LANGUAGE plpgsql IMMUTABLE`,
		`CREATE OR REPLACE FUNCTION cadastral_point_from_geojson(source text) RETURNS geometry AS $$
DECLARE parsed geometry;
BEGIN
  parsed := ST_SetSRID(ST_Force2D(ST_GeomFromGeoJSON(source)), 0);
  IF GeometryType(parsed) <> 'POINT' OR NOT ST_IsValid(parsed) THEN
    RAISE EXCEPTION 'cadastral point GeoJSON is invalid';
  END IF;
  RETURN parsed;
END;
$$ LANGUAGE plpgsql IMMUTABLE`,
		`CREATE OR REPLACE FUNCTION cadastral_sync_land_parcel_geom() RETURNS trigger AS $$
BEGIN
  NEW.boundary_geom := cadastral_polygon_from_geojson(NEW.boundary_geojson);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE FUNCTION cadastral_sync_observation_geom() RETURNS trigger AS $$
BEGIN
  NEW.point_geom := cadastral_point_from_geojson(NEW.point_geojson);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE FUNCTION cadastral_sync_proposal_geom() RETURNS trigger AS $$
BEGIN
  NEW.proposed_geom := cadastral_polygon_from_geojson(NEW.proposed_geojson);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
		`CREATE OR REPLACE FUNCTION cadastral_sync_conflict_geom() RETURNS trigger AS $$
BEGIN
  IF NEW.geometry_geojson IS NULL OR btrim(NEW.geometry_geojson) = '' THEN
    NEW.conflict_geom := NULL;
  ELSE
    NEW.conflict_geom := ST_SetSRID(ST_Force2D(ST_GeomFromGeoJSON(NEW.geometry_geojson)), 0);
    IF NOT ST_IsValid(NEW.conflict_geom) THEN
      RAISE EXCEPTION 'topology conflict GeoJSON is invalid';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS sync_land_parcel_geom ON land_parcels`,
		`CREATE TRIGGER sync_land_parcel_geom BEFORE INSERT OR UPDATE OF boundary_geojson ON land_parcels FOR EACH ROW EXECUTE FUNCTION cadastral_sync_land_parcel_geom()`,
		`DROP TRIGGER IF EXISTS sync_observation_geom ON survey_observations`,
		`CREATE TRIGGER sync_observation_geom BEFORE INSERT OR UPDATE OF point_geojson ON survey_observations FOR EACH ROW EXECUTE FUNCTION cadastral_sync_observation_geom()`,
		`DROP TRIGGER IF EXISTS sync_proposal_geom ON boundary_proposals`,
		`CREATE TRIGGER sync_proposal_geom BEFORE INSERT OR UPDATE OF proposed_geojson ON boundary_proposals FOR EACH ROW EXECUTE FUNCTION cadastral_sync_proposal_geom()`,
		`DROP TRIGGER IF EXISTS sync_conflict_geom ON topology_conflicts`,
		`CREATE TRIGGER sync_conflict_geom BEFORE INSERT OR UPDATE OF geometry_geojson ON topology_conflicts FOR EACH ROW EXECUTE FUNCTION cadastral_sync_conflict_geom()`,
		`UPDATE land_parcels SET boundary_geom = cadastral_polygon_from_geojson(boundary_geojson) WHERE boundary_geom IS NULL`,
		`UPDATE survey_observations SET point_geom = cadastral_point_from_geojson(point_geojson) WHERE point_geom IS NULL`,
		`UPDATE boundary_proposals SET proposed_geom = cadastral_polygon_from_geojson(proposed_geojson) WHERE proposed_geom IS NULL`,
		`UPDATE topology_conflicts SET conflict_geom = CASE WHEN geometry_geojson IS NULL OR btrim(geometry_geojson) = '' THEN NULL ELSE ST_SetSRID(ST_Force2D(ST_GeomFromGeoJSON(geometry_geojson)), 0) END WHERE conflict_geom IS NULL`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("install PostGIS geometry migration: %w", err)
		}
	}
	return nil
}
