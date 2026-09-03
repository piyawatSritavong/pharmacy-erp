"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
  BarChart3,
  ArrowLeftRight,
  Building2,
  CalendarClock,
  ChevronDown,
  Cross,
  FileText,
  History,
  LayoutDashboard,
  PackageCheck,
	PackagePlus,
  PackageSearch,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  ShoppingBasket,
	Handshake,
  Sparkles,
  Store,
	Tags,
  TableProperties,
  Warehouse,
  WandSparkles,
  type LucideIcon
} from "lucide-react";

import { LogoutButton } from "@/components/layout/logout-button";
import { usePosBadges } from "@/components/layout/use-pos-badges";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { NavigationItem, Session } from "@/types";

const iconByKey: Record<string, LucideIcon> = {
  dashboard: LayoutDashboard,
  reports_group: BarChart3,
  inventory_group: Warehouse,
  documents_group: FileText,
  real_inventory: Warehouse,
  ghost_inventory: WandSparkles,
	product_categories: Tags,
	purchase_orders: PackagePlus,
	suppliers: Handshake,
  stock_transfers: ArrowLeftRight,
  month_end: CalendarClock,
  month_end_report: TableProperties,
  government_sales: Building2,
  sales_management: FileText,
  generate_report: TableProperties,
  settings: Settings,
  pos_screen: ShoppingBasket,
  sales_history: History,
  inventory_check: PackageSearch,
  goods_transfer_receipt: PackageCheck,
  daily_sales_summary: BarChart3
};

function activePath(pathname: string, href: string) {
  return pathname === href || pathname.startsWith(`${href}/`);
}

/** The little corner badge on a Pro-gated menu item. Decorative (aria-hidden)
 *  so it doesn't lengthen the link's accessible name — the Pro gate on the page
 *  itself is what announces the feature is locked. */
function ProTag() {
  return (
    <span aria-hidden className="ml-auto inline-flex shrink-0 items-center gap-0.5 rounded-full bg-gradient-to-r from-amber-400 to-amber-500 px-1.5 py-0.5 text-[10px] font-bold uppercase leading-none text-white shadow-sm">
      <Sparkles className="h-2.5 w-2.5" />
      Pro
    </span>
  );
}

/** True when the item itself, or (for a C1 parent menu) any of its children, matches the current route. */
function itemIsActive(pathname: string, item: NavigationItem) {
  if (item.children?.length) return item.children.some((child) => activePath(pathname, child.href));
  return activePath(pathname, item.href);
}

export function MobileNav({ navigation }: { navigation: NavigationItem[] }) {
  const pathname = usePathname();

  return (
    <nav aria-label="เมนูหลักบนมือถือ" className="mb-5 overflow-x-auto lg:hidden print:hidden">
      <div className="flex min-w-max gap-2 rounded-2xl border bg-white p-2 shadow-card">
        {navigation.map((item) => {
          const active = itemIsActive(pathname, item);
          return (
            <Link
              className={cn(
                "rounded-xl px-4 py-2 text-sm font-semibold transition",
                active ? "bg-primary text-white" : "text-muted-foreground hover:bg-muted"
              )}
              href={item.href}
              key={item.key}
            >
              {item.title}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}

export function Sidebar({
  navigation,
  collapsed,
  onToggle
}: {
  navigation: NavigationItem[];
  collapsed: boolean;
  onToggle: () => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  // C1: which parent group is expanded — accordion-style, one at a time.
  // Whichever group contains the active route stays (or becomes) expanded.
  const [expandedKey, setExpandedKey] = useState<string | null>(null);

  useEffect(() => {
    const activeGroup = navigation.find((item) => item.children?.length && itemIsActive(pathname, item));
    setExpandedKey(activeGroup ? activeGroup.key : null);
  }, [pathname, navigation]);

  function selectParent(item: NavigationItem) {
    const alreadyExpanded = expandedKey === item.key;
    setExpandedKey(alreadyExpanded ? null : item.key);
    // Expanding a parent auto-activates its first submenu item as default.
    if (!alreadyExpanded && item.children?.length) router.push(item.children[0].href);
  }

  return (
    <aside className={cn("sticky top-0 flex h-screen min-h-0 flex-col border-r bg-white py-4 transition-[width,padding] duration-300", collapsed ? "w-[72px] px-2" : "w-[232px] px-3")}>
      <div className={cn("flex shrink-0 items-center", collapsed ? "justify-center" : "justify-between gap-2")}>
      <Link className={cn("flex items-center gap-2.5", !collapsed && "px-1")} href="/dashboard" title="PharmaPOS">
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-2xl bg-primary text-white shadow-brand">
          <Cross className="h-5 w-5" strokeWidth={3} />
        </span>
        <span className={collapsed ? "sr-only" : "block min-w-0"}>
          <span className="block truncate text-base font-bold tracking-tight">PharmaPOS</span>
          <span className="block truncate text-[11px] font-medium text-muted-foreground">ระบบบริหารร้านขายยา</span>
        </span>
      </Link>
      {!collapsed ? (
        <button aria-label="ซ่อนแถบเมนู" className="grid h-9 w-9 shrink-0 place-items-center rounded-xl text-muted-foreground transition hover:bg-muted hover:text-foreground" onClick={onToggle} title="ซ่อนแถบเมนู" type="button">
          <PanelLeftClose className="h-5 w-5" />
        </button>
      ) : null}
      </div>

      {collapsed ? (
        <button aria-label="แสดงแถบเมนู" className="mx-auto mt-3 grid h-9 w-9 shrink-0 place-items-center rounded-xl text-muted-foreground transition hover:bg-muted hover:text-foreground" onClick={onToggle} title="แสดงแถบเมนู" type="button">
          <PanelLeftOpen className="h-5 w-5" />
        </button>
      ) : null}

      <nav aria-label="เมนูหลัก" className={cn("min-h-0 flex-1 space-y-1 overflow-y-auto", collapsed ? "mt-4" : "mt-6")}>
        {navigation.map((item) => {
          const Icon = iconByKey[item.key] || LayoutDashboard;
          const active = itemIsActive(pathname, item);

          if (!item.children?.length) {
            return (
              <Link
                className={cn(
                  "flex items-center rounded-xl py-2.5 text-sm font-semibold transition",
                  collapsed ? "justify-center px-3" : "gap-3 px-3",
                  active ? "bg-secondary text-foreground shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
                href={item.href}
                key={item.key}
                title={collapsed ? item.title : undefined}
              >
                <Icon className={cn("h-5 w-5 shrink-0", active && "text-primary")} />
                <span className={collapsed ? "sr-only" : "truncate"}>{item.title}</span>
              </Link>
            );
          }

          // C2: collapsed sidebar shows a parent's submenu as a popover
          // instead of auto-expanding the whole sidebar.
          if (collapsed) {
            return (
              <DropdownMenu key={item.key}>
                <DropdownMenuTrigger>
                  <button
                    aria-label={item.title}
                    className={cn(
                      "flex w-full items-center justify-center rounded-xl py-2.5 transition",
                      active ? "bg-secondary text-foreground shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                    )}
                    title={item.title}
                    type="button"
                  >
                    <Icon className={cn("h-5 w-5", active && "text-primary")} />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" side="right">
                  <p className="px-3 pb-1.5 pt-1 text-xs font-bold text-muted-foreground">{item.title}</p>
                  {item.children.map((child) => (
                    <DropdownMenuItem asChild key={child.key}>
                      <Link className={cn("flex items-center gap-2", activePath(pathname, child.href) && "font-bold text-primary")} href={child.href}>
                        <span className="truncate">{child.title}</span>
                        {child.pro ? <ProTag /> : null}
                      </Link>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            );
          }

          const expanded = expandedKey === item.key;
          return (
            <div key={item.key}>
              <button
                aria-expanded={expanded}
                className={cn(
                  "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-semibold transition",
                  active ? "bg-secondary text-foreground shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
                onClick={() => selectParent(item)}
                type="button"
              >
                <Icon className={cn("h-5 w-5 shrink-0", active && "text-primary")} />
                <span className="flex-1 truncate text-left">{item.title}</span>
                <ChevronDown className={cn("h-4 w-4 shrink-0 transition-transform", expanded && "rotate-180")} />
              </button>
              {expanded ? (
                <div className="ml-[19px] mt-1 space-y-0.5 border-l pl-4">
                  {item.children.map((child) => {
                    const childActive = activePath(pathname, child.href);
                    return (
                      <Link
                        className={cn(
                          "flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-medium transition",
                          childActive ? "bg-secondary/60 font-bold text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                        )}
                        href={child.href}
                        key={child.key}
                      >
                        <span className="truncate">{child.title}</span>
                        {child.pro ? <ProTag /> : null}
                      </Link>
                    );
                  })}
                </div>
              ) : null}
            </div>
          );
        })}
      </nav>
      {/* Account and logout live in the sticky header now, not here. */}
    </aside>
  );
}

/**
 * POS chrome — logo, menu, branch/user and logout. Rendered as a bottom bar:
 * a POS runs on a touchscreen where the hand rests low, and the till's own
 * action buttons already live at the bottom of the cart, so nav sitting
 * beside them is both easier to reach and easier to see than a top header.
 */
export function PosBottomNav({
  navigation,
  user
}: {
  navigation: NavigationItem[];
  user: Session["user"];
}) {
  const pathname = usePathname();
  const badges = usePosBadges(String(user.branch_id || ""));

  return (
    <footer className="z-40 shrink-0 border-t bg-white/95 backdrop-blur">
      <div className="flex min-h-20 items-center gap-4 px-4 lg:px-6">
        <Link className="flex shrink-0 items-center gap-2" href="/sales">
          <span className="grid h-11 w-11 place-items-center rounded-2xl bg-foreground text-white">
            <Store className="h-5 w-5" />
          </span>
          <span className="hidden font-bold sm:block">PharmaPOS</span>
        </Link>

        <nav aria-label="เมนูจุดขาย" className="flex flex-1 items-center gap-1 overflow-x-auto">
          {navigation.map((item) => {
            const active = activePath(pathname, item.href);
            const Icon = iconByKey[item.key] || Store;
            const badge = badges[item.key] || 0;
            return (
              <Link
                className={cn(
                  "relative flex shrink-0 items-center gap-2 rounded-full px-4 py-2.5 text-sm font-semibold transition",
                  active ? "bg-primary text-white" : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
                href={item.href}
                key={item.key}
              >
                <Icon className="h-4 w-4" />
                <span>{item.title}</span>
                {badge > 0 ? (
                  <span className="absolute -right-0.5 -top-0.5 grid min-w-5 animate-pulse place-items-center rounded-full bg-red-600 px-1 text-[11px] font-bold leading-5 text-white">
                    {badge > 99 ? "99+" : badge}
                  </span>
                ) : null}
              </Link>
            );
          })}
        </nav>

        <div className="hidden shrink-0 text-right xl:block">
          <p className="text-sm font-bold">{user.name}</p>
          <p className="text-xs text-muted-foreground">{user.branch_name}</p>
        </div>
        <LogoutButton compact />
      </div>
    </footer>
  );
}
