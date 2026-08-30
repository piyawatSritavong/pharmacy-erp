package reportbuilder

import (
	"context"
	"os"
	"testing"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

func TestSavedReportsAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	ownerID := platform.MustUUID()
	var roleID, otherOwnerID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM roles WHERE role_key='branch_pos'`).Scan(&roleID); err != nil {
		t.Fatalf("load role for isolated report owner: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email = 'superadmin@erp.local'`).Scan(&otherOwnerID); err != nil {
		t.Fatalf("load other owner: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users(id,role_id,branch_id,full_name,email,password_hash,active,created_at,updated_at)
		VALUES($1,$2,NULL,'Report integration owner',$3,'not-used-by-test',TRUE,NOW(),NOW())
	`, ownerID, roleID, "report-integration-"+platform.MustUUID()+"@erp.local"); err != nil {
		t.Fatalf("create isolated report owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, ownerID)
	})

	service := NewService(db, audit.NewService(db))
	definition := defaultDefinition("branch_product_overview")
	definition.TimeConfig = nil
	first, err := service.Create(ctx, ownerID, SaveReportRequest{Name: "Integration Report " + platform.MustUUID(), Definition: definition}, audit.LogEntry{ActorID: &ownerID})
	if err != nil {
		t.Fatalf("create first report: %v", err)
	}
	if first.Definition.Layout == nil || first.Definition.Layout.FieldSidebarWidth != 288 {
		t.Fatalf("expected saved field sidebar width, got %#v", first.Definition.Layout)
	}
	second, err := service.Create(ctx, ownerID, SaveReportRequest{Name: "Integration Report " + platform.MustUUID(), Definition: definition}, audit.LogEntry{ActorID: &ownerID})
	if err != nil {
		t.Fatalf("create second report: %v", err)
	}

	if _, err := service.Get(ctx, otherOwnerID, first.ID); err == nil {
		t.Fatal("expected a different owner to be unable to read the report")
	}
	first, err = service.SetPinned(ctx, ownerID, first.ID, true, "generate_report", audit.LogEntry{ActorID: &ownerID})
	if err != nil || !first.IsPinned || first.PinOrder == nil || *first.PinOrder != 0 {
		t.Fatalf("pin first report: %#v, %v", first, err)
	}
	second, err = service.SetPinned(ctx, ownerID, second.ID, true, "generate_report", audit.LogEntry{ActorID: &ownerID})
	if err != nil || second.PinOrder == nil || *second.PinOrder != 1 {
		t.Fatalf("pin second report: %#v, %v", second, err)
	}
	if err := service.ReorderPins(ctx, ownerID, "generate_report", []string{second.ID, first.ID}, audit.LogEntry{ActorID: &ownerID}); err != nil {
		t.Fatalf("reorder reports: %v", err)
	}
	items, err := service.List(ctx, ownerID, "")
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(items) < 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("unexpected pinned order: %#v", items)
	}
	if _, err := service.SetPinned(ctx, ownerID, second.ID, false, "", audit.LogEntry{ActorID: &ownerID}); err != nil {
		t.Fatalf("unpin second report: %v", err)
	}
	first, err = service.Get(ctx, ownerID, first.ID)
	if err != nil || first.PinOrder == nil || *first.PinOrder != 0 {
		t.Fatalf("expected compacted pin order, got %#v, %v", first, err)
	}

	result, err := service.Execute(ctx, platform.AuthUser{ID: ownerID, RoleKey: "super_admin"}, ownerID, ExecuteRequest{Definition: &definition, Page: 1})
	if err != nil {
		t.Fatalf("execute overview: %v", err)
	}
	if !result.Meta.ReadOnly || len(result.Columns) == 0 {
		t.Fatalf("unexpected query result: %#v", result.Meta)
	}
}
