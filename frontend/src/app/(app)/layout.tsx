import type { ReactNode } from "react";

import { AppShell } from "@/components/layout/app-shell";
import { requireSession } from "@/services/erp";

export default async function ProtectedLayout({
  children
}: {
  children: ReactNode;
}) {
  const session = await requireSession();

  return <AppShell session={session}>{children}</AppShell>;
}
