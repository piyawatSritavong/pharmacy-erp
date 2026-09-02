package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/config"
	httpmiddleware "pharmacy-erp/backend/internal/http/middleware"
	"pharmacy-erp/backend/internal/platform"

	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	db     *sql.DB
	config config.Config
}

func NewService(db *sql.DB, cfg config.Config) *Service {
	return &Service{db: db, config: cfg}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SessionResponse struct {
	Token      string            `json:"token"`
	User       platform.AuthUser `json:"user"`
	Navigation []map[string]any  `json:"navigation"`
	HomePath   string            `json:"home_path"`
}

func (s *Service) Login(ctx context.Context, input LoginRequest) (SessionResponse, error) {
	type userRow struct {
		ID           string
		Name         string
		Email        string
		PasswordHash string
		RoleKey      string
		RoleName     string
		RoleActive   bool
		Portal       string
		Scope        string
		BranchID     sql.NullString
		BranchCode   sql.NullString
		BranchName   sql.NullString
		Active       bool
	}

	var row userRow
	if err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.full_name, u.email, u.password_hash, r.role_key, r.name, r.active, r.portal, r.scope,
		       u.branch_id::text, b.code, b.name, u.active
		FROM users u
		INNER JOIN roles r ON r.id = u.role_id
		LEFT JOIN branches b ON b.id = u.branch_id
		WHERE LOWER(u.email) = LOWER($1)
	`, strings.TrimSpace(input.Email)).Scan(
		&row.ID, &row.Name, &row.Email, &row.PasswordHash, &row.RoleKey, &row.RoleName, &row.RoleActive, &row.Portal, &row.Scope,
		&row.BranchID, &row.BranchCode, &row.BranchName, &row.Active,
	); err != nil {
		if err == sql.ErrNoRows {
			return SessionResponse{}, platform.NewError(http.StatusUnauthorized, "invalid credentials")
		}
		return SessionResponse{}, platform.WrapError(http.StatusInternalServerError, "failed to load user", err)
	}

	if !row.Active {
		return SessionResponse{}, platform.NewError(http.StatusForbidden, "account is inactive")
	}
	if !row.RoleActive {
		return SessionResponse{}, platform.NewError(http.StatusForbidden, "role is inactive")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(input.Password)); err != nil {
		return SessionResponse{}, platform.NewError(http.StatusUnauthorized, "invalid credentials")
	}

	permissions, err := s.loadPermissions(ctx, row.ID)
	if err != nil {
		return SessionResponse{}, err
	}

	user := platform.AuthUser{
		ID:          row.ID,
		Name:        row.Name,
		Email:       row.Email,
		RoleKey:     row.RoleKey,
		RoleName:    row.RoleName,
		Portal:      row.Portal,
		Scope:       row.Scope,
		BranchID:    platform.StringPointer(row.BranchID),
		BranchCode:  platform.StringPointer(row.BranchCode),
		BranchName:  platform.StringPointer(row.BranchName),
		Permissions: permissions,
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, row.ID); err != nil {
		return SessionResponse{}, platform.WrapError(http.StatusInternalServerError, "failed to update login timestamp", err)
	}

	token, err := s.signToken(user)
	if err != nil {
		return SessionResponse{}, platform.WrapError(http.StatusInternalServerError, "failed to sign token", err)
	}

	return SessionResponse{
		Token:      token,
		User:       user,
		Navigation: navigationFor(user),
		HomePath:   homePathFor(user),
	}, nil
}

func (s *Service) loadPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.permission_key
		FROM users u
		INNER JOIN role_permissions rp ON rp.role_id = u.role_id
		INNER JOIN permissions p ON p.id = rp.permission_id
		WHERE u.id = $1
		ORDER BY p.permission_key
	`, userID)
	if err != nil {
		return nil, platform.WrapError(http.StatusInternalServerError, "failed to load permissions", err)
	}
	defer rows.Close()

	permissions := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		permissions = append(permissions, value)
	}
	return permissions, rows.Err()
}

func (s *Service) signToken(user platform.AuthUser) (string, error) {
	claims := &httpmiddleware.Claims{
		UserID:      user.ID,
		Name:        user.Name,
		Email:       user.Email,
		RoleKey:     user.RoleKey,
		RoleName:    user.RoleName,
		Portal:      user.Portal,
		Scope:       user.Scope,
		BranchID:    user.BranchID,
		BranchCode:  user.BranchCode,
		BranchName:  user.BranchName,
		Permissions: user.Permissions,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(12 * time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "pharmacy-erp",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

// navigationFor is permission-driven (D11): a role's nav is derived from its
// actual permission set rather than a hardcoded role_key switch, so any
// number of back-office role presets (super_admin, admin, office,
// branch_head, ...) each see exactly the subset their permissions unlock.
// The *shape* of the nav (POS flat bar vs back-office grouped sidebar) still
// follows Portal, since that also decides which app shell renders it
// (AppShell branches the same way).
func navigationFor(user platform.AuthUser) []map[string]any {
	appendItem := func(items []map[string]any, key string, title string, href string, description string) []map[string]any {
		return append(items, map[string]any{
			"key":         key,
			"title":       title,
			"href":        href,
			"description": description,
		})
	}
	// appendItemIf only adds the item when the user holds the permission(s)
	// that gate its page, so nav membership always matches what the user can
	// actually open (including via direct URL — see requirePermission()).
	appendItemIf := func(items []map[string]any, allowed bool, key string, title string, href string, description string) []map[string]any {
		if !allowed {
			return items
		}
		return appendItem(items, key, title, href, description)
	}
	// appendGroup adds a parent/group menu item (C1) whose href is its first
	// child's href, so clicking the parent itself auto-activates that child.
	// Omitted entirely when it would have no visible children.
	appendGroup := func(items []map[string]any, key string, title string, description string, children []map[string]any) []map[string]any {
		if len(children) == 0 {
			return items
		}
		href, _ := children[0]["href"].(string)
		return append(items, map[string]any{
			"key":         key,
			"title":       title,
			"href":        href,
			"description": description,
			"children":    children,
		})
	}
	has := func(keys ...string) bool {
		for _, key := range keys {
			if platform.HasPermission(user, key) {
				return true
			}
		}
		return false
	}

	items := []map[string]any{}

	if user.Portal == "pos" {
		items = appendItemIf(items, has("invoice.create.pos"), "pos_screen", "ขายหน้าร้าน", "/sales", "ขายสินค้าและออกใบเสร็จ")
		// พักบิล needs no permission — parking a cart is ordinary counter
		// behaviour, and branch_pos deliberately holds no document rights.
		items = appendItem(items, "parked_bills", "พักบิล", "/parked-bills", "บิลที่พักไว้ รอกลับมาชำระเงิน")
		items = appendItemIf(items, has("invoice.view"), "sales_history", "ประวัติ", "/sales-history", "ดูและพิมพ์ใบขายย้อนหลัง")
		items = appendItemIf(items, has("inventory.view.branch"), "inventory_check", "เช็กสต๊อก", "/inventory-check", "ค้นหาสต๊อกของสาขา")
		items = appendItemIf(items, has("transfer.receive"), "goods_transfer_receipt", "รับโอนสินค้า", "/transfer-receipts", "ตรวจจำนวนที่ส่งและยืนยันจำนวนสินค้าที่ได้รับจริง")
		items = appendItemIf(items, has("dashboard.view.self"), "daily_sales_summary", "สรุปยอดขาย", "/daily-sales", "ยอดขายและยอดรับชำระประจำวัน")
		return items
	}

	var reportsChildren []map[string]any
	reportsChildren = appendItemIf(reportsChildren, has("dashboard.view.global"), "dashboard", "Dashboard", "/dashboard", "ภาพรวมยอดขายและสต๊อกทุกสาขา")
	reportsChildren = appendItemIf(reportsChildren, has("reports.generate.global"), "generate_report", "Generate Report", "/generate-report", "สร้างและปักหมุดรายงานแบบกำหนดเองจากข้อมูลทุกสาขา")
	reportsChildren = appendItemIf(reportsChildren, user.RoleKey == "super_admin" && has("month_end.view", "month_end.manage"), "month_end", "สรุปสิ้นเดือน", "/month-end", "ซ่อนบิลที่เข้าเงื่อนไข ส่ง Real คืน WH และตัด Ghost แบบตรวจสอบย้อนหลังได้")
	// This comparison exposes hidden invoices and Ghost Stock deductions, so the
	// literal superadmin role is required in addition to the report permission.
	reportsChildren = appendItemIf(reportsChildren, user.RoleKey == "super_admin" && has("reports.view.global", "reports.generate.global"), "month_end_report", "รายงานสรุปสิ้นเดือน", "/month-end-report", "เปรียบเทียบบิล ราคา และการตัดสต๊อกก่อนกับหลังปิดรอบ")
	// สรุปยอดขาย is intentionally absent from the back-office nav: รายงานสรุปสิ้นเดือน
	// covers the same ground for a global user in more detail. The POS portal
	// keeps its own สรุปยอดขาย, which is that cashier's till for the day.

	var inventoryChildren []map[string]any
	inventoryChildren = appendItemIf(inventoryChildren, has("products.view", "products.manage"), "product_catalog", "รายการสินค้า", "/product-catalog", "แหล่งข้อมูลสินค้าเดียวที่ทุกสาขาดึงไปใช้")
	inventoryChildren = appendItemIf(inventoryChildren, has("inventory.manage.global"), "real_inventory", "สต๊อกจริง", "/real-inventory", "ดู รับเข้า และปรับยอดสต๊อกจริง")
	inventoryChildren = appendItemIf(inventoryChildren, user.RoleKey == "super_admin" && has("inventory.ghost.manage"), "ghost_inventory", "สต๊อกผี", "/ghost-inventory", "ดู รับเข้า และปรับยอดสต๊อกผี")
	inventoryChildren = appendItemIf(inventoryChildren, has("products.manage"), "product_categories", "หมวดสินค้า", "/product-categories", "จัดกลุ่มสินค้าและกำหนดสีสำหรับการค้นหา")
	inventoryChildren = appendItemIf(inventoryChildren, has("promotion.manage"), "promotions", "โปรโมชั่น", "/promotions", "ส่วนลด ของแถม และราคาชุดที่หน้าร้านใช้อัตโนมัติ")
	inventoryChildren = appendItemIf(inventoryChildren, user.Scope == "global" && has("inventory.view.branch"), "inventory_check", "เช็กสต๊อก", "/inventory-check", "ค้นหาสต๊อกและดูสินค้าที่ถึงจุดแจ้งเตือน")
	inventoryChildren = appendItemIf(inventoryChildren, has("transfer.approve"), "stock_transfers", "โอนสินค้า", "/transfers", "สร้างใบโอนและตรวจสอบคำขอสินค้าจากสาขา")

	var documentsChildren []map[string]any
	documentsChildren = appendItemIf(documentsChildren, has("purchase_orders.view.global", "purchase_orders.manage.global"), "purchase_orders", "ใบสั่งซื้อเข้า", "/purchase-orders", "ประวัติและสร้างใบสั่งซื้อพร้อมรับสินค้าเข้าคลัง")
	documentsChildren = appendItemIf(documentsChildren, has("suppliers.view.global", "suppliers.manage.global"), "suppliers", "บริษัทคู่ค้า", "/suppliers", "จัดการบริษัทคู่ค้าส่วนกลาง")
	documentsChildren = appendItemIf(documentsChildren, has("quotation.manage"), "government_sales", "รพ.สต.", "/government-sales", "ใบเสนอราคาและใบขายสำหรับงานราชการ")
	documentsChildren = appendItemIf(documentsChildren, has("quotation.manage"), "sales_management", "ใบขาย", "/sales-management", "ใบเสนอราคา ใบขาย และประวัติเอกสาร")
	documentsChildren = appendItemIf(documentsChildren, has("returns.manage"), "claims", "เคลม/คืนสินค้า", "/claims", "ส่งเคลมให้คู่ค้าและปิดเคลมรับรุ่นเดิมหรือรุ่นทดแทน")
	documentsChildren = appendItemIf(documentsChildren, has("fda.manage"), "fda_reports", "อย.", "/fda-reports", "เลือกสินค้าและสร้างเอกสารนำส่ง อย.")

	items = appendGroup(items, "reports_group", "รายงาน", "รายงานสรุปและแดชบอร์ด", reportsChildren)
	items = appendGroup(items, "inventory_group", "คลังสินค้า", "สต๊อกจริง สต๊อกผี หมวดสินค้า และการโอนสินค้า", inventoryChildren)
	items = appendGroup(items, "documents_group", "ใบเอกสาร", "ใบสั่งซื้อ บริษัทคู่ค้า รพ.สต. ใบขาย และเอกสาร อย.", documentsChildren)

	// branch_ops_group: a branch-scoped back-office role (e.g. หัวหน้าสาขา)
	// doesn't hold any of the *.global permissions the groups above gate on,
	// but does need somewhere to reach their own branch's operations —
	// reusing the same pages the POS portal links to, under requirePermission()
	// gates a branch_head plausibly holds. Never shown for scope=="global"
	// roles, so super_admin/admin/office's nav is unchanged by this.
	if user.Scope == "branch" {
		var branchOpsChildren []map[string]any
		branchOpsChildren = appendItemIf(branchOpsChildren, has("dashboard.view.self"), "daily_sales_summary", "สรุปยอดขาย", "/daily-sales", "ยอดขายและยอดรับชำระประจำวันของสาขา")
		// ประวัติการขาย lives in the ระบบ group now — not repeated here.
		branchOpsChildren = appendItemIf(branchOpsChildren, has("inventory.view.branch"), "inventory_check", "เช็กสต๊อก", "/inventory-check", "ค้นหาสต๊อกของสาขา")
		branchOpsChildren = appendItemIf(branchOpsChildren, has("transfer.receive"), "goods_transfer_receipt", "รับโอนสินค้า", "/transfer-receipts", "ตรวจจำนวนที่ส่งและยืนยันจำนวนสินค้าที่ได้รับจริง")
		items = appendGroup(items, "branch_ops_group", "สาขาของฉัน", "ยอดขาย สต๊อก และการโอนสินค้าของสาขาที่ดูแล", branchOpsChildren)
	}

	// global_reports ("รายงาน" ภาษีและกำไรขาดทุนทุกสาขา) is removed from
	// the menu per C1 — /global-reports itself still works, just unlinked.

	// เมนู: ระบบ — ตั้งค่า, ประวัติระบบ, ประวัติการขาย as siblings
	// (business-flow.md). ประวัติระบบ used to be a Settings tab and
	// ประวัติการขาย only existed on the POS portal; both are now top-level
	// here for back-office roles.
	var systemChildren []map[string]any
	systemChildren = appendItemIf(systemChildren, has("settings.manage", "users.manage"), "settings", "ตั้งค่า", "/settings", "สาขา ผู้ใช้ บทบาทและสิทธิ์ เลขที่เอกสาร และตลาดออนไลน์")
	systemChildren = appendItemIf(systemChildren, has("audit.view.global"), "audit", "ประวัติระบบ", "/audit", "ประวัติการทำงานทุกอย่างของระบบ")
	systemChildren = appendItemIf(systemChildren, has("invoice.view"), "sales_history", "ประวัติการขาย", "/sales-history", "บิลใบเสร็จและบิลคืนสินค้าย้อนหลัง")
	items = appendGroup(items, "system_group", "ระบบ", "ตั้งค่าระบบ ประวัติการทำงาน และประวัติการขาย", systemChildren)

	return items
}

func homePathFor(user platform.AuthUser) string {
	switch {
	case user.Portal == "pos":
		return "/sales"
	case platform.HasPermission(user, "dashboard.view.global"):
		return "/dashboard"
	case platform.HasPermission(user, "dashboard.view.self"):
		return "/daily-sales"
	case platform.HasPermission(user, "reports.generate.global"):
		return "/generate-report"
	case platform.HasPermission(user, "invoice.view"):
		return "/sales-history"
	case platform.HasPermission(user, "inventory.view.branch"):
		return "/inventory-check"
	default:
		// No permission-appropriate landing page found (misconfigured role) —
		// fall back to the previous default rather than leaving this unset.
		return "/dashboard"
	}
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Login(c echo.Context) error {
	var input LoginRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	result, err := h.service.Login(c.Request().Context(), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Logout(c echo.Context) error {
	return platform.JSONMessage(c, http.StatusOK, "ออกจากระบบแล้ว")
}

func (h *Handler) Me(c echo.Context) error {
	user := platform.CurrentUser(c)
	return platform.JSON(c, http.StatusOK, map[string]any{
		"user":       user,
		"navigation": navigationFor(user),
		"home_path":  homePathFor(user),
	})
}
