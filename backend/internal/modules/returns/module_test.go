package returns

import (
	"net/http"
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func TestReturnsRejectGhostForEveryRole(t *testing.T) {
	for _, test := range []struct {
		role string
		code int
	}{
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
		{role: "super_admin", code: http.StatusBadRequest},
	} {
		err := validateReturnVisibility(platform.AuthUser{RoleKey: test.role}, "branch", "ghost")
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}
