package promotions

import (
	"net/http"
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func branchUser(branchID string) platform.AuthUser {
	return platform.AuthUser{RoleKey: "branch_pos", Scope: "branch", BranchID: &branchID}
}

// A shop's promotion is its own, whatever the request asks for: the branch is
// taken from who is signed in, not from the payload.
func TestBranchPromotionIsPinnedToItsOwnBranch(t *testing.T) {
	own := "branch-a"
	for _, requested := range []string{"", "branch-a", "  branch-a  "} {
		got, err := ownerBranch(branchUser(own), requested)
		if err != nil {
			t.Fatalf("requested %q: unexpected error %v", requested, err)
		}
		if got != own {
			t.Fatalf("requested %q: expected the caller's own branch, got %q", requested, got)
		}
	}
}

func TestBranchCannotAimAPromotionAtAnotherShop(t *testing.T) {
	_, err := ownerBranch(branchUser("branch-a"), "branch-b")
	appErr, ok := err.(*platform.AppError)
	if !ok || appErr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for another branch, got %#v", err)
	}
}

// Head office keeps the wider choice — a named branch, or none at all, which is
// what a company-wide promotion is.
func TestHeadOfficeChoosesAnyScopeIncludingEveryBranch(t *testing.T) {
	global := platform.AuthUser{RoleKey: "super_admin", Scope: "global"}
	for requested, want := range map[string]string{"": "", "branch-b": "branch-b"} {
		got, err := ownerBranch(global, requested)
		if err != nil {
			t.Fatalf("requested %q: unexpected error %v", requested, err)
		}
		if got != want {
			t.Fatalf("requested %q: expected %q, got %q", requested, want, got)
		}
	}
	// A global role that happens to carry a branch is still global.
	branch := "branch-a"
	central := platform.AuthUser{RoleKey: "central_admin", Scope: "global", BranchID: &branch}
	if got, err := ownerBranch(central, "branch-b"); err != nil || got != "branch-b" {
		t.Fatalf("expected central_admin to keep its choice, got %q / %v", got, err)
	}
}
