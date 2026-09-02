package monthend

import (
	"context"
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func TestNormalizeMonthEndReportPage(t *testing.T) {
	page, pageSize := normalizeMonthEndReportPage(0, 1000)
	if page != 1 || pageSize != 200 {
		t.Fatalf("unexpected normalized pagination: %d/%d", page, pageSize)
	}
	page, pageSize = normalizeMonthEndReportPage(3, 0)
	if page != 3 || pageSize != 50 {
		t.Fatalf("unexpected default pagination: %d/%d", page, pageSize)
	}
}

func TestMonthEndReportRejectsNonSuperadminBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	_, err := service.MonthEndReport(context.Background(), platform.AuthUser{RoleKey: "central_admin"}, "", "2026-01", "", "", "", 1, 50)
	if err == nil {
		t.Fatal("expected non-superadmin report access to be rejected")
	}
}

func TestMonthEndReportValidatesPeriodBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	_, err := service.MonthEndReport(context.Background(), platform.AuthUser{RoleKey: "super_admin"}, "", "not-a-period", "", "", "", 1, 50)
	if err == nil {
		t.Fatal("expected invalid period to be rejected")
	}
}
