"use client";

import { usePathname } from "next/navigation";

import { LogoutButton } from "@/components/layout/logout-button";
import { NotificationBell } from "@/components/layout/notification-bell";
import { usePageHeader } from "@/components/layout/page-header";
import { MobileNav } from "@/components/layout/sidebar";
import { ThemeToggle } from "@/components/ui/theme-toggle";
import type { NavigationItem, Session } from "@/types";

/** Longest-prefix match of the current route against the nav tree, so a page
 *  that hasn't registered its own title still shows the right heading. */
function navMatch(navigation: NavigationItem[], pathname: string): NavigationItem | null {
  let best: NavigationItem | null = null;
  const consider = (item: NavigationItem) => {
    if (item.href && (pathname === item.href || pathname.startsWith(`${item.href}/`))) {
      if (!best || item.href.length >= best.href.length) best = item;
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

  // -mt on the header cancels <main>'s top padding so the bar sits flush at the
  // very top (y=0) whether or not the page is scrolled — otherwise it starts
  // lower when unscrolled and jumps up on scroll, which shifted anything
  // anchored to its bottom edge (e.g. the notification panel).
  return (
    <header className="sticky top-0 z-30 -mx-3 -mt-4 mb-4 flex items-center justify-between gap-2 border-b bg-background/95 px-2 py-2 backdrop-blur sm:-mx-6 sm:-mt-6 sm:mb-6 sm:gap-4 sm:px-6 sm:py-3 lg:-mx-10 lg:-mt-8 lg:px-10 print:hidden">
      <MobileNav navigation={navigation} user={user} />
      <div className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-3 gap-y-0.5">
        <h1 className="break-words text-base leading-snug sm:truncate sm:text-xl font-semibold tracking-tight sm:text-2xl">{title}</h1>
        {description ? (
          <p className="hidden max-w-2xl truncate text-sm leading-6 text-muted-foreground lg:block">{description}</p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <div className="hidden sm:block"><ThemeToggle /></div>
        <NotificationBell />
        <div className="hidden text-right sm:block">
          <p className="text-sm font-semibold leading-tight">{user.name}</p>
          <p className="text-xs text-muted-foreground">{user.role_name}</p>
        </div>
        <span className="hidden h-9 w-9 shrink-0 place-items-center rounded-full bg-foreground text-sm font-bold text-background sm:grid">
          {user.name.slice(0, 1).toUpperCase()}
        </span>
        <div className="hidden sm:block"><LogoutButton compact /></div>
      </div>
    </header>
  );
}
