import { Inbox, type LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

/** Fixed copy per spec (A3) — every empty table/list in the app renders this
 * same message, so it's defined once here instead of per page. */
const EMPTY_MESSAGE = "ไม่มีรายการแสดง";

export function EmptyState({
  description,
  icon: Icon = Inbox,
  className
}: {
  /** Optional secondary context (e.g. "ลองปรับตัวกรองแล้วค้นหาใหม่") — the primary message is always EMPTY_MESSAGE. */
  description?: string;
  icon?: LucideIcon;
  className?: string;
}) {
  return (
    <div className={cn("grid place-items-center gap-2 p-10 text-center", className)}>
      <Icon className="h-9 w-9 text-neutral-400" />
      <p className="text-sm font-semibold text-muted-foreground">{EMPTY_MESSAGE}</p>
      {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
    </div>
  );
}

/** Same message, rendered as a table row for empty <table><tbody> bodies. */
export function TableEmptyState({
  colSpan,
  description,
  icon,
  className
}: {
  colSpan: number;
  description?: string;
  icon?: LucideIcon;
  className?: string;
}) {
  return (
    <tr>
      <td className={cn("p-0", className)} colSpan={colSpan}>
        <EmptyState description={description} icon={icon} />
      </td>
    </tr>
  );
}
