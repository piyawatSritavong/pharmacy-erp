package auth

import (
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func navigationHrefs(items []map[string]any) []string {
	hrefs := make([]string, 0, len(items))
	for _, item := range items {
		hrefs = append(hrefs, item["href"].(string))
	}
	return hrefs
}

func containsHref(items []map[string]any, href string) bool {
	for _, item := range items {
		if item["href"] == href {
			return true
		}
	}
	return false
}

func TestNavigationForPOS(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_pos"}

	items := navigationFor(user)
	if len(items) != 5 {
		t.Fatalf("expected 5 POS navigation items, got %d: %v", len(items), navigationHrefs(items))
	}
	if items[0]["href"] != "/sales" {
		t.Fatalf("expected POS first navigation item to be /sales, got %#v", items[0]["href"])
	}
	if !containsHref(items, "/installments") {
		t.Fatalf("expected POS installments menu, got %v", navigationHrefs(items))
	}
	if items[len(items)-1]["href"] != "/daily-sales" {
		t.Fatalf("expected POS summary navigation item last, got %v", navigationHrefs(items))
	}
	if homePathFor(user) != "/sales" {
		t.Fatalf("expected sales home path, got %s", homePathFor(user))
	}
}

func TestNavigationForBranchAdmin(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_admin"}

	items := navigationFor(user)
	if len(items) != 5 {
		t.Fatalf("expected 5 branch admin navigation items, got %d: %v", len(items), navigationHrefs(items))
	}
	if items[0]["href"] != "/branch-dashboard" {
		t.Fatalf("expected branch dashboard home, got %#v", items[0]["href"])
	}
	if items[1]["href"] != "/branch-inventory" {
		t.Fatalf("expected branch inventory menu, got %#v", items[1]["href"])
	}
	if !containsHref(items, "/installments") {
		t.Fatalf("expected branch admin installments menu, got %v", navigationHrefs(items))
	}
	if items[len(items)-1]["href"] != "/local-finance" {
		t.Fatalf("expected local finance menu last, got %v", navigationHrefs(items))
	}
	if homePathFor(user) != "/branch-dashboard" {
		t.Fatalf("expected branch dashboard home path, got %s", homePathFor(user))
	}
}

func TestNavigationForSuperAdmin(t *testing.T) {
	user := platform.AuthUser{RoleKey: "super_admin"}

	items := navigationFor(user)
	if len(items) != 6 {
		t.Fatalf("expected 6 super admin navigation items, got %d: %v", len(items), navigationHrefs(items))
	}
	if !containsHref(items, "/installments") {
		t.Fatalf("expected super admin installments menu, got %v", navigationHrefs(items))
	}
	if homePathFor(user) != "/dashboard" {
		t.Fatalf("expected dashboard home path, got %s", homePathFor(user))
	}
}
