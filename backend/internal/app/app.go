package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/http"
	"pharmacy-erp/backend/internal/platform/objectstore"
)

// ShutdownDrain is how long in-flight requests get to finish after SIGTERM.
// Cloud Run's default grace period is 10 seconds, so anything longer would be
// cut short by the platform rather than honoured here.
const ShutdownDrain = 10 * time.Second

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

// redactURL keeps a misconfigured value legible without printing a password
// that a connection string would carry.
func redactURL(value string) string {
	value = strings.TrimSpace(value)
	if at := strings.LastIndex(value, "@"); at > 0 {
		if scheme := strings.Index(value, "://"); scheme >= 0 && scheme+3 < at {
			return value[:scheme+3] + "***@" + value[at+1:]
		}
	}
	return value
}

// UploadProductImages puts the catalog photographs into the bucket. It is the
// migration off local disk, and it is idempotent, so it is also what a new
// environment runs to fill an empty bucket.
func (a *App) UploadProductImages(ctx context.Context) (uploaded int, skipped int, err error) {
	manifest, err := LoadOchaCatalog()
	if err != nil {
		return 0, 0, err
	}
	if err := objectstore.ValidateURL(a.Config.SupabaseURL); err != nil {
		return 0, 0, fmt.Errorf("SUPABASE_URL=%q: %w", redactURL(a.Config.SupabaseURL), err)
	}
	if strings.TrimSpace(a.Config.SupabaseServiceRoleKey) == "" {
		return 0, 0, errors.New("SUPABASE_SERVICE_ROLE_KEY is empty; copy the service_role key from the Supabase dashboard under Settings → API")
	}
	images := objectstore.New(a.Config.SupabaseURL, a.Config.SupabaseServiceRoleKey, a.Config.ProductImageBucket)
	return InstallOchaImageAssets(ctx, images, manifest)
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

// Serve runs the API until the process is asked to stop, then drains.
//
// The timeouts are the ones an instance needs when it faces the public
// internet: Echo's own Start sets none, so a client that opens a connection and
// then stalls holds a slot on that instance for as long as it likes.
//
// Cloud Run's shutdown is SIGTERM followed by a grace period, and it can still
// route a request to an instance for a moment after sending it. So the listener
// is closed first and the drain window is spent finishing what is already in
// flight, rather than exiting the moment the signal lands and cutting off a
// sale that was halfway through being rung up.
func (a *App) Serve() error {
	server := stdhttp.Server{
		Addr:              fmt.Sprintf(":%s", a.Config.HTTPPort),
		Handler:           http.NewServer(a.Config, a.DB).Handler(),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)

	serveErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-stop:
	}

	log.Println("shutdown signal received; draining")
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownDrain)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		// The window expired with requests still running. They are cut off
		// here; saying so is the point, since a truncated sale is exactly the
		// thing an operator would otherwise have to guess at.
		log.Printf("drain did not finish within %s: %v", ShutdownDrain, err)
		return server.Close()
	}
	log.Println("drained cleanly")
	return nil
}
