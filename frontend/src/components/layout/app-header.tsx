"use client";

import { usePathname } from "next/navigation";

import { LogoutButton } from "@/components/layout/logout-button";
import { NotificationBell } from "@/components/layout/notification-bell";
import { usePageHeader } from "@/components/layout/page-header";
import type { NavigationItem, Session } from "@/types";

/** Longest-prefix match of the current route against the nav tree, so a page
 *  that hasn't registered its own title still shows the right heading. */
function navMatch(navigation: NavigationItem[], pathname: string): NavigationItem | null {
  let best: NavigationItem | null = null;
  const consider = (item: NavigationItem) => {
    if (item.href && (pathname === item.href || pathname.startsWith(`${item.href}/`))) {
      if (!best || item.href.length > best.href.length) best = item;
    }
    item.children?.forEach(consider);
  };
  navigation.forEach(consider);
  return best;
}

/**
 * Sticky back-office header. The page heading (title + description) rides on
 * the left of this same bar — pages hand it up through PageHeaderProvider — so
 * it shares one row with the notification bell and the account controls (who
 * you are, and how to log out) that used to sit at the bottom of the sidebar.
 * The whole bar stays put as the page scrolls, so all of it is one reach away.
 */
export function AppHeader({ user, navigation }: { user: Session["user"]; navigation: NavigationItem[] }) {
  const pathname = usePathname();
  const context = usePageHeader();
  const fallback = navMatch(navigation, pathname);
  const title = context?.header?.title ?? fallback?.title ?? "";
  const description = context?.header?.description ?? fallback?.description;

  return (
    <header className="sticky top-0 z-30 -mx-4 mb-6 flex items-center justify-between gap-4 border-b bg-background/80 px-4 py-3 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-10 lg:px-10 print:hidden">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5">
        <h1 className="truncate text-xl font-semibold tracking-tight sm:text-2xl">{title}</h1>
        {description ? (
          <p className="hidden max-w-2xl truncate text-sm leading-6 text-muted-foreground lg:block">{description}</p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <NotificationBell />
        <div className="hidden text-right sm:block">
          <p className="text-sm font-semibold leading-tight">{user.name}</p>
          <p className="text-xs text-muted-foreground">{user.role_name}</p>
        </div>
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-foreground text-sm font-bold text-white">
          {user.name.slice(0, 1).toUpperCase()}
        </span>
        <LogoutButton compact />
      </div>
    </header>
  );
}
