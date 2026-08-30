package platform

import (
	"net/http"
	"regexp"
	"testing"
)

func TestGenerateReadableCode(t *testing.T) {
	first := GenerateReadableCode("prd")
	second := GenerateReadableCode("prd")
	pattern := regexp.MustCompile(`^PRD-\d{8}-[A-F0-9]{6}$`)
	if !pattern.MatchString(first) {
		t.Fatalf("unexpected readable code format: %s", first)
	}
	if first == second {
		t.Fatalf("generated codes must be unique: %s", first)
	}
}

func TestEnforceGhostWritePolicy(t *testing.T) {
	if err := EnforceGhostWritePolicy(AuthUser{RoleKey: "central_admin"}, false); err != nil {
		t.Fatalf("real stock must remain writable: %v", err)
	}

	for _, test := range []struct {
		name string
		role string
		code int
	}{
		{name: "central admin", role: "central_admin", code: http.StatusForbidden},
		{name: "branch admin", role: "branch_admin", code: http.StatusForbidden},
		{name: "POS", role: "branch_pos", code: http.StatusForbidden},
		{name: "superadmin", role: "super_admin", code: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := EnforceGhostWritePolicy(AuthUser{RoleKey: test.role}, true)
			appErr, ok := err.(*AppError)
			if !ok || appErr.Code != test.code {
				t.Fatalf("expected AppError %d, got %#v", test.code, err)
			}
		})
	}
}
