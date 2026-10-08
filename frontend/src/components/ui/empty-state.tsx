import type { ReactNode } from "react";
import { Inbox, type LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

/** Fixed copy per spec (A3) — every empty table/list in the app renders this
 * same message, so it's defined once here instead of per page. */
const EMPTY_MESSAGE = "ไม่มีรายการแสดง";

export function EmptyState({
  description,
  icon: Icon = Inbox,
  action,
  variant = "plain",
  className
}: {
  /** Optional secondary context (e.g. "ลองปรับตัวกรองแล้วค้นหาใหม่") — the primary message is always EMPTY_MESSAGE. */
  description?: string;
  icon?: LucideIcon;
  /** The next step on a first run — usually the page's own create button. */
  action?: ReactNode;
  /** plain: inside a card or table. card: standing alone as its own dashed panel. */
  variant?: "plain" | "card";
  className?: string;
}) {
  return (
    <div className={cn("grid place-items-center gap-2 p-10 text-center", variant === "card" && "rounded-2xl border border-dashed bg-card", className)}>
      <Icon aria-hidden className="h-9 w-9 text-muted-foreground/60" />
      <p className="text-sm font-semibold text-muted-foreground">{EMPTY_MESSAGE}</p>
      {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

/** Same message, rendered as a table row for empty <table><tbody> bodies. */
export function TableEmptyState({
  colSpan,
  description,
  icon,
  action,
  className
}: {
  colSpan: number;
  description?: string;
  icon?: LucideIcon;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <tr>
      <td className={cn("p-0", className)} colSpan={colSpan}>
        <EmptyState action={action} description={description} icon={icon} />
      </td>
    </tr>
  );
}
