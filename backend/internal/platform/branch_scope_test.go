package platform

import (
	"net/http"
	"testing"
)

func branchUser(branchID string) AuthUser {
	return AuthUser{RoleKey: "branch_pos", Scope: "branch", BranchID: &branchID}
}

func globalUser() AuthUser {
	return AuthUser{RoleKey: "super_admin", Scope: "global"}
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	appErr, ok := err.(*AppError)
	if !ok {
		t.Fatalf("expected an *AppError, got %T: %v", err, err)
	}
	return appErr.Code
}

// The rule in one table: the branch comes from the token, a request may narrow
// within it, and nothing may widen it.
func TestMustBranchID(t *testing.T) {
	unattached := AuthUser{RoleKey: "branch_pos", Scope: "branch"}
	blank := ""

	for _, test := range []struct {
		name      string
		user      AuthUser
		requested string
		want      string
		status    int
	}{
		{name: "หน้าร้านไม่ระบุสาขา ได้สาขาตัวเอง", user: branchUser("branch-a"), want: "branch-a"},
		{name: "หน้าร้านระบุสาขาตัวเอง ผ่าน", user: branchUser("branch-a"), requested: "branch-a", want: "branch-a"},
		{name: "หน้าร้านระบุสาขาอื่น ถูกปฏิเสธ", user: branchUser("branch-a"), requested: "branch-b", status: http.StatusForbidden},
		{name: "ช่องว่างรอบรหัสสาขาไม่ทำให้เล็ดลอด", user: branchUser("branch-a"), requested: "  branch-b  ", status: http.StatusForbidden},
		{name: "เว้นวรรครหัสสาขาตัวเอง ยังเป็นสาขาตัวเอง", user: branchUser("branch-a"), requested: " branch-a ", want: "branch-a"},
		{name: "บัญชีสาขาที่ไม่ผูกสาขา ถูกปฏิเสธ", user: unattached, status: http.StatusForbidden},
		{name: "บัญชีสาขาที่ผูกสาขาว่าง ถูกปฏิเสธ", user: AuthUser{RoleKey: "branch_pos", Scope: "branch", BranchID: &blank}, requested: "branch-a", status: http.StatusForbidden},
		{name: "สำนักงานใหญ่ระบุสาขาได้ทุกสาขา", user: globalUser(), requested: "branch-b", want: "branch-b"},
		{name: "สำนักงานใหญ่ไม่ระบุสาขา ต้องระบุ", user: globalUser(), status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := MustBranchID(test.user, test.requested)
			if code := statusOf(t, err); code != test.status {
				t.Fatalf("expected status %d, got %d (%v)", test.status, code, err)
			}
			if got != test.want {
				t.Fatalf("expected branch %q, got %q", test.want, got)
			}
		})
	}
}

// BranchFilter differs from MustBranchID in one place only: a global caller
// naming no branch means every branch, which is a legitimate answer for a
// listing. A branch-scoped caller still never gets it.
func TestBranchFilter(t *testing.T) {
	for _, test := range []struct {
		name      string
		user      AuthUser
		requested string
		want      string
		status    int
	}{
		{name: "สำนักงานใหญ่ไม่ระบุสาขา เห็นทุกสาขา", user: globalUser(), want: ""},
		{name: "สำนักงานใหญ่ระบุสาขา เห็นเฉพาะสาขานั้น", user: globalUser(), requested: "branch-b", want: "branch-b"},
		{name: "หน้าร้านไม่ระบุสาขา ถูกจำกัดที่สาขาตัวเอง", user: branchUser("branch-a"), want: "branch-a"},
		{name: "หน้าร้านระบุสาขาอื่น ถูกปฏิเสธ", user: branchUser("branch-a"), requested: "branch-b", status: http.StatusForbidden},
		{name: "บัญชีสาขาที่ไม่ผูกสาขา ไม่ได้เห็นทุกสาขา", user: AuthUser{RoleKey: "branch_pos", Scope: "branch"}, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := BranchFilter(test.user, test.requested)
			if code := statusOf(t, err); code != test.status {
				t.Fatalf("expected status %d, got %d (%v)", test.status, code, err)
			}
			if got != test.want {
				t.Fatalf("expected branch %q, got %q", test.want, got)
			}
		})
	}
}

func TestRequireGlobalScope(t *testing.T) {
	if err := RequireGlobalScope(globalUser()); err != nil {
		t.Fatalf("expected head office to pass: %v", err)
	}
	if code := statusOf(t, RequireGlobalScope(branchUser("branch-a"))); code != http.StatusForbidden {
		t.Fatalf("expected a shop to be refused, got %d", code)
	}
	// A token carrying no branch at all is refused too. The guard this replaced
	// read that as "unrestricted" and let it through.
	if code := statusOf(t, RequireGlobalScope(AuthUser{RoleKey: "branch_pos", Scope: "branch"})); code != http.StatusForbidden {
		t.Fatalf("expected an unattached account to be refused, got %d", code)
	}
}

// A deliberate refusal must survive the handlers that wrap whatever a service
// returns as a 500: reporting a branch-scope 403 as a server fault hides both
// the reason and the fact that the guard fired at all.
func TestWrapErrorKeepsARefusal(t *testing.T) {
	_, err := MustBranchID(branchUser("branch-a"), "branch-b")
	wrapped := WrapError(http.StatusInternalServerError, "โหลดข้อมูลไม่สำเร็จ", err)
	if wrapped.Code != http.StatusForbidden {
		t.Fatalf("expected the 403 to survive wrapping, got %d", wrapped.Code)
	}
	plain := WrapError(http.StatusInternalServerError, "โหลดข้อมูลไม่สำเร็จ", errPlain{})
	if plain.Code != http.StatusInternalServerError {
		t.Fatalf("expected a bare error to become a 500, got %d", plain.Code)
	}
}

type errPlain struct{}

func (errPlain) Error() string { return "boom" }
