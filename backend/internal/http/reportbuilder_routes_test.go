package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/config"
	appMiddleware "pharmacy-erp/backend/internal/http/middleware"

	"github.com/golang-jwt/jwt"
)

func reportBuilderToken(t *testing.T, secret string, permissions []string) string {
	t.Helper()
	claims := &appMiddleware.Claims{
		UserID:      "11111111-1111-4111-8111-111111111111",
		Name:        "Test User",
		Email:       "test@example.com",
		RoleKey:     "super_admin",
		RoleName:    "Super Admin",
		Permissions: permissions,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestReportBuilderCatalogRequiresGeneratePermission(t *testing.T) {
	const secret = "report-builder-route-test"
	server := NewServer(config.Config{JWTSecret: secret, FrontendURL: "http://localhost:3000"}, nil)

	for _, test := range []struct {
		name        string
		permissions []string
		wantStatus  int
	}{
		{name: "super admin permission", permissions: []string{"reports.generate.global"}, wantStatus: http.StatusOK},
		{name: "POS permissions", permissions: []string{"invoice.create.pos", "products.view"}, wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/report-builder/catalog", nil)
			request.Header.Set("Authorization", "Bearer "+reportBuilderToken(t, secret, test.permissions))
			server.engine.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d: %s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
			if recorder.Code == http.StatusOK {
				body := strings.ToLower(recorder.Body.String())
				for _, forbidden := range []string{"password_hash", "credentials_json", "raw_payload", "before_data", "after_data"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("catalog exposed %q", forbidden)
					}
				}
			}
		})
	}
}

func TestSCMRoutesRejectPOSPermissions(t *testing.T) {
	const secret = "scm-route-test"
	server := NewServer(config.Config{JWTSecret: secret, FrontendURL: "http://localhost:3000"}, nil)
	token := reportBuilderToken(t, secret, []string{"invoice.create.pos", "inventory.view.branch", "products.view"})
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/suppliers"},
		{http.MethodPost, "/api/v1/suppliers"},
		{http.MethodGet, "/api/v1/suppliers/11111111-1111-4111-8111-111111111111"},
		{http.MethodPut, "/api/v1/suppliers/11111111-1111-4111-8111-111111111111"},
		{http.MethodDelete, "/api/v1/suppliers/11111111-1111-4111-8111-111111111111"},
		{http.MethodGet, "/api/v1/purchase-orders"},
		{http.MethodPost, "/api/v1/purchase-orders"},
		{http.MethodGet, "/api/v1/purchase-orders/product-options"},
		{http.MethodGet, "/api/v1/purchase-orders/11111111-1111-4111-8111-111111111111"},
		{http.MethodPut, "/api/v1/purchase-orders/11111111-1111-4111-8111-111111111111"},
		{http.MethodPost, "/api/v1/purchase-orders/11111111-1111-4111-8111-111111111111/cancel"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			server.engine.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
