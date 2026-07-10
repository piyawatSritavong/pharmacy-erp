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

	assertSeededDocPattern(t, application.DB, "invoices", "invoice_number", `^BL[0-9]{8}[0-9]{5}$`)
	assertSeededDocPattern(t, application.DB, "quotations", "quote_number", `^QT[0-9]{8}[0-9]{5}$`)
	assertRolePermissionAbsent(t, application.DB, "branch_admin", "payment.collect")
}

func assertSeededDocPattern(t *testing.T, db *sql.DB, table string, column string, pattern string) {
	t.Helper()

	var matches int
	query := `SELECT COUNT(*) FROM ` + table + ` WHERE ` + column + ` ~ $1`
	if err := db.QueryRow(query, pattern).Scan(&matches); err != nil {
		t.Fatalf("query seeded %s pattern: %v", table, err)
	}
	if matches == 0 {
		t.Fatalf("expected seeded %s rows matching pattern %s", table, pattern)
	}
}

func assertRolePermissionAbsent(t *testing.T, db *sql.DB, roleKey string, permissionKey string) {
	t.Helper()

	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM role_permissions rp
			INNER JOIN roles r ON r.id = rp.role_id
			INNER JOIN permissions p ON p.id = rp.permission_id
			WHERE r.role_key = $1 AND p.permission_key = $2
		)
	`, roleKey, permissionKey).Scan(&exists); err != nil {
		t.Fatalf("query role permission: %v", err)
	}
	if exists {
		t.Fatalf("expected %s to exclude %s", roleKey, permissionKey)
	}
}
