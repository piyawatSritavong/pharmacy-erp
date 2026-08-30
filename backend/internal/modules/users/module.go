package users

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type UserInput struct {
	FullName string  `json:"full_name"`
	Email    string  `json:"email"`
	Password string  `json:"password"`
	RoleID   string  `json:"role_id"`
	BranchID *string `json:"branch_id"`
	Active   *bool   `json:"active"`
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) ListUsers(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.full_name, u.email, u.active, COALESCE(u.last_login_at, NULL),
		       r.role_key, r.name, COALESCE(b.id::text, ''), COALESCE(b.name, '')
		FROM users u
		INNER JOIN roles r ON r.id = u.role_id
		LEFT JOIN branches b ON b.id = u.branch_id
		ORDER BY u.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, name, email, roleKey, roleName, branchID, branchName string
		var active bool
		var lastLoginAt sql.NullTime
		if err := rows.Scan(&id, &name, &email, &active, &lastLoginAt, &roleKey, &roleName, &branchID, &branchName); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":        id,
			"name":      name,
			"email":     email,
			"active":    active,
			"role_key":  roleKey,
			"role_name": roleName,
		}
		if lastLoginAt.Valid {
			item["last_login_at"] = lastLoginAt.Time
		}
		if branchID != "" {
			item["branch_id"] = branchID
			item["branch_name"] = branchName
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ListRoles(ctx context.Context) ([]map[string]any, error) {
	// D11: no more hardcoded role_key filter — every role preset (fixed by
	// design, not user-creatable) shows up here once seeded by migration.
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.role_key, r.name, r.active, r.is_system, r.portal, r.scope,
		       COALESCE(string_agg(p.permission_key, ',' ORDER BY p.permission_key), '')
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		GROUP BY r.id, r.role_key, r.name, r.active, r.is_system, r.portal, r.scope
		ORDER BY r.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, roleKey, name, portal, scope, permissionCSV string
		var active, isSystem bool
		if err := rows.Scan(&id, &roleKey, &name, &active, &isSystem, &portal, &scope, &permissionCSV); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":          id,
			"role_key":    roleKey,
			"name":        name,
			"active":      active,
			"is_system":   isSystem,
			"portal":      portal,
			"scope":       scope,
			"permissions": splitCSV(permissionCSV),
		})
	}
	return items, rows.Err()
}

// ListPermissions returns the full permission catalog (key/name/description)
// for the D11 role-permission checkbox editor.
func (s *Service) ListPermissions(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT permission_key, name, description
		FROM permissions
		ORDER BY permission_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var key, name, description string
		if err := rows.Scan(&key, &name, &description); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"permission_key": key,
			"name":           name,
			"description":    description,
		})
	}
	return items, rows.Err()
}

// UpdateRolePermissions replaces a role's permission set (D11's "Permissions
// checkbox set" editor). is_system roles (super_admin, branch_pos) are
// refused — super_admin's set must always cover "everything except POS", and
// branch_pos's set is tightly coupled to the POS frontend's assumptions, so
// neither is safe to edit from this generic endpoint.
func (s *Service) UpdateRolePermissions(ctx context.Context, roleID string, user platform.AuthUser, meta audit.LogEntry, permissionKeys []string) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var roleKey, name string
		var isSystem bool
		if err := tx.QueryRowContext(ctx, `
			SELECT role_key, name, is_system FROM roles WHERE id = $1 FOR UPDATE
		`, roleID).Scan(&roleKey, &name, &isSystem); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบบทบาทที่เลือก")
			}
			return err
		}
		if isSystem {
			return platform.NewError(http.StatusBadRequest, "ไม่สามารถแก้ไขสิทธิ์ของบทบาทหลักของระบบ")
		}

		cleaned := make([]string, 0, len(permissionKeys))
		seen := map[string]bool{}
		for _, key := range permissionKeys {
			key = strings.TrimSpace(key)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			cleaned = append(cleaned, key)
		}

		var beforeCSV string
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(string_agg(p.permission_key, ',' ORDER BY p.permission_key), '')
			FROM role_permissions rp INNER JOIN permissions p ON p.id = rp.permission_id
			WHERE rp.role_id = $1
		`, roleID).Scan(&beforeCSV); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			return err
		}
		if len(cleaned) > 0 {
			var matched int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM permissions WHERE permission_key = ANY($1)
			`, pq.Array(cleaned)).Scan(&matched); err != nil {
				return err
			}
			if matched != len(cleaned) {
				return platform.NewError(http.StatusBadRequest, "พบรหัสสิทธิ์ที่ไม่ถูกต้องในรายการที่ส่งมา")
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO role_permissions (role_id, permission_id, created_at)
				SELECT $1, id, NOW() FROM permissions WHERE permission_key = ANY($2)
			`, roleID, pq.Array(cleaned)); err != nil {
				return err
			}
		}

		meta.EntityType = "role"
		meta.EntityID = &roleID
		meta.Action = "role.permissions.update"
		meta.Before = map[string]any{"permissions": splitCSV(beforeCSV)}
		meta.After = map[string]any{"permissions": cleaned}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) CreateUser(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input UserInput) (string, error) {
	fullName := strings.TrimSpace(input.FullName)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if fullName == "" || email == "" || strings.TrimSpace(input.Password) == "" || strings.TrimSpace(input.RoleID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อ อีเมล รหัสผ่าน และบทบาท")
	}
	if err := s.ensureSuperadminRoleAllowed(ctx, user, input.RoleID); err != nil {
		return "", err
	}
	if err := s.validateRoleAssignment(ctx, input.RoleID, input.BranchID); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	userID := platform.MustUUID()
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		`, userID, input.RoleID, platform.NullUUID(input.BranchID), fullName, email, string(hash), active); err != nil {
			return platform.MapUniqueViolation(err, "อีเมลนี้มีผู้ใช้งานแล้ว")
		}
		meta.EntityType = "user"
		meta.EntityID = &userID
		meta.Action = "user.create"
		meta.After = map[string]any{"full_name": fullName, "email": email, "role_id": input.RoleID, "branch_id": input.BranchID, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
	return userID, err
}

func (s *Service) UpdateUser(ctx context.Context, userID string, user platform.AuthUser, meta audit.LogEntry, input UserInput) error {
	fullName := strings.TrimSpace(input.FullName)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if fullName == "" || email == "" || strings.TrimSpace(input.RoleID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อ อีเมล และบทบาท")
	}
	if err := s.ensureSuperadminUserAllowed(ctx, user, userID); err != nil {
		return err
	}
	if err := s.ensureSuperadminRoleAllowed(ctx, user, input.RoleID); err != nil {
		return err
	}
	if err := s.validateRoleAssignment(ctx, input.RoleID, input.BranchID); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		var currentActive bool
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(u)::text, active
			FROM (
				SELECT full_name, email, role_id::text, branch_id::text, active
				FROM users
				WHERE id = $1
			) u
		`, userID).Scan(&beforeJSON, &currentActive); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "user not found")
			}
			return err
		}

		active := currentActive
		if input.Active != nil {
			active = *input.Active
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET role_id = $2, branch_id = $3, full_name = $4, email = $5, active = $6, updated_at = NOW()
			WHERE id = $1
		`, userID, input.RoleID, platform.NullUUID(input.BranchID), fullName, email, active); err != nil {
			return platform.MapUniqueViolation(err, "อีเมลนี้มีผู้ใช้งานแล้ว")
		}
		meta.EntityType = "user"
		meta.EntityID = &userID
		meta.Action = "user.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"full_name": fullName, "email": email, "role_id": input.RoleID, "branch_id": input.BranchID, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
}

// validateRoleAssignment replaces the old hardcoded "only super_admin or
// branch_pos" check (D11): any active role now works, and the branch_id
// requirement follows the role's scope (global roles must not have a
// branch_id; branch-scoped roles must). The sales_enabled branch check only
// applies to portal=="pos" roles — a back-office branch-scoped role (e.g.
// หัวหน้าสาขา) can be assigned to a warehouse-only branch that doesn't sell.
func (s *Service) validateRoleAssignment(ctx context.Context, roleID string, branchID *string) error {
	var roleName, portal, scope string
	var active bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT name, portal, scope, active FROM roles WHERE id = $1
	`, roleID).Scan(&roleName, &portal, &scope, &active); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusBadRequest, "ไม่พบบทบาทที่เลือก")
		}
		return err
	}
	if !active {
		return platform.NewError(http.StatusBadRequest, "บทบาทนี้ถูกปิดใช้งานแล้ว")
	}
	hasBranch := branchID != nil && strings.TrimSpace(*branchID) != ""
	if scope == "global" && hasBranch {
		return platform.NewError(http.StatusBadRequest, roleName+"ไม่ต้องผูกกับสาขา")
	}
	if scope == "branch" && !hasBranch {
		return platform.NewError(http.StatusBadRequest, roleName+"ต้องเลือกสาขา")
	}
	if portal == "pos" && hasBranch {
		if err := s.validatePOSBranch(ctx, *branchID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validatePOSBranch(ctx context.Context, branchID string) error {
	var salesEnabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT sales_enabled FROM branches WHERE id=$1 AND active=TRUE`, branchID).Scan(&salesEnabled); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เปิดใช้งาน")
		}
		return err
	}
	if !salesEnabled {
		return platform.NewError(http.StatusBadRequest, "ไม่สามารถสร้างบัญชี POS ให้โกดังที่ไม่เปิดขาย")
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, userID string, user platform.AuthUser, meta audit.LogEntry, input ResetPasswordRequest) error {
	if strings.TrimSpace(input.Password) == "" {
		return platform.NewError(http.StatusBadRequest, "password is required")
	}
	if err := s.ensureSuperadminUserAllowed(ctx, user, userID); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return platform.NewError(http.StatusNotFound, "user not found")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, userID, string(hash)); err != nil {
			return err
		}
		meta.EntityType = "user"
		meta.EntityID = &userID
		meta.Action = "user.password.reset"
		meta.After = map[string]any{"reset": true}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) DeletionImpact(ctx context.Context, userID string) (map[string]any, error) {
	var name, email, roleKey string
	if err := s.db.QueryRowContext(ctx, `
		SELECT u.full_name, u.email, r.role_key
		FROM users u INNER JOIN roles r ON r.id = u.role_id
		WHERE u.id = $1
	`, userID).Scan(&name, &email, &roleKey); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบผู้ใช้")
		}
		return nil, err
	}
	queries := map[string]string{
		"ใบเสนอราคา":       "SELECT COUNT(*) FROM quotations WHERE created_by = $1",
		"ใบขาย":            "SELECT COUNT(*) FROM invoices WHERE created_by = $1 AND deleted_at IS NULL",
		"รายการรับชำระ":    "SELECT COUNT(*) FROM invoice_payments WHERE created_by = $1",
		"การโอนสินค้า":     "SELECT COUNT(*) FROM transfers WHERE requested_by = $1 OR dispatched_by = $1 OR received_by = $1",
		"รายการเคลื่อนไหว": "SELECT COUNT(*) FROM inventory_movements WHERE performed_by = $1",
		"ประวัติการทำงาน":  "SELECT COUNT(*) FROM audit_logs WHERE actor_id = $1",
	}
	counts := map[string]int{}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return map[string]any{
		"id": userID, "name": name, "email": email, "role_key": roleKey,
		"counts": counts, "confirmation": "ลบ " + email,
	}, nil
}

func rebuildInventory(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE inventory i
		SET qty_real = COALESCE((
			SELECT SUM(m.quantity_delta) FROM inventory_movements m
			WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'real'
		), 0),
		qty_ghost = COALESCE((
			SELECT SUM(m.quantity_delta) FROM inventory_movements m
			WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'ghost'
		), 0),
		updated_at = NOW()
	`)
	return err
}

func (s *Service) DeleteUser(ctx context.Context, userID string, actor platform.AuthUser, meta audit.LogEntry, confirmation string) error {
	if actor.ID == userID {
		return platform.NewError(http.StatusBadRequest, "ไม่สามารถลบบัญชีที่กำลังใช้งานอยู่")
	}
	if err := s.ensureSuperadminUserAllowed(ctx, actor, userID); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var name, email, roleKey string
		if err := tx.QueryRowContext(ctx, `
			SELECT u.full_name, u.email, r.role_key
			FROM users u INNER JOIN roles r ON r.id = u.role_id
			WHERE u.id = $1 FOR UPDATE
		`, userID).Scan(&name, &email, &roleKey); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบผู้ใช้")
			}
			return err
		}
		if confirmation != "ลบ "+email {
			return platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
		}
		if roleKey == "super_admin" {
			var superCount int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM users u INNER JOIN roles r ON r.id = u.role_id
				WHERE r.role_key = 'super_admin'
			`).Scan(&superCount); err != nil {
				return err
			}
			if superCount <= 1 {
				return platform.NewError(http.StatusConflict, "ไม่สามารถลบผู้ดูแลระบบคนสุดท้าย")
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
			return err
		}
		if err := rebuildInventory(ctx, tx); err != nil {
			return err
		}
		meta.EntityType = "user"
		meta.EntityID = nil
		meta.Action = "user.delete"
		meta.After = map[string]any{"deleted_id": userID, "name": name, "email": email}
		return s.audit.Log(ctx, tx, meta)
	})
}

func splitCSV(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, ",")
}

type deleteRequest struct {
	Confirmation string `json:"confirmation"`
}

func (s *Service) ensureSuperadminRoleAllowed(ctx context.Context, actor platform.AuthUser, roleID string) error {
	if actor.RoleKey == "super_admin" {
		return nil
	}
	var roleKey string
	if err := s.db.QueryRowContext(ctx, `SELECT role_key FROM roles WHERE id=$1`, roleID).Scan(&roleKey); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusBadRequest, "ไม่พบบทบาทที่เลือก")
		}
		return err
	}
	if roleKey == "super_admin" {
		return platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้นที่จัดการบัญชีผู้ดูแลระบบสูงสุดได้")
	}
	return nil
}

func (s *Service) ensureSuperadminUserAllowed(ctx context.Context, actor platform.AuthUser, userID string) error {
	if actor.RoleKey == "super_admin" {
		return nil
	}
	var roleKey string
	if err := s.db.QueryRowContext(ctx, `SELECT r.role_key FROM users u INNER JOIN roles r ON r.id=u.role_id WHERE u.id=$1`, userID).Scan(&roleKey); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusNotFound, "user not found")
		}
		return err
	}
	if roleKey == "super_admin" {
		return platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้นที่จัดการบัญชีผู้ดูแลระบบสูงสุดได้")
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListUsers(c echo.Context) error {
	items, err := h.service.ListUsers(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load users", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateUser(c echo.Context) error {
	var input UserInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.CreateUser(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "เพิ่มผู้ใช้แล้ว"})
}

func (h *Handler) UpdateUser(c echo.Context) error {
	var input UserInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpdateUser(c.Request().Context(), c.Param("userID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกผู้ใช้แล้ว")
}

func (h *Handler) ResetPassword(c echo.Context) error {
	var input ResetPasswordRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.ResetPassword(c.Request().Context(), c.Param("userID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ตั้งรหัสผ่านใหม่แล้ว")
}

func (h *Handler) UserDeletionImpact(c echo.Context) error {
	impact, err := h.service.DeletionImpact(c.Request().Context(), c.Param("userID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) DeleteUser(c echo.Context) error {
	var input deleteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันการลบไม่ถูกต้อง"))
	}
	if err := h.service.DeleteUser(c.Request().Context(), c.Param("userID"), platform.CurrentUser(c), audit.MetaFromContext(c), input.Confirmation); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบผู้ใช้และข้อมูลที่เกี่ยวข้องแล้ว")
}

func (h *Handler) ListRoles(c echo.Context) error {
	items, err := h.service.ListRoles(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load roles", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListPermissions(c echo.Context) error {
	items, err := h.service.ListPermissions(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load permissions", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

type updateRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func (h *Handler) UpdateRolePermissions(c echo.Context) error {
	var input updateRolePermissionsRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpdateRolePermissions(c.Request().Context(), c.Param("roleID"), platform.CurrentUser(c), meta, input.Permissions); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกสิทธิ์ของบทบาทแล้ว")
}
