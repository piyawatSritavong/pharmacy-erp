import type { ReactNode } from "react";

import { AppShell } from "@/components/layout/app-shell";
import { requireSession } from "@/services/erp";

/**
 * Nothing behind the auth gate is ever static.
 *
 * Belt to the braces of reading cookies() first in apiServer: this holds even
 * if some future code path throws before any dynamic API is touched, which is
 * exactly how every page under this layout came to be prerendered as a
 * redirect to /login. A route whose content depends on who is asking has
 * nothing to prerender.
 */
export const dynamic = "force-dynamic";

export default async function ProtectedLayout({
  children
}: {
  children: ReactNode;
}) {
  const session = await requireSession();

  return <AppShell session={session}>{children}</AppShell>;
}
