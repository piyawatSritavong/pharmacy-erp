"use client";

import type { PropsWithChildren } from "react";
import { useState } from "react";

import { AppHeader } from "@/components/layout/app-header";
import { PageHeaderProvider } from "@/components/layout/page-header";
import { PosBottomNav, Sidebar } from "@/components/layout/sidebar";
import { useVisualViewport } from "@/components/ui/use-visual-viewport";
import { cn } from "@/lib/utils";
import type { Session } from "@/types";

export function AppShell({
  session,
  children
}: PropsWithChildren<{ session: Session }>) {
  useVisualViewport();
	const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

	if (session.user.portal === "pos") {
    return (
      // Nav renders after <main> so it sits along the bottom edge of the
      // screen, next to the till's own action buttons.
      <div className="flex h-[var(--app-viewport-height,100dvh)] min-h-0 flex-col overflow-hidden bg-pos-canvas">
        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto p-3 sm:p-4 lg:p-5">{children}</main>
        <PosBottomNav navigation={session.navigation} user={session.user} />
      </div>
    );
	}
	// print:min-h-0 on the wrapper below — with the sidebar print:hidden,
	// its own forced min-h-screen would otherwise reserve a full blank
	// viewport above whatever actually prints (a portaled dialog's print
	// document renders as a later sibling in the document, after this).
	return (
		<div className={cn("grid min-h-screen bg-background transition-[grid-template-columns] duration-300 print:min-h-0", sidebarCollapsed ? "lg:grid-cols-[72px_minmax(0,1fr)]" : "lg:grid-cols-[232px_minmax(0,1fr)]")}>
			{/* print:hidden — the nav chrome should never leak into any in-page
			    window.print() call (see D4's PO print layout; month-end has its
			    own in-page print button too, so only the sidebar/nav is hidden
			    here, not all of <main>). */}
			<div className="hidden lg:block print:hidden">
				<Sidebar collapsed={sidebarCollapsed} navigation={session.navigation} onToggle={() => setSidebarCollapsed((current) => !current)} />
      </div>
      {/* Every back-office page shares this padded template — no route gets a
          full-bleed, non-scrolling workspace of its own (สรุปสิ้นเดือน used
          to, and its content was simply clipped once it outgrew the viewport). */}
      <main className="min-w-0 px-3 py-4 sm:px-6 sm:py-6 lg:px-10 lg:py-8">
        <PageHeaderProvider>
          <AppHeader navigation={session.navigation} user={session.user} />
          {children}
        </PageHeaderProvider>
      </main>
    </div>
  );
}
