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
		BranchID     sql.NullString
		BranchCode   sql.NullString
		BranchName   sql.NullString
		Active       bool
	}

	var row userRow
	if err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.full_name, u.email, u.password_hash, r.role_key, r.name, r.active,
		       u.branch_id::text, b.code, b.name, u.active
		FROM users u
		INNER JOIN roles r ON r.id = u.role_id
		LEFT JOIN branches b ON b.id = u.branch_id
		WHERE LOWER(u.email) = LOWER($1)
	`, strings.TrimSpace(input.Email)).Scan(
		&row.ID, &row.Name, &row.Email, &row.PasswordHash, &row.RoleKey, &row.RoleName, &row.RoleActive,
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

func navigationFor(user platform.AuthUser) []map[string]any {
	appendItem := func(items []map[string]any, key string, title string, href string, description string) []map[string]any {
		return append(items, map[string]any{
			"key":         key,
			"title":       title,
			"href":        href,
			"description": description,
		})
	}

	items := []map[string]any{}
	switch user.RoleKey {
	case "super_admin":
		items = appendItem(items, "dashboard", "Dashboard", "/dashboard", "Global sales and stock overview")
		items = appendItem(items, "inventory_management", "Inventory Management", "/inventory-management", "Master products, aliases, and enterprise inventory")
		items = appendItem(items, "finance_central", "Finance Central", "/finance-central", "Central check clearing and outstanding invoices")
		items = appendItem(items, "global_reports", "Global Reports", "/global-reports", "Tax and profit/loss across all branches")
		items = appendItem(items, "settings", "Settings", "/settings", "Branches, users, roles, sequences, marketplace, and audit")
	case "branch_admin":
		items = appendItem(items, "branch_dashboard", "Branch Dashboard", "/branch-dashboard", "Branch-only sales and transfer overview")
		items = appendItem(items, "branch_inventory", "Branch Inventory", "/branch-inventory", "Branch stock, rebalance, transfer request, and dispatch queue")
		items = appendItem(items, "sales_invoices", "Sales & Invoices", "/sales-invoices", "Quotations, invoice issue, history, and reprint")
		items = appendItem(items, "local_finance", "Local Finance", "/local-finance", "Match branch invoices with checks")
	case "branch_pos":
		items = appendItem(items, "pos_screen", "POS Screen", "/sales", "Retail and government-mode billing")
		items = appendItem(items, "inventory_check", "Inventory Check", "/inventory-check", "Search branch stock without edit actions")
		items = appendItem(items, "goods_transfer_receipt", "Goods Transfer Receipt", "/transfer-receipts", "Receive transfers by QR camera or manual code")
		items = appendItem(items, "daily_sales_summary", "Daily Sales Summary", "/daily-sales", "Your daily sales and collections")
	default:
		items = appendItem(items, "dashboard", "Dashboard", "/dashboard", "Application overview")
	}

	return items
}

func homePathFor(user platform.AuthUser) string {
	switch user.RoleKey {
	case "branch_admin":
		return "/branch-dashboard"
	case "branch_pos":
		return "/sales"
	default:
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
	return platform.JSONMessage(c, http.StatusOK, "logged out")
}

func (h *Handler) Me(c echo.Context) error {
	user := platform.CurrentUser(c)
	return platform.JSON(c, http.StatusOK, map[string]any{
		"user":       user,
		"navigation": navigationFor(user),
		"home_path":  homePathFor(user),
	})
}
