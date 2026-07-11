import type { PropsWithChildren } from "react";

import { MobileNav, Sidebar } from "@/components/layout/sidebar";
import type { Session } from "@/types";

export function AppShell({
  session,
  children
}: PropsWithChildren<{ session: Session }>) {
  return (
    <div className="grid min-h-screen bg-background lg:grid-cols-[auto_minmax(0,1fr)]">
      <div className="hidden lg:block">
        <Sidebar navigation={session.navigation} user={session.user} />
      </div>
      <main className="min-w-0 px-4 py-6 sm:px-6 lg:px-8">
        <MobileNav navigation={session.navigation} />
        {children}
      </main>
    </div>
  );
}
