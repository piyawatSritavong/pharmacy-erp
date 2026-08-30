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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch command {
	case "migrate":
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrations applied")
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
		if err := application.Migrate(ctx); err != nil {
			log.Fatalf("migrate before serve: %v", err)
		}
		if err := application.Seed(ctx); err != nil {
			log.Fatalf("seed before serve: %v", err)
		}
		if err := application.Serve(); err != nil {
			log.Fatalf("serve: %v", err)
		}
	default:
		log.Fatalf("unknown command %q", command)
	}
}
