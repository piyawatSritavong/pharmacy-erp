package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/app"
	"pharmacy-erp/backend/internal/config"
)

// migrationTimeout bounds the one-shot commands. It is generous because a
// migration against a cold managed database is slow, and nothing waits on it:
// it is a deploy step, not something in the path of a first request.
const migrationTimeout = 5 * time.Minute

func main() {
	cfg := config.Load()

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	defer func() {
		_ = application.Close()
	}()

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	// The one-shot commands get a bounded context. serve does not: it is not a
	// task that finishes, and a deadline on it would stop the server after a
	// minute.
	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()

	switch command {
	case "migrate":
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
	case "migrate-and-seed":
		// What serve used to do implicitly, kept as something you run on
		// purpose: the compose stack and a fresh environment both need the
		// schema and the seed in one step.
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
		if err := application.Seed(ctx); err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Println("seed completed")
	case "upload-product-images":
		// The one-off move from UPLOAD_DIR to Supabase Storage, and the step a
		// fresh environment runs to fill an empty bucket. Safe to repeat: what
		// is already there is skipped.
		uploaded, skipped, err := application.UploadProductImages(ctx)
		if err != nil {
			log.Fatalf("upload product images: %v", err)
		}
		log.Printf("product images: %d uploaded, %d already present", uploaded, skipped)
	case "seed":
		if err := application.Seed(ctx); err != nil {
			log.Fatalf("seed: %v", err)
		}
		log.Println("seed completed")
	case "seed-inventory-floor":
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate before inventory floor seed: %v", err)
		}
		result, err := application.SeedInventoryFloor(ctx)
		if err != nil {
			log.Fatalf("seed inventory floor: %v", err)
		}
		for _, line := range result.Lines() {
			log.Println(line)
		}
		log.Println("inventory floor seed completed")
	case "reset-operational-data":
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate before operational reset: %v", err)
		}
		confirmation := ""
		for _, argument := range os.Args[2:] {
			if strings.HasPrefix(argument, "--confirm=") {
				confirmation = strings.TrimPrefix(argument, "--confirm=")
			}
		}
		result, err := application.ResetOperationalData(ctx, confirmation)
		if err != nil {
			log.Fatalf("reset operational data: %v", err)
		}
		for _, line := range result.Lines() {
			log.Println(line)
		}
		if result.DryRun {
			log.Println("dry-run only; pass --confirm=RESET-OPERATIONAL-DATA and ALLOW_OPERATIONAL_DATA_RESET=true to execute")
		} else {
			log.Println("operational data reset completed")
		}
	case "replace-ocha-catalog":
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate before Ocha catalog replacement: %v", err)
		}
		confirmation := ""
		for _, argument := range os.Args[2:] {
			if strings.HasPrefix(argument, "--confirm=") {
				confirmation = strings.TrimPrefix(argument, "--confirm=")
			}
		}
		result, err := application.ReplaceOchaCatalog(ctx, confirmation)
		if err != nil {
			log.Fatalf("replace Ocha catalog: %v", err)
		}
		for _, line := range result.Lines() {
			log.Println(line)
		}
		if result.DryRun {
			log.Println("dry-run only; back up the database and uploads, then pass --confirm=REPLACE-OCHA-CATALOG with ALLOW_MASTER_DATA_RESET=true")
		} else {
			log.Println("Ocha catalog replacement completed")
		}
	case "serve":
		// serve serves. It used to run Migrate then Seed on every boot, which
		// is wrong anywhere more than one copy of the process starts at once:
		// Cloud Run brings up instances in parallel and they would race each
		// other through the same migrations, against a schema_migrations table
		// that has no lock and decides what to apply by reading the row first.
		// Cold start made it worse — the whole of migrate and seed had to fit
		// inside one 60-second context before the first request could be
		// served. Migrations are now a deploy step: `migrate`, run once.
		cancel()
		if err := application.Serve(); err != nil {
			log.Fatalf("serve: %v", err)
		}
	default:
		log.Fatalf("unknown command %q (serve, migrate, migrate-and-seed, seed, upload-product-images, seed-inventory-floor, reset-operational-data, replace-ocha-catalog)", command)
	}
}
