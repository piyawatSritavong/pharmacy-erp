package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/v2"
)

// This entrypoint serves only the additive V2 API. It does not migrate, seed,
// change any V1 endpoint on startup. V2 jobs require explicit opt-in.
func main() {
	cfg := config.Load()
	port := os.Getenv("V2_HTTP_PORT")
	if port == "" {
		port = "8082"
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("V2 database unavailable")
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("V2_JOBS_ENABLED") == "true" {
		go v2.RunJobs(ctx, db)
	}
	server := &http.Server{Addr: ":" + port, Handler: v2.NewHandler(cfg, db), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("V2 API listening on port %s", port)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print("V2 API server stopped unexpectedly")
	}
}
