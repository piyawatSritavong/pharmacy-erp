"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { AlertTriangle, Bell, PackagePlus, RotateCcw } from "lucide-react";
import { toast } from "sonner";

import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { proxyClient } from "@/services/api";

type Notification = { type: string; label: string; detail: string; count: number; href: string };

const ICONS: Record<string, typeof Bell> = {
  low_stock: AlertTriangle,
  requisition: PackagePlus,
  claim: RotateCcw
};

const POLL_MS = 45_000;
const STORAGE_KEY = "pharmapos.notifications.ack";

function signature(items: Notification[]) {
  return items.map((item) => `${item.type}:${item.count}`).join("|");
}

/**
 * The bell watches for things needing attention — low stock, requisitions,
 * claims — and does three things: keeps a running count on the bell until the
 * viewer opens it, pops a toast (that fades on its own) when something new
 * arrives, and lists what is waiting with a link to act on each.
 */
export function NotificationBell() {
  const [items, setItems] = useState<Notification[]>([]);
  const [acknowledged, setAcknowledged] = useState(true);
  const lastSignature = useRef<string | null>(null);
  const firstLoad = useRef(true);

  const load = useCallback(async () => {
    try {
      const response = await proxyClient<{ items: Notification[] }>("/notifications");
      const next = (response.items || []).filter((item) => item.count > 0);
      const sig = signature(next);
      const total = next.reduce((sum, item) => sum + item.count, 0);

      let ackSig: string | null = null;
      try { ackSig = window.localStorage.getItem(STORAGE_KEY); } catch { ackSig = null; }

      if (firstLoad.current) {
        // On first load, don't toast a backlog; only badge if unseen since last visit.
        firstLoad.current = false;
        setAcknowledged(total === 0 || sig === ackSig);
      } else if (sig !== lastSignature.current && total > 0 && sig !== ackSig) {
        // Something changed since the last poll — surface it, then let it fade.
        for (const item of next) {
          const Icon = ICONS[item.type] || Bell;
          toast(item.label, { description: `${item.detail} · ${item.count.toLocaleString("th-TH")} รายการ`, duration: 5000, icon: <Icon className="h-4 w-4" /> });
        }
        setAcknowledged(false);
      } else if (total === 0) {
        setAcknowledged(true);
      }
      lastSignature.current = sig;
      setItems(next);
    } catch {
      // A failed poll is silent; the bell keeps its last known state.
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  const total = items.reduce((sum, item) => sum + item.count, 0);
  const showBadge = total > 0 && !acknowledged;

  function markRead() {
    setAcknowledged(true);
    try { window.localStorage.setItem(STORAGE_KEY, signature(items)); } catch { /* private mode */ }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger>
        <button aria-label={`การแจ้งเตือน${total > 0 ? ` ${total} รายการ` : ""}`} className="relative grid h-10 w-10 place-items-center rounded-full text-muted-foreground transition hover:bg-muted hover:text-foreground" onClick={markRead} type="button">
          <Bell className="h-5 w-5" />
          {showBadge ? (
            <span className="absolute -right-0.5 -top-0.5 grid min-w-5 animate-pulse place-items-center rounded-full bg-red-600 px-1 text-[11px] font-bold leading-5 text-white">
              {total > 99 ? "99+" : total}
            </span>
          ) : null}
        </button>
      </DropdownMenuTrigger>
      {/* notif-flush-right pins the panel to the header's right edge via a
          scoped rule in globals.css (Radix puts its transform on the popper
          wrapper, so a class on the content alone can't move it). */}
      <DropdownMenuContent align="end" className="w-80 p-0 notif-flush-right">
        <div className="border-b px-4 py-3">
          <p className="text-sm font-semibold">การแจ้งเตือนระบบ</p>
          <p className="text-xs text-muted-foreground">{total > 0 ? `มี ${total.toLocaleString("th-TH")} รายการที่ต้องดำเนินการ` : "ไม่มีรายการที่ต้องดำเนินการ"}</p>
        </div>
        {items.length === 0 ? (
          <p className="px-4 py-8 text-center text-sm text-muted-foreground">ทุกอย่างเรียบร้อย ไม่มีการแจ้งเตือน</p>
        ) : (
          <ul className="max-h-96 overflow-auto py-1">
            {items.map((item) => {
              const Icon = ICONS[item.type] || Bell;
              return (
                <li key={item.type}>
                  <Link className="flex items-start gap-3 px-4 py-3 transition hover:bg-muted" href={item.href}>
                    <span className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-full bg-amber-100 text-amber-700"><Icon className="h-4 w-4" /></span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center justify-between gap-2">
                        <span className="text-sm font-medium">{item.label}</span>
                        <span className="shrink-0 rounded-full bg-red-100 px-2 py-0.5 text-xs font-semibold text-red-700">{item.count.toLocaleString("th-TH")}</span>
                      </span>
                      <span className="mt-0.5 block text-xs text-muted-foreground">{item.detail}</span>
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
