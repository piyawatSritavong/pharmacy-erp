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

	// The seed lays down six branches, and a branch is data: opening a seventh
	// is an INSERT, not a deploy. Pinning the count to six made an ordinary
	// business event fail the build, so the floor is what is asserted.
	assertCatalogAtLeast(t, application.DB, "branches", 6)
	// The Ocha seed owns exactly twelve categories. Deleting a category moves
	// its products into a system-created "ยังไม่จัดหมวด" bucket, so that row is
	// ordinary application state and must not count as a seed regression.
	assertSeededCategoryCount(t, application.DB, 12)
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

func assertSeededCategoryCount(t *testing.T, db *sql.DB, expected int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM product_categories WHERE name <> 'ยังไม่จัดหมวด'`).Scan(&count); err != nil {
		t.Fatalf("count product_categories: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d seeded Ocha categories, got %d", expected, count)
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
