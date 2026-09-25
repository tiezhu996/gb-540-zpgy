package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/config"
	"cadastral-boundary-topology-resolution/backend/internal/handler"
	appmw "cadastral-boundary-topology-resolution/backend/internal/middleware"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
	"cadastral-boundary-topology-resolution/backend/internal/router"
	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_invalid", "error", err)
		os.Exit(1)
	}
	db, err := repository.Open(cfg)
	if err != nil {
		logger.Error("database_open_failed", "error", err)
		os.Exit(1)
	}
	if cfg.DBAutoMigrate {
		if err := repository.MigrateAndSeed(db); err != nil {
			logger.Error("database_prepare_failed", "error", err)
			os.Exit(1)
		}
	}
	store := repository.NewStore(db)
	authService := service.NewAuthService(store, cfg)
	cadastralService := service.NewCadastralService(store)
	auditService := service.NewAuditService(store)
	validate := validator.New(validator.WithRequiredStructEnabled())
	deps := router.Dependencies{
		AuthHandler:      handler.NewAuthHandler(authService, validate),
		AuditHandler:     handler.NewAuditHandler(auditService),
		AuthService:      authService,
		CadastralHandler: handler.NewCadastralHandler(cadastralService, validate),
		LoginLimiter:     appmw.NewRateLimiter(cfg.LoginRateLimit, time.Minute), ImportLimiter: appmw.NewRateLimiter(cfg.ImportRateLimit, time.Minute), AnalyzeLimiter: appmw.NewRateLimiter(cfg.AnalyzeRateLimit, time.Minute),
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(
		appmw.RequestIDMiddleware(),
		appmw.AuditContextMiddleware(),
		appmw.AccessLogMiddleware(logger),
		appmw.RecoveryMiddleware(logger),
		appmw.ErrorHandlerMiddleware(),
		appmw.CORSMiddleware(cfg.CORSOrigins),
	)
	router.Register(engine, deps)
	server := &http.Server{Addr: ":" + cfg.Port, Handler: engine, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		logger.Info("server_started", "port", cfg.Port, "db_driver", cfg.DBDriver)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server_shutdown_failed", "error", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	logger.Info("server_stopped")
}
