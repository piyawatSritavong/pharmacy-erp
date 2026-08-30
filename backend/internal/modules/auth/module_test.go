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

// flattenHrefs walks top-level items and any C1 group children so tests can
// assert on a page's presence regardless of which parent menu it's nested under.
func flattenHrefs(items []map[string]any) []string {
	hrefs := []string{}
	for _, item := range items {
		hrefs = append(hrefs, item["href"].(string))
		if children, ok := item["children"].([]map[string]any); ok {
			hrefs = append(hrefs, navigationHrefs(children)...)
		}
	}
	return hrefs
}

func containsAny(hrefs []string, href string) bool {
	for _, item := range hrefs {
		if item == href {
			return true
		}
	}
	return false
}

// branchPOSPermissions/superAdminPermissions mirror seed.go's
// rolePermissionKeys — navigationFor() is permission-driven (D11), so tests
// need a realistic permission set, not just a RoleKey (which is no longer
// what it switches on).
var branchPOSPermissions = []string{
	"dashboard.view.self", "products.view", "inventory.view.branch", "price.override.pos", "government.use",
	"transfer.request.branch", "invoice.create.pos", "invoice.view", "transfer.receive", "payment.collect",
}

var superAdminPermissions = []string{
	// inventory.ghost.manage is what unlocks สต๊อกผี since migration 038 —
	// สต๊อกจริง and สต๊อกผี are separately grantable.
	"dashboard.view.global", "products.manage", "products.view", "inventory.manage.global", "inventory.ghost.manage", "inventory.rebalance",
	"inventory.receive", "price.override.global", "government.manage_alias", "government.use", "invoice.sequence.manage", "invoice.view",
	"invoice.reprint", "quotation.manage",
	"transfer.approve", "transfer.request", "transfer.dispatch", "transfer.receive",
	"payment.collect", "reports.view.global", "reports.generate.global", "month_end.manage", "month_end.view", "month_end.create",
	"month_end.calculate", "month_end.adjust", "month_end.approve", "month_end.close", "month_end.reopen", "month_end.export", "settings.manage", "users.manage",
	"audit.view.global", "marketplace.manage.global", "marketplace.view.branch",
	"suppliers.view.global", "suppliers.manage.global", "purchase_orders.view.global", "purchase_orders.manage.global",
}

func TestNavigationForPOS(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_pos", Portal: "pos", Scope: "branch", Permissions: branchPOSPermissions}

	items := navigationFor(user)
	if len(items) != 6 {
		t.Fatalf("expected 6 POS navigation items, got %d: %v", len(items), navigationHrefs(items))
	}
	if items[0]["href"] != "/sales" {
		t.Fatalf("expected POS first navigation item to be /sales, got %#v", items[0]["href"])
	}
	// พักบิล is permission-free on purpose: branch_pos holds no document
	// permissions, so gating it would hide it from the only role that uses it.
	if !containsHref(items, "/parked-bills") {
		t.Fatalf("expected POS parked-bills menu, got %v", navigationHrefs(items))
	}
	if containsHref(items, "/installments") {
		t.Fatalf("did not expect POS installments menu, got %v", navigationHrefs(items))
	}
	if !containsHref(items, "/sales-history") {
		t.Fatalf("expected POS sales history menu, got %v", navigationHrefs(items))
	}
	if items[len(items)-1]["href"] != "/daily-sales" {
		t.Fatalf("expected POS summary navigation item last, got %v", navigationHrefs(items))
	}
	if homePathFor(user) != "/sales" {
		t.Fatalf("expected sales home path, got %s", homePathFor(user))
	}
}

func TestNavigationForSuperAdmin(t *testing.T) {
	user := platform.AuthUser{RoleKey: "super_admin", Portal: "backoffice", Scope: "global", Permissions: superAdminPermissions}

	items := navigationFor(user)
	// C1: grouped into 3 parent menus (รายงาน, คลังสินค้า, ใบเอกสาร) + settings standalone.
	if len(items) != 4 {
		t.Fatalf("expected 4 top-level super admin navigation items, got %d: %v", len(items), navigationHrefs(items))
	}
	if items[len(items)-1]["href"] != "/settings" {
		t.Fatalf("expected settings last, got %v", navigationHrefs(items))
	}
	for _, item := range items[:len(items)-1] {
		children, ok := item["children"].([]map[string]any)
		if !ok || len(children) == 0 {
			t.Fatalf("expected %v to be a group with children", item)
		}
		if item["href"] != children[0]["href"] {
			t.Fatalf("expected group %v href to match its first child %v", item["href"], children[0]["href"])
		}
	}

	all := flattenHrefs(items)
	if containsAny(all, "/external-accounting") {
		t.Fatalf("did not expect external accounting menu, got %v", all)
	}
	if containsAny(all, "/installments") {
		t.Fatalf("did not expect super admin installments menu, got %v", all)
	}
	// global_reports ("รายงาน" ภาษีและกำไรขาดทุน) is removed from the menu entirely per C1.
	if containsAny(all, "/global-reports") {
		t.Fatalf("did not expect global reports menu, got %v", all)
	}
	if !containsAny(all, "/sales-management") {
		t.Fatalf("expected transferred sales management (ใบขาย) menu, got %v", all)
	}
	for _, href := range []string{
		"/generate-report", "/month-end", "/dashboard",
		"/real-inventory", "/ghost-inventory", "/product-categories", "/transfers",
		"/purchase-orders", "/suppliers", "/government-sales",
	} {
		if !containsAny(all, href) {
			t.Fatalf("expected %s menu, got %v", href, all)
		}
	}
	if homePathFor(user) != "/dashboard" {
		t.Fatalf("expected dashboard home path, got %s", homePathFor(user))
	}
}

// officePermissions/branchHeadPermissions mirror migration
// 025_role_presets_and_scoping.sql's default sets.
var officePermissions = []string{
	"dashboard.view.global", "products.view", "invoice.view", "invoice.reprint", "invoice.sequence.manage",
	"quotation.manage", "payment.collect", "government.use", "government.manage_alias",
	"reports.view.global", "reports.generate.global",
	"month_end.view", "month_end.create", "month_end.calculate", "month_end.adjust", "month_end.export",
	"suppliers.view.global", "suppliers.manage.global", "purchase_orders.view.global", "purchase_orders.manage.global",
	"marketplace.view.branch", "audit.view.global",
}

var branchHeadPermissions = []string{
	"dashboard.view.self", "products.view", "inventory.view.branch", "inventory.rebalance", "inventory.receive",
	"transfer.request.branch", "transfer.receive", "invoice.view", "invoice.reprint", "payment.collect",
	"government.use", "marketplace.view.branch",
}

func TestNavigationForOffice(t *testing.T) {
	user := platform.AuthUser{RoleKey: "office", Portal: "backoffice", Scope: "global", Permissions: officePermissions}

	items := navigationFor(user)
	all := flattenHrefs(items)
	// office holds none of inventory.manage.global/products.manage/transfer.approve
	// — the inventory group must not appear at all.
	if containsAny(all, "/real-inventory") || containsAny(all, "/product-categories") || containsAny(all, "/transfers") {
		t.Fatalf("did not expect inventory-group pages for office, got %v", all)
	}
	// no settings.manage or users.manage — settings must be hidden.
	if containsAny(all, "/settings") {
		t.Fatalf("did not expect settings for office, got %v", all)
	}
	for _, href := range []string{"/generate-report", "/dashboard", "/purchase-orders", "/suppliers", "/government-sales", "/sales-management"} {
		if !containsAny(all, href) {
			t.Fatalf("expected %s menu for office, got %v", href, all)
		}
	}
	if containsAny(all, "/month-end") {
		t.Fatalf("did not expect superadmin-only /month-end menu for office, got %v", all)
	}
	if homePathFor(user) != "/dashboard" {
		t.Fatalf("expected dashboard home path for office, got %s", homePathFor(user))
	}
}

func TestNavigationForBranchHead(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_head", Portal: "backoffice", Scope: "branch", Permissions: branchHeadPermissions}

	items := navigationFor(user)
	all := flattenHrefs(items)
	// none of the *.global-gated groups apply to a branch-scoped role.
	if containsAny(all, "/real-inventory") || containsAny(all, "/purchase-orders") || containsAny(all, "/settings") {
		t.Fatalf("did not expect global-scope pages for branch_head, got %v", all)
	}
	// scope=="branch" gets its own branch_ops_group instead, reusing the POS
	// pages under back-office-appropriate permissions.
	for _, href := range []string{"/daily-sales", "/sales-history", "/inventory-check", "/transfer-receipts"} {
		if !containsAny(all, href) {
			t.Fatalf("expected %s menu for branch_head, got %v", href, all)
		}
	}
	if homePathFor(user) != "/daily-sales" {
		t.Fatalf("expected daily-sales home path for branch_head (no dashboard.view.global), got %s", homePathFor(user))
	}
}

// A central admin sees สต๊อกจริง but not สต๊อกผี — the whole point of splitting
// inventory.ghost.manage out of inventory.manage.global (migration 038).
func TestNavigationForCentralAdminHidesGhostStock(t *testing.T) {
	permissions := []string{}
	for _, key := range superAdminPermissions {
		if key == "inventory.ghost.manage" || key == "users.manage" || key == "settings.manage" {
			continue
		}
		permissions = append(permissions, key)
	}
	user := platform.AuthUser{RoleKey: "central_admin", Portal: "backoffice", Scope: "global", Permissions: permissions}

	routes := map[string]bool{}
	var walk func(items []map[string]any)
	walk = func(items []map[string]any) {
		for _, item := range items {
			if href, ok := item["href"].(string); ok {
				routes[href] = true
			}
			if children, ok := item["children"].([]map[string]any); ok {
				walk(children)
			}
		}
	}
	walk(navigationFor(user))

	if !routes["/real-inventory"] {
		t.Fatal("central admin should still see สต๊อกจริง")
	}
	if routes["/ghost-inventory"] {
		t.Fatal("central admin must not see สต๊อกผี")
	}
}
