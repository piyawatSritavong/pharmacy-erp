import { redirect } from "next/navigation";

import type { Session } from "@/types";

/** @deprecated D11 — pages should call requirePermission() instead, since
 * the role catalog is no longer just super_admin/branch_pos. Kept only in
 * case something still needs a literal role-key check. */
export function requireRole(session: Session, roles: string[]) {
  if (!roles.includes(session.user.role_key)) {
    // DIAG — temporary.
    console.log(JSON.stringify({
      DIAG: "requireRole.redirect",
      to: session.home_path,
      role_key: session.user.role_key,
      required_roles: roles
    }));
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
    // DIAG — temporary. Which gate, which role, what it held, what it needed.
    console.log(JSON.stringify({
      DIAG: "requirePermission.redirect",
      to: session.home_path,
      role_key: session.user.role_key,
      required_permissions: permissionKeys,
      permission_count: session.user.permissions.length
    }));
    redirect(session.home_path);
  }
  return session;
}
