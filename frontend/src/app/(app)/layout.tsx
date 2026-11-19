import type { ReactNode } from "react";

import { headers } from "next/headers";
import { redirect } from "next/navigation";

import { AppShell } from "@/components/layout/app-shell";
import { portalAllowed, portalForHost } from "@/lib/portal-host";
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
  // Login already refuses the wrong door; this stops a session carried over
  // from before the doors were named from using the other one.
  const requestHeaders = await headers();
  const hostPortal = portalForHost(requestHeaders.get("x-forwarded-host") || requestHeaders.get("host"));
  if (!portalAllowed(hostPortal, session.user.portal)) redirect("/login?wrong_door=1");

  return <AppShell session={session}>{children}</AppShell>;
}
