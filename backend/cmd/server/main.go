package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lohasle/nimbus-framework-go/docs"
	"github.com/lohasle/nimbus-framework-go/internal/modules/infra"
	"github.com/lohasle/nimbus-framework-go/internal/modules/system"
	"github.com/lohasle/nimbus-framework-go/internal/modules/visionai"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/database"
	"github.com/lohasle/nimbus-framework-go/internal/platform/router"
)

// @title Nimbus Framework Go API
// @version 1.0
// @description Go modular-monolith scaffold for operations platforms. Default database: MySQL 8.4.
// @BasePath /admin-api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	db, err := database.Open(cfg)
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	var tenant system.Tenant
	if err = db.Where("name = ?", cfg.BootstrapTenant).First(&tenant).Error; err == nil {
		err = infra.Migrate(db)
	}
	if err == nil {
		err = visionai.Migrate(db)
	}
	if err == nil {
		err = infra.Seed(db, tenant.ID)
	}
	if err != nil {
		slog.Error("module database initialization failed", "error", err)
		os.Exit(1)
	}
	service := system.NewService(db, cfg)
	engine := router.New(system.NewHandler(service), db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go visionai.RunOutboxPublisher(ctx, db, cfg)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	slog.Info("nimbus server started", "address", cfg.HTTPAddr)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = server.Shutdown(shutdownCtx); err != nil {
			slog.Error("nimbus server shutdown failed", "error", err)
		}
	case err = <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("nimbus server stopped", "error", err)
			os.Exit(1)
		}
	}
}
