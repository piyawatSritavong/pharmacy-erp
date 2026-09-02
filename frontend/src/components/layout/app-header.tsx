import { LogoutButton } from "@/components/layout/logout-button";
import { NotificationBell } from "@/components/layout/notification-bell";
import type { Session } from "@/types";

/**
 * Sticky back-office header. It carries the two things that used to sit at the
 * bottom of the sidebar — who you are and how to log out — plus the
 * notification bell, and stays put as the page scrolls so both are always one
 * reach away.
 */
export function AppHeader({ user }: { user: Session["user"] }) {
  return (
    <header className="sticky top-0 z-30 -mx-4 mb-6 flex items-center justify-end gap-2 border-b bg-background/80 px-4 py-3 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-10 lg:px-10 print:hidden">
      <NotificationBell />
      <div className="hidden text-right sm:block">
        <p className="text-sm font-semibold leading-tight">{user.name}</p>
        <p className="text-xs text-muted-foreground">{user.role_name}</p>
      </div>
      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-foreground text-sm font-bold text-white">
        {user.name.slice(0, 1).toUpperCase()}
      </span>
      <LogoutButton compact />
    </header>
  );
}
