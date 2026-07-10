"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import type { NavigationItem, Session } from "@/types";

export function MobileNav({ navigation }: { navigation: NavigationItem[] }) {
  const pathname = usePathname();

  return (
    <div className="mb-4 overflow-x-auto rounded-[20px] border border-black/10 bg-white/80 p-1.5 backdrop-blur lg:hidden">
      <div className="flex min-w-max gap-1">
        {navigation.map((item) => {
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.key}
              href={item.href}
              className={cn(
                "rounded-[14px] px-4 py-2 text-sm font-medium transition",
                active
                  ? "bg-black text-white"
                  : "text-black/60 hover:bg-black/5 hover:text-black"
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

  return (
    <aside className="sticky top-0 flex h-screen flex-col gap-4 border-r border-black/8 bg-white/80 px-4 py-5 backdrop-blur">
      <div className="space-y-1 rounded-[24px] border border-black/10 bg-black px-4 py-4 text-white shadow-panel">
        <p className="text-xs uppercase tracking-[0.28em] text-white/60">Pharmacy ERP</p>
        <h1 className="text-lg font-semibold">{user.role_name}</h1>
        <p className="text-sm text-white/70">{user.branch_name || "Enterprise"}</p>
      </div>
      <nav className="flex-1 space-y-1 overflow-y-auto">
        {navigation.map((item) => {
          const active = pathname.startsWith(item.href);
          return (
            <Link
              key={item.key}
              href={item.href}
              className={cn(
                "block rounded-[18px] border px-4 py-3 transition",
                active
                  ? "border-black bg-black text-white"
                  : "border-transparent bg-surface-50 text-black hover:border-black/10 hover:bg-white"
              )}
            >
              <p className="text-sm font-medium">{item.title}</p>
              <p className={cn("mt-0.5 text-xs", active ? "text-white/65" : "text-black/45")}>
                {item.description}
              </p>
            </Link>
          );
        })}
      </nav>
      <div className="rounded-[20px] border border-black/10 bg-surface-50 p-4">
        <p className="text-sm font-medium text-black">{user.name}</p>
        <p className="text-xs text-black/55">{user.email}</p>
      </div>
    </aside>
  );
}
