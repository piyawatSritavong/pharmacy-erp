package inventory

import (
	"context"
	"net/http"
	"testing"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

func requireAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	appErr, ok := err.(*platform.AppError)
	if !ok || appErr.Code != code {
		t.Fatalf("expected AppError %d, got %#v", code, err)
	}
}

func TestApplyRebalanceResult(t *testing.T) {
	qtyReal, qtyGhost, err := applyRebalanceResult(10, 5, 3, "real")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qtyReal != 7 || qtyGhost != 8 {
		t.Fatalf("unexpected rebalance result: %d %d", qtyReal, qtyGhost)
	}
}

func TestApplyAdjustmentResult(t *testing.T) {
	qtyReal, qtyGhost, err := applyAdjustmentResult(10, 5, "ghost", -2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if qtyReal != 10 || qtyGhost != 3 {
		t.Fatalf("unexpected adjustment result: %d %d", qtyReal, qtyGhost)
	}
}

func TestValidateBranchScope(t *testing.T) {
	branchID := "branch-a"
	otherBranchID := "branch-b"
	user := platform.AuthUser{RoleKey: "branch_pos", BranchID: &branchID}

	if err := validateBranchScope(user, branchID); err != nil {
		t.Fatalf("expected same-branch access to pass: %v", err)
	}
	if err := validateBranchScope(user, otherBranchID); err == nil {
		t.Fatal("expected cross-branch access to be rejected")
	}
}

func TestOperationalInventoryWritesRejectGhostBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	for _, test := range []struct {
		role string
		code int
	}{
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
		{role: "super_admin", code: http.StatusBadRequest},
	} {
		user := platform.AuthUser{RoleKey: test.role, Permissions: []string{"inventory.manage.global"}}
		requireAppErrorCode(t, service.Rebalance(context.Background(), user, audit.LogEntry{}, RebalanceRequest{
			ProductID: "product", BranchID: "branch", FromBucket: "real", ToBucket: "ghost", Quantity: 1,
		}), test.code)
		requireAppErrorCode(t, service.Adjust(context.Background(), user, audit.LogEntry{}, AdjustRequest{
			ProductID: "product", BranchID: "branch", StockBucket: "ghost", QuantityDelta: 1,
		}), test.code)
		requireAppErrorCode(t, service.Receive(context.Background(), user, audit.LogEntry{}, ReceiveRequest{
			ProductID: "product", BranchID: "branch", GhostQuantity: 1,
		}), test.code)
		requireAppErrorCode(t, service.ReviewReceiptRequest(context.Background(), user, audit.LogEntry{}, "request", ReviewReceiptRequest{
			Decision: "approve", Quantity: 1, StockBucket: "ghost",
		}), test.code)
	}
}
