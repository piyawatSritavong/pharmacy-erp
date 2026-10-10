package middleware

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/platform"

	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

type Claims struct {
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	RoleKey     string   `json:"role_key"`
	RoleName    string   `json:"role_name"`
	Portal      string   `json:"portal"`
	Scope       string   `json:"scope"`
	BranchID    *string  `json:"branch_id"`
	BranchCode  *string  `json:"branch_code"`
	BranchName  *string  `json:"branch_name"`
	Permissions []string `json:"permissions"`
	AuthVersion int      `json:"auth_version"`
	jwt.StandardClaims
}

func JWT(secret string, db *sql.DB) echo.MiddlewareFunc {
	verifySignature := echoMiddleware.JWTWithConfig(echoMiddleware.JWTConfig{
		SigningKey:  []byte(secret),
		TokenLookup: "header:Authorization:Bearer ,cookie:pharmacy_erp_auth",
		ContextKey:  "jwt_token",
		Claims:      &Claims{},
		ErrorHandlerWithContext: func(err error, c echo.Context) error {
			return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "unauthorized"})
		},
	})
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return verifySignature(func(c echo.Context) error {
			token, ok := c.Get("jwt_token").(*jwt.Token)
			if !ok {
				return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "unauthorized"})
			}
			claims, ok := token.Claims.(*Claims)
			if !ok {
				return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "unauthorized"})
			}
			user, err := currentSessionUser(c, db, claims.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "session expired"})
				}
				c.Logger().Errorf("load current session: %v", err)
				return c.JSON(http.StatusServiceUnavailable, platform.ErrorResponse{Message: "session service unavailable"})
			}
			if user.AuthVersion != claims.AuthVersion {
				return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "session expired"})
			}
			user.ScopeAudit = platform.BranchAuditFrom(c)
			c.Set(platform.ContextUserKey, user)
			return next(c)
		})
	}
}

func currentSessionUser(c echo.Context, db *sql.DB, userID string) (platform.AuthUser, error) {
	var user platform.AuthUser
	var branchID, branchCode, branchName sql.NullString
	var permissionCSV string
	var userActive, roleActive, branchActive bool
	err := db.QueryRowContext(c.Request().Context(), `
		SELECT u.id::text,u.full_name,u.email,u.auth_version,u.active,
		       r.role_key,r.name,r.portal,r.scope,r.active,
		       u.branch_id::text,b.code,b.name,COALESCE(b.active,TRUE),
		       COALESCE(string_agg(p.permission_key,',' ORDER BY p.permission_key),'')
		FROM users u
		INNER JOIN roles r ON r.id=u.role_id
		LEFT JOIN branches b ON b.id=u.branch_id
		LEFT JOIN role_permissions rp ON rp.role_id=r.id
		LEFT JOIN permissions p ON p.id=rp.permission_id
		WHERE u.id=$1
		GROUP BY u.id,r.id,b.id
	`, userID).Scan(
		&user.ID, &user.Name, &user.Email, &user.AuthVersion, &userActive,
		&user.RoleKey, &user.RoleName, &user.Portal, &user.Scope, &roleActive,
		&branchID, &branchCode, &branchName, &branchActive, &permissionCSV,
	)
	if err != nil {
		return platform.AuthUser{}, err
	}
	if !userActive || !roleActive || !branchActive {
		return platform.AuthUser{}, sql.ErrNoRows
	}
	user.BranchID = platform.StringPointer(branchID)
	user.BranchCode = platform.StringPointer(branchCode)
	user.BranchName = platform.StringPointer(branchName)
	if permissionCSV == "" {
		user.Permissions = []string{}
	} else {
		user.Permissions = strings.Split(permissionCSV, ",")
	}
	return user, nil
}

func RequireAnyPermission(keys ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := platform.CurrentUser(c)
			for _, key := range keys {
				if platform.HasPermission(user, key) {
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, platform.ErrorResponse{Message: "forbidden"})
		}
	}
}

// RequireRole is used for boundaries that must not be delegated through the
// editable permission system. Ghost stock and month-end reconciliation
// contain hidden invoice data, so possessing a similarly named permission is
// intentionally insufficient.
func RequireRole(roleKeys ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := platform.CurrentUser(c)
			for _, roleKey := range roleKeys {
				if user.RoleKey == roleKey {
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, platform.ErrorResponse{Message: "forbidden"})
		}
	}
}

func RequestInfo() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestID := c.Response().Header().Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = c.Request().Header.Get(echo.HeaderXRequestID)
			}
			c.Set(platform.ContextRequestIDKey, requestID)

			if auth := c.Request().Header.Get(echo.HeaderAuthorization); auth != "" && strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				c.Request().Header.Set(echo.HeaderAuthorization, auth)
			}

			return next(c)
		}
	}
}
