import { redirect } from "next/navigation";

import type { Session } from "@/types";

/**
 * One structured line to stderr when a gate turns someone away.
 *
 * A session can load perfectly well and still hold nothing — a role whose
 * role_permissions rows are missing gives /me a 200 with permissions: [] —
 * and then these gates redirect to home_path, which for a back-office user
 * with no permissions at all resolves to /login. That looks exactly like a
 * failed login and is not one. The permission count is the number that tells
 * the two apart; the role and the required keys say which gate and why.
 */
function logGateRedirect(gate: string, session: Session, required: string[]) {
  console.error(JSON.stringify({
    event: "session.denied",
    gate,
    to: session.home_path,
    role_key: session.user.role_key,
    portal: session.user.portal,
    required,
    permission_count: session.user.permissions.length
  }));
}

/** @deprecated D11 — pages should call requirePermission() instead, since
 * the role catalog is no longer just super_admin/branch_pos. Kept only in
 * case something still needs a literal role-key check. */
export function requireRole(session: Session, roles: string[]) {
  if (!roles.includes(session.user.role_key)) {
    logGateRedirect("requireRole", session, roles);
    redirect(session.home_path);
  }
  return session;
}

// requirePermission is the D11 server-side route guard: a page.tsx calls
// this with the permission key(s) that gate its data (matching the same
// RequireAnyPermission(...) key(s) the backend route uses), and gets
// redirected to their own home_path if they hold none of them. Because this
// runs in a server component, it blocks direct URL access too — not just
// hiding the nav link.
export function requirePermission(session: Session, permissionKeys: string[]) {
  const allowed = permissionKeys.some((key) => session.user.permissions.includes(key));
  if (!allowed) {
    logGateRedirect("requirePermission", session, permissionKeys);
    redirect(session.home_path);
  }
  return session;
}
