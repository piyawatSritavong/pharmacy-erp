package auth

import (
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func TestNavigationForPOS(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_pos"}

	items := navigationFor(user)
	if len(items) != 4 {
		t.Fatalf("expected 4 POS navigation items, got %d", len(items))
	}
	if items[0]["href"] != "/sales" {
		t.Fatalf("expected POS first navigation item to be /sales, got %#v", items[0]["href"])
	}
	if items[3]["href"] != "/daily-sales" {
		t.Fatalf("expected POS summary navigation item, got %#v", items[3]["href"])
	}
	if homePathFor(user) != "/sales" {
		t.Fatalf("expected sales home path, got %s", homePathFor(user))
	}
}

func TestNavigationForBranchAdmin(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_admin"}

	items := navigationFor(user)
	if len(items) != 4 {
		t.Fatalf("expected 4 branch admin navigation items, got %d", len(items))
	}
	if items[0]["href"] != "/branch-dashboard" {
		t.Fatalf("expected branch dashboard home, got %#v", items[0]["href"])
	}
	if items[1]["href"] != "/branch-inventory" {
		t.Fatalf("expected branch inventory menu, got %#v", items[1]["href"])
	}
	if items[3]["href"] != "/local-finance" {
		t.Fatalf("expected local finance menu, got %#v", items[3]["href"])
	}
	if homePathFor(user) != "/branch-dashboard" {
		t.Fatalf("expected branch dashboard home path, got %s", homePathFor(user))
	}
}
