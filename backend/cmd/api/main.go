package main

import (
	"context"
	"log"
	"os"
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
