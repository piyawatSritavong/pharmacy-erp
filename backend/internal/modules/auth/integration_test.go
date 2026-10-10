package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"

	"github.com/labstack/echo/v4"
)

func TestLoginAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.OpenTest(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	cfg := config.Load()
	service := NewService(db, cfg)
	password := os.Getenv("SEED_POS_PASSWORD")
	if password == "" {
		t.Fatal("SEED_POS_PASSWORD is required for the login integration test")
	}
	result, err := service.Login(context.Background(), LoginRequest{
		Email: "pos.mes@erp.local", Password: password,
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.User.RoleKey != "branch_pos" || result.HomePath != "/sales" {
		t.Fatalf("unexpected POS session: %#v", result)
	}

	body, _ := json.Marshal(LoginRequest{Email: "pos.mes@erp.local", Password: password})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	context := echo.New().NewContext(request, recorder)
	if err := NewHandler(service).Login(context); err != nil {
		t.Fatalf("login handler: %v", err)
	}
	var browserPayload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &browserPayload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if _, exposed := browserPayload["token"]; exposed {
		t.Fatal("login response body exposed the JWT")
	}
	setCookie := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "pharmacy_erp_auth=") || !strings.Contains(strings.ToLower(setCookie), "httponly") {
		t.Fatalf("login did not issue an HttpOnly session cookie: %q", setCookie)
	}
}
