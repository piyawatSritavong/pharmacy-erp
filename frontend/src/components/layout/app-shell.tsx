import type { PropsWithChildren } from "react";

import { MobileNav, Sidebar } from "@/components/layout/sidebar";
import type { Session } from "@/types";

export function AppShell({
  session,
  children
}: PropsWithChildren<{ session: Session }>) {
  return (
    <div className="grid min-h-screen bg-[radial-gradient(circle_at_top_left,_rgba(0,0,0,0.06),transparent_28%),linear-gradient(180deg,#fafaf7_0%,#f1f0ea_100%)] lg:grid-cols-[260px_minmax(0,1fr)]">
      <div className="hidden lg:block">
        <Sidebar navigation={session.navigation} user={session.user} />
      </div>
      <main className="min-w-0 px-4 py-4 sm:px-6 lg:px-8 lg:py-8">
        <MobileNav navigation={session.navigation} />
        {children}
      </main>
    </div>
  );
}
