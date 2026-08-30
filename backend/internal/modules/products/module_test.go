package products

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

func TestListRejectsCrossBranchQuery(t *testing.T) {
	engine := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/products?branch_id=branch-b", nil)
	recorder := httptest.NewRecorder()
	context := engine.NewContext(request, recorder)
	context.Set(platform.ContextUserKey, platform.AuthUser{
		RoleKey:  "branch_pos",
		BranchID: ptr("branch-a"),
	})

	handler := NewHandler(nil)
	if err := handler.List(context); err != nil {
		t.Fatalf("expected handler to write forbidden response without bubbling error: %v", err)
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden status, got %d", recorder.Code)
	}
}

func ptr(value string) *string {
	return &value
}
