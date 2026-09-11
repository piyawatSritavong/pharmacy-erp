package platform

import "github.com/labstack/echo/v4"

const (
	ContextUserKey        = "auth_user"
	ContextRequestIDKey   = "request_id"
	ContextBranchAuditKey = "branch_audit"
	ContextServerErrorKey = "server_error"
)

// BranchAuditFrom returns the request's branch-decision recorder, or nil when
// the request is not running under the access log (a test, a CLI command).
func BranchAuditFrom(c echo.Context) *BranchAudit {
	audit, _ := c.Get(ContextBranchAuditKey).(*BranchAudit)
	return audit
}

type AuthUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	RoleKey  string `json:"role_key"`
	RoleName string `json:"role_name"`
	// Portal is "backoffice" or "pos" — which app shell the user's role logs
	// into (D11). Scope is "global" or "branch" — whether the role is tied
	// to one branch_id. Both come from roles.portal/roles.scope.
	Portal      string   `json:"portal"`
	Scope       string   `json:"scope"`
	BranchID    *string  `json:"branch_id"`
	BranchCode  *string  `json:"branch_code"`
	BranchName  *string  `json:"branch_name"`
	Permissions []string `json:"permissions"`

	// ScopeAudit is the request's branch-decision recorder, attached by the
	// access log and written by the branch rule. It is not part of the user and
	// never crosses the wire; a zero AuthUser carries none and records nothing.
	ScopeAudit *BranchAudit `json:"-"`
}

func CurrentUser(c echo.Context) AuthUser {
	user, ok := c.Get(ContextUserKey).(AuthUser)
	if !ok {
		return AuthUser{}
	}
	return user
}

func HasPermission(user AuthUser, permission string) bool {
	for _, item := range user.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}
