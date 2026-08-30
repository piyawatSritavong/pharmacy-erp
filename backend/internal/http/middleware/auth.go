package middleware

import (
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
	jwt.StandardClaims
}

func JWT(secret string) echo.MiddlewareFunc {
	return echoMiddleware.JWTWithConfig(echoMiddleware.JWTConfig{
		SigningKey:  []byte(secret),
		TokenLookup: "header:Authorization:Bearer ",
		ContextKey:  "jwt_token",
		Claims:      &Claims{},
		SuccessHandler: func(c echo.Context) {
			token := c.Get("jwt_token").(*jwt.Token)
			claims := token.Claims.(*Claims)
			c.Set(platform.ContextUserKey, platform.AuthUser{
				ID:          claims.UserID,
				Name:        claims.Name,
				Email:       claims.Email,
				RoleKey:     claims.RoleKey,
				RoleName:    claims.RoleName,
				Portal:      claims.Portal,
				Scope:       claims.Scope,
				BranchID:    claims.BranchID,
				BranchCode:  claims.BranchCode,
				BranchName:  claims.BranchName,
				Permissions: claims.Permissions,
			})
		},
		ErrorHandlerWithContext: func(err error, c echo.Context) error {
			return c.JSON(http.StatusUnauthorized, platform.ErrorResponse{Message: "unauthorized"})
		},
	})
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
