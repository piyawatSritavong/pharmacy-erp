import { redirect } from "next/navigation";

import type { Session } from "@/types";

export function requireRole(session: Session, roles: string[]) {
  if (!roles.includes(session.user.role_key)) {
    redirect(session.home_path);
  }
  return session;
}
