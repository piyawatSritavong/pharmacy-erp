package auth

import (
	"context"
	"os"
	"testing"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"
)

func TestLoginAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	cfg := config.Load()
	service := NewService(db, cfg)
	result, err := service.Login(context.Background(), LoginRequest{
		Email: "pos.mes@erp.local", Password: "LocalDevTill-2026!",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.User.RoleKey != "branch_pos" || result.HomePath != "/sales" {
		t.Fatalf("unexpected POS session: %#v", result)
	}
}
