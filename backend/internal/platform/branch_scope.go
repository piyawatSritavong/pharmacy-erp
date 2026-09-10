package platform

import (
	"net/http"
	"strings"
)

// Branch scope lives here and nowhere else.
//
// The rule is one sentence: the branch a request acts on is derived from the
// token, and a branch id arriving from a query string, a path segment or a body
// may narrow that but never widen it. Everything below is that sentence applied
// to the three shapes call sites actually need.
//
// It is centralised because the alternative was measured: 86 sites read
// user.BranchID directly, 24 more took a branch id straight from the client,
// the same guard was retyped inline 16 times, and two packages had a
// validateBranchScope of their own whose behaviour differed — inventory's
// returned nil for an empty branch id, so "no branch" passed validation as
// though it were a branch. Two endpoints had no check at all and leaked another
// shop's rows to anyone who asked for them by id.

// IsGlobalScope reports whether the caller may act beyond a single branch.
//
// Deliberately keyed on Scope alone. The old inline test was
// `user.BranchID != nil && user.Scope != "global"` to mean "branch-scoped",
// which reads a token with no branch on it as unrestricted — the failure mode
// points the wrong way. Here a branch-scoped caller carrying no branch is
// refused instead.
func IsGlobalScope(user AuthUser) bool {
	return user.Scope == "global"
}

// OwnBranchID is the branch in the caller's token, or "" for a global caller.
func OwnBranchID(user AuthUser) string {
	if user.BranchID == nil {
		return ""
	}
	return strings.TrimSpace(*user.BranchID)
}

// resolveBranch applies the rule. The result is the branch the caller may act
// on, or "" meaning every branch — which only a global caller can ever be given.
func resolveBranch(user AuthUser, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if IsGlobalScope(user) {
		return requested, nil
	}
	own := OwnBranchID(user)
	if own == "" {
		return "", NewError(http.StatusForbidden, "บัญชีนี้ไม่ได้ผูกกับสาขา")
	}
	if requested != "" && requested != own {
		return "", NewError(http.StatusForbidden, "เข้าถึงได้เฉพาะสาขาของตนเอง")
	}
	return own, nil
}

// MustBranchID resolves the single branch a request acts on, and never returns
// an empty one. Use it wherever the operation is meaningless without a branch:
// a sale, a stock movement, a per-branch setting.
//
// A branch-scoped caller gets their own branch whether or not they named it;
// naming somebody else's is 403. A global caller must name one — "every branch"
// is not an answer to "which branch is this sale at".
func MustBranchID(user AuthUser, requested string) (string, error) {
	branchID, err := resolveBranch(user, requested)
	if err != nil {
		return "", err
	}
	if branchID == "" {
		return "", NewError(http.StatusForbidden, "กรุณาระบุสาขา")
	}
	return branchID, nil
}

// BranchFilter resolves the branch a listing is narrowed to, where showing every
// branch is a legitimate answer — but only for a global caller. A branch-scoped
// caller always comes back pinned to their own branch, so "" here can never mean
// "everything" for them.
func BranchFilter(user AuthUser, requested string) (string, error) {
	return resolveBranch(user, requested)
}

// RequireGlobalScope refuses a branch-scoped caller outright. It is for the
// operations that are *about* branches rather than within one — editing a
// branch, deleting it, moving its document sequence — where clamping to the
// caller's own branch would not secure the endpoint but break it.
func RequireGlobalScope(user AuthUser) error {
	if IsGlobalScope(user) {
		return nil
	}
	return NewError(http.StatusForbidden, "ต้องใช้สิทธิ์ระดับสำนักงานใหญ่")
}
