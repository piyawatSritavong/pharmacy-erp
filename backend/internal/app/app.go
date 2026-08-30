package app

import (
	"context"
	"database/sql"
	"fmt"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/http"
)

type App struct {
	Config config.Config
	DB     *sql.DB
}

func New(cfg config.Config) (*App, error) {
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	return &App{Config: cfg, DB: db}, nil
}

func (a *App) Close() error {
	if a.DB == nil {
		return nil
	}
	return a.DB.Close()
}

func (a *App) Migrate(ctx context.Context) error {
	return database.RunMigrations(ctx, a.DB)
}

func (a *App) Seed(ctx context.Context) error {
	return Seed(ctx, a.DB, a.Config)
}

func (a *App) SeedDemo(ctx context.Context) error {
	return SeedDemo(ctx, a.DB, a.Config)
}

func (a *App) SeedMonthEnd(ctx context.Context) error {
	return SeedMonthEnd(ctx, a.DB, a.Config)
}

func (a *App) SeedInventoryFloor(ctx context.Context) (SeedInventoryFloorResult, error) {
	return SeedInventoryFloor(ctx, a.DB, a.Config)
}

func (a *App) ResetOperationalData(ctx context.Context, confirmation string) (ResetOperationalDataResult, error) {
	return ResetOperationalData(ctx, a.DB, a.Config, confirmation)
}

func (a *App) ReplaceOchaCatalog(ctx context.Context, confirmation string) (ReplaceOchaCatalogResult, error) {
	return ReplaceOchaCatalog(ctx, a.DB, a.Config, confirmation)
}

func (a *App) Serve() error {
	server := http.NewServer(a.Config, a.DB)
	return server.Start(fmt.Sprintf(":%s", a.Config.HTTPPort))
}
