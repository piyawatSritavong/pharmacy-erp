package products

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

// Product listing uses two dynamic numeric expressions: the effective selling
// price and the private ghost-stock threshold. Keep an integration check for
// superadmin because swapping their SELECT positions compiles successfully but
// fails while scanning real PostgreSQL rows.
func TestListProductsScansSuperadminPriceAndGhostThreshold(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	service := NewService(db, audit.NewService(db), t.TempDir())
	result, err := service.List(context.Background(), platform.AuthUser{RoleKey: "super_admin"}, "", ListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list products as superadmin: %v", err)
	}
	if len(result.Items) == 0 {
		t.Skip("database has no seeded products")
	}
	if _, ok := result.Items[0]["effective_price"].(float64); !ok {
		t.Fatalf("effective_price has unexpected type %T", result.Items[0]["effective_price"])
	}
	if _, ok := result.Items[0]["low_stock_ghost_threshold"].(int); !ok {
		t.Fatalf("ghost threshold has unexpected type %T", result.Items[0]["low_stock_ghost_threshold"])
	}
}

// D3: deleting a category must never orphan its products — they get
// reassigned into an auto-created "ยังไม่จัดหมวด" bucket instead of relying
// on category_id's plain ON DELETE SET NULL.
func TestDeleteCategoryReassignsProductsToUncategorized(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	var userID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	auditService := audit.NewService(db)
	service := NewService(db, auditService, t.TempDir())
	user := platform.AuthUser{ID: userID, RoleKey: "super_admin"}
	meta := audit.LogEntry{ActorID: &userID}

	unique := platform.MustUUID()[:8]
	categoryID, err := service.CreateCategory(ctx, meta, CategoryInput{Name: "ทดสอบลบหมวด " + unique, Color: "#123456"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	productID, err := service.CreateProduct(ctx, user, meta, ProductInput{
		Name:       "สินค้าทดสอบลบหมวด " + unique,
		UnitName:   "ชิ้น",
		CategoryID: &categoryID,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	moved, err := service.DeleteCategory(ctx, categoryID, meta)
	if err != nil {
		t.Fatalf("delete category: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 product reassigned, got %d", moved)
	}

	var stillExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_categories WHERE id = $1)`, categoryID).Scan(&stillExists); err != nil {
		t.Fatal(err)
	}
	if stillExists {
		t.Fatal("expected category row to be deleted")
	}

	var newCategoryName sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT c.name FROM products p JOIN product_categories c ON c.id = p.category_id WHERE p.id = $1
	`, productID).Scan(&newCategoryName); err != nil {
		t.Fatal(err)
	}
	if !newCategoryName.Valid || newCategoryName.String != uncategorizedCategoryName {
		t.Fatalf("expected product to be reassigned to %q, got %v", uncategorizedCategoryName, newCategoryName)
	}

	// The safety-net bucket itself must be undeletable.
	var uncategorizedID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM product_categories WHERE name = $1`, uncategorizedCategoryName).Scan(&uncategorizedID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteCategory(ctx, uncategorizedID, meta); err == nil {
		t.Fatal("expected deleting the ยังไม่จัดหมวด category itself to fail")
	}
}
