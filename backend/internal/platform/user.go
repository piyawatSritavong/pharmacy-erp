package platform

import "github.com/labstack/echo/v4"

const (
	ContextUserKey      = "auth_user"
	ContextRequestIDKey = "request_id"
)

type AuthUser struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	RoleKey     string   `json:"role_key"`
	RoleName    string   `json:"role_name"`
	// Portal is "backoffice" or "pos" — which app shell the user's role logs
	// into (D11). Scope is "global" or "branch" — whether the role is tied
	// to one branch_id. Both come from roles.portal/roles.scope.
	Portal      string   `json:"portal"`
	Scope       string   `json:"scope"`
	BranchID    *string  `json:"branch_id"`
	BranchCode  *string  `json:"branch_code"`
	BranchName  *string  `json:"branch_name"`
	Permissions []string `json:"permissions"`
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
