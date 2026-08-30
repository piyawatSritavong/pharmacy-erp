package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/app"
	"pharmacy-erp/backend/internal/config"
)

func TestIntegrationHarness(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	cfg := config.Load()
	cfg.DatabaseURL = databaseURL

	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer application.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := application.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := application.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}

	assertCatalogCount(t, application.DB, "branches", 6)
	assertCatalogCount(t, application.DB, "product_categories", 12)
	assertCatalogAtLeast(t, application.DB, "products", 694)
	assertCatalogAtLeast(t, application.DB, "inventory", 1374)
	assertRoleAbsent(t, application.DB, "branch_admin")
	assertInventoryLedgerBalanced(t, application.DB)
}

func assertCatalogAtLeast(t *testing.T, db *sql.DB, table string, expected int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count < expected {
		t.Fatalf("expected %s to contain at least %d Ocha rows, got %d", table, expected, count)
	}
}

func assertCatalogCount(t *testing.T, db *sql.DB, table string, expected int) {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != expected {
		t.Fatalf("expected %s to contain %d Ocha rows, got %d", table, expected, count)
	}
}

func assertRoleAbsent(t *testing.T, db *sql.DB, roleKey string) {
	t.Helper()

	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM roles WHERE role_key = $1
		)
	`, roleKey).Scan(&exists); err != nil {
		t.Fatalf("query role: %v", err)
	}
	if exists {
		t.Fatalf("expected role %s to be absent", roleKey)
	}
}

func assertInventoryLedgerBalanced(t *testing.T, db *sql.DB) {
	t.Helper()
	var mismatches int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM inventory i
		WHERE i.qty_real <> COALESCE((
			SELECT SUM(m.quantity_delta) FROM inventory_movements m
			WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'real'
		), 0)
		OR i.qty_ghost <> COALESCE((
			SELECT SUM(m.quantity_delta) FROM inventory_movements m
			WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'ghost'
		), 0)
	`).Scan(&mismatches); err != nil {
		t.Fatalf("query inventory ledger balance: %v", err)
	}
	if mismatches != 0 {
		t.Fatalf("expected inventory ledger to be balanced, found %d mismatches", mismatches)
	}
}
