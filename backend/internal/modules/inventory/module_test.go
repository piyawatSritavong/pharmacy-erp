package inventory

import (
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

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
	user := platform.AuthUser{RoleKey: "branch_admin", BranchID: &branchID}

	if err := validateBranchScope(user, branchID); err != nil {
		t.Fatalf("expected same-branch access to pass: %v", err)
	}
	if err := validateBranchScope(user, otherBranchID); err == nil {
		t.Fatal("expected cross-branch access to be rejected")
	}
}
