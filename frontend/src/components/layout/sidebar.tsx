"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  BarChart3,
  Boxes,
  CalendarClock,
  Cross,
  Landmark,
  LayoutDashboard,
  PackageSearch,
  PanelLeftClose,
  PanelLeftOpen,
  QrCode,
  ReceiptText,
  Settings,
  Store,
  type LucideIcon
} from "lucide-react";

import { cn } from "@/lib/utils";
import type { NavigationItem, Session } from "@/types";

const iconByKey: Record<string, LucideIcon> = {
  dashboard: LayoutDashboard,
  branch_dashboard: LayoutDashboard,
  inventory_management: Boxes,
  branch_inventory: Boxes,
  installments: CalendarClock,
  finance_central: Landmark,
  local_finance: Landmark,
  global_reports: BarChart3,
  daily_sales_summary: BarChart3,
  settings: Settings,
  sales_invoices: ReceiptText,
  pos_screen: Store,
  inventory_check: PackageSearch,
  goods_transfer_receipt: QrCode
};

const COLLAPSE_KEY = "pharmacy-sidebar-collapsed";

export function MobileNav({ navigation }: { navigation: NavigationItem[] }) {
  const pathname = usePathname();

  return (
    <div className="mb-6 overflow-x-auto rounded-lg border bg-card p-1 shadow-sm lg:hidden">
      <div className="flex min-w-max gap-1">
        {navigation.map((item) => {
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.key}
              href={item.href}
              className={cn(
                "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                active
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              {item.title}
            </Link>
          );
        })}
      </div>
    </div>
  );
}

export function Sidebar({
  navigation,
  user
}: {
  navigation: NavigationItem[];
  user: Session["user"];
}) {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    setCollapsed(window.localStorage.getItem(COLLAPSE_KEY) === "1");
  }, []);

  function toggle() {
    setCollapsed((current) => {
      window.localStorage.setItem(COLLAPSE_KEY, current ? "0" : "1");
      return !current;
    });
  }

  const initials = user.name
    .split(" ")
    .map((part) => part[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();

  return (
    <aside
      className={cn(
        "sticky top-0 flex h-screen flex-col border-r bg-card transition-[width] duration-200",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <div className={cn("flex h-16 items-center gap-3 border-b px-4", collapsed && "justify-center px-2")}>
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-md bg-accent text-accent-foreground">
          <Cross className="h-4 w-4" />
        </span>
        {!collapsed ? (
          <span className="min-w-0">
            <span className="block truncate text-sm font-semibold leading-tight">Pharmacy ERP</span>
            <span className="block truncate text-xs text-muted-foreground">{user.role_name}</span>
          </span>
        ) : null}
      </div>

      <nav className="flex-1 space-y-1 overflow-y-auto p-2">
        {navigation.map((item) => {
          const active = pathname.startsWith(item.href);
          const Icon = iconByKey[item.key] || LayoutDashboard;
          return (
            <Link
              key={item.key}
              href={item.href}
              title={collapsed ? item.title : undefined}
              className={cn(
                "relative flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                collapsed && "justify-center px-2",
                active
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              {active ? (
                <span aria-hidden className="absolute inset-y-1.5 left-0 w-0.5 rounded-full bg-accent" />
              ) : null}
              <Icon className="h-4 w-4 shrink-0" />
              {!collapsed ? <span className="truncate">{item.title}</span> : null}
            </Link>
          );
        })}
      </nav>

      <div className="space-y-1 border-t p-2">
        <button
          aria-label="Toggle sidebar"
          onClick={toggle}
          type="button"
          className={cn(
            "flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
            collapsed && "justify-center px-2"
          )}
        >
          {collapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
          {!collapsed ? <span>ย่อเมนู</span> : null}
        </button>
        <div className={cn("flex items-center gap-3 rounded-md px-3 py-2", collapsed && "justify-center px-2")}>
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-foreground">
            {initials}
          </span>
          {!collapsed ? (
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium leading-tight">{user.name}</span>
              <span className="block truncate text-xs text-muted-foreground">{user.email}</span>
            </span>
          ) : null}
        </div>
      </div>
    </aside>
  );
}
