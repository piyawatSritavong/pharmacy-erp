package users

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type RoleInput struct {
	RoleKey string `json:"role_key"`
	Name    string `json:"name"`
	Active  *bool  `json:"active"`
}

type RolePermissionsInput struct {
	PermissionKeys []string `json:"permission_keys"`
}

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
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.role_key, r.name, r.active, r.is_system,
		       COALESCE(string_agg(p.permission_key, ',' ORDER BY p.permission_key), '')
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		GROUP BY r.id, r.role_key, r.name, r.active, r.is_system
		ORDER BY r.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, roleKey, name, permissionCSV string
		var active, isSystem bool
		if err := rows.Scan(&id, &roleKey, &name, &active, &isSystem, &permissionCSV); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":          id,
			"role_key":    roleKey,
			"name":        name,
			"active":      active,
			"is_system":   isSystem,
			"permissions": splitCSV(permissionCSV),
		})
	}
	return items, rows.Err()
}

func (s *Service) ListPermissions(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, permission_key, name, description
		FROM permissions
		ORDER BY permission_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, key, name, description string
		if err := rows.Scan(&id, &key, &name, &description); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":             id,
			"permission_key": key,
			"name":           name,
			"description":    description,
		})
	}
	return items, rows.Err()
}

func (s *Service) CreateRole(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input RoleInput) (string, error) {
	roleKey := normalizeRoleKey(input.RoleKey)
	name := strings.TrimSpace(input.Name)
	if roleKey == "" || name == "" {
		return "", platform.NewError(http.StatusBadRequest, "role key and name are required")
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	roleID := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO roles (id, role_key, name, active, is_system, created_at, updated_at)
			VALUES ($1, $2, $3, $4, FALSE, NOW(), NOW())
		`, roleID, roleKey, name, active); err != nil {
			return err
		}
		meta.EntityType = "role"
		meta.EntityID = &roleID
		meta.Action = "role.create"
		meta.After = map[string]any{"role_key": roleKey, "name": name, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
	return roleID, err
}

func (s *Service) UpdateRole(ctx context.Context, roleID string, user platform.AuthUser, meta audit.LogEntry, input RoleInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return platform.NewError(http.StatusBadRequest, "role name is required")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		var currentActive bool
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(r)::text, active
			FROM (
				SELECT role_key, name, active, is_system
				FROM roles
				WHERE id = $1
			) r
		`, roleID).Scan(&beforeJSON, &currentActive); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "role not found")
			}
			return err
		}
		active := currentActive
		if input.Active != nil {
			active = *input.Active
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE roles
			SET name = $2, active = $3, updated_at = NOW()
			WHERE id = $1
		`, roleID, name, active); err != nil {
			return err
		}
		meta.EntityType = "role"
		meta.EntityID = &roleID
		meta.Action = "role.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"name": name, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) UpdateRolePermissions(ctx context.Context, roleID string, user platform.AuthUser, meta audit.LogEntry, input RolePermissionsInput) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1)`, roleID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return platform.NewError(http.StatusNotFound, "role not found")
		}

		permissionIDs := make([]string, 0, len(input.PermissionKeys))
		for _, key := range input.PermissionKeys {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			var permissionID string
			if err := tx.QueryRowContext(ctx, `SELECT id::text FROM permissions WHERE permission_key = $1`, trimmed).Scan(&permissionID); err != nil {
				if err == sql.ErrNoRows {
					return platform.NewError(http.StatusBadRequest, "unknown permission key: "+trimmed)
				}
				return err
			}
			permissionIDs = append(permissionIDs, permissionID)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			return err
		}
		for _, permissionID := range permissionIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO role_permissions (role_id, permission_id, created_at)
				VALUES ($1, $2, NOW())
			`, roleID, permissionID); err != nil {
				return err
			}
		}
		meta.EntityType = "role"
		meta.EntityID = &roleID
		meta.Action = "role.permissions.update"
		meta.After = map[string]any{"permission_keys": input.PermissionKeys}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) CreateUser(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input UserInput) (string, error) {
	fullName := strings.TrimSpace(input.FullName)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if fullName == "" || email == "" || strings.TrimSpace(input.Password) == "" || strings.TrimSpace(input.RoleID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "full_name, email, password, and role_id are required")
	}
	var roleKey string
	if err := s.db.QueryRowContext(ctx, `SELECT role_key FROM roles WHERE id = $1`, input.RoleID).Scan(&roleKey); err != nil {
		if err == sql.ErrNoRows {
			return "", platform.NewError(http.StatusBadRequest, "role not found")
		}
		return "", err
	}
	if roleKey == "super_admin" && input.BranchID != nil && strings.TrimSpace(*input.BranchID) != "" {
		return "", platform.NewError(http.StatusBadRequest, "super_admin cannot be assigned to a branch")
	}
	if roleKey != "super_admin" && (input.BranchID == nil || strings.TrimSpace(*input.BranchID) == "") {
		return "", platform.NewError(http.StatusBadRequest, "branch_id is required for branch roles")
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
			return platform.MapUniqueViolation(err, "email already exists")
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
		return platform.NewError(http.StatusBadRequest, "full_name, email, and role_id are required")
	}
	var roleKey string
	if err := s.db.QueryRowContext(ctx, `SELECT role_key FROM roles WHERE id = $1`, input.RoleID).Scan(&roleKey); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusBadRequest, "role not found")
		}
		return err
	}
	if roleKey == "super_admin" && input.BranchID != nil && strings.TrimSpace(*input.BranchID) != "" {
		return platform.NewError(http.StatusBadRequest, "super_admin cannot be assigned to a branch")
	}
	if roleKey != "super_admin" && (input.BranchID == nil || strings.TrimSpace(*input.BranchID) == "") {
		return platform.NewError(http.StatusBadRequest, "branch_id is required for branch roles")
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
			return platform.MapUniqueViolation(err, "email already exists")
		}
		meta.EntityType = "user"
		meta.EntityID = &userID
		meta.Action = "user.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"full_name": fullName, "email": email, "role_id": input.RoleID, "branch_id": input.BranchID, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) ResetPassword(ctx context.Context, userID string, user platform.AuthUser, meta audit.LogEntry, input ResetPasswordRequest) error {
	if strings.TrimSpace(input.Password) == "" {
		return platform.NewError(http.StatusBadRequest, "password is required")
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

func splitCSV(raw string) []string {
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, ",")
}

func normalizeRoleKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	return key
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
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "user created"})
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
	return platform.JSONMessage(c, http.StatusOK, "user updated")
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
	return platform.JSONMessage(c, http.StatusOK, "password reset")
}

func (h *Handler) ListRoles(c echo.Context) error {
	items, err := h.service.ListRoles(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load roles", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateRole(c echo.Context) error {
	var input RoleInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.CreateRole(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "role created"})
}

func (h *Handler) UpdateRole(c echo.Context) error {
	var input RoleInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpdateRole(c.Request().Context(), c.Param("roleID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "role updated")
}

func (h *Handler) UpdateRolePermissions(c echo.Context) error {
	var input RolePermissionsInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpdateRolePermissions(c.Request().Context(), c.Param("roleID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "role permissions updated")
}

func (h *Handler) ListPermissions(c echo.Context) error {
	items, err := h.service.ListPermissions(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load permissions", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}
