package returns

import (
	"net/http"
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

// Resolving a claim has to be able to reach Ghost, or every qty_ghost branch in
// this package is unreachable — which is what it was before stock claims
// existed. Everyone below Superadmin is still refused outright.
func TestGhostClaimReachesSuperadminAndNobodyElse(t *testing.T) {
	for _, test := range []struct {
		role      string
		allowed   bool
		code      int
		claimKind string
	}{
		{role: "super_admin", allowed: true},
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
	} {
		err := validateReturnVisibility(platform.AuthUser{RoleKey: test.role}, "branch", "ghost")
		if test.allowed {
			if err != nil {
				t.Fatalf("role %s: expected Ghost to be permitted, got %v", test.role, err)
			}
			continue
		}
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}

// Real stock is every role's business, so the Ghost guard must not touch it.
func TestRealClaimIsOpenToEveryRoleInScope(t *testing.T) {
	for _, role := range []string{"super_admin", "central_admin", "branch_admin", "branch_pos"} {
		if err := validateReturnVisibility(platform.AuthUser{RoleKey: role}, "branch", "real"); err != nil {
			t.Fatalf("role %s: real stock should pass, got %v", role, err)
		}
	}
}

// A branch may only act on its own claims, whichever bucket they sit in.
func TestClaimStaysInsideTheBranchScope(t *testing.T) {
	other := "another-branch"
	err := validateReturnVisibility(platform.AuthUser{RoleKey: "branch_admin", BranchID: &other, Scope: "branch"}, "branch", "real")
	appErr, ok := err.(*platform.AppError)
	if !ok || appErr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a foreign branch, got %#v", err)
	}
	same := "branch"
	if err := validateReturnVisibility(platform.AuthUser{RoleKey: "branch_admin", BranchID: &same, Scope: "branch"}, "branch", "real"); err != nil {
		t.Fatalf("own branch should pass, got %v", err)
	}
}

// The sales, quotation and transfer paths keep the blanket ban: their guard is
// EnforceGhostWritePolicy, which refuses a Superadmin too.
func TestSellingGhostStaysImpossibleForEveryRole(t *testing.T) {
	for _, test := range []struct {
		role string
		code int
	}{
		{role: "super_admin", code: http.StatusBadRequest},
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
	} {
		err := platform.EnforceGhostWritePolicy(platform.AuthUser{RoleKey: test.role}, true)
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}
