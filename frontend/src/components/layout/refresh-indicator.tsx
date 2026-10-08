"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState, useSyncExternalStore, useTransition } from "react";

/*
 * After a save, consoles re-fetch the page's server data with router.refresh().
 * That used to run in a bare startTransition, so the list sat there stale with
 * nothing to say it was updating. useRefresh tracks the transition, and the one
 * RefreshIndicator in the shell shows a thin bar while any refresh is pending.
 *
 * The same bar covers moving between pages. Every page awaits its API calls
 * before rendering, so a menu click used to leave the old page up with no sign
 * of life. A route-level loading.tsx would say so too, but it swaps the whole
 * page for a fallback on every search-param change — and that aborted the
 * dashboard's own filter navigations. The bar leaves the page alone.
 */
let pendingCount = 0;
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function emit() {
  listeners.forEach((listener) => listener());
}

/** router.refresh(), with its pending state shown by the shell's RefreshIndicator. */
export function useRefresh() {
  const router = useRouter();
  const [isPending, startTransition] = useTransition();
  useEffect(() => {
    if (!isPending) return;
    pendingCount += 1;
    emit();
    return () => {
      pendingCount -= 1;
      emit();
    };
  }, [isPending]);
  return useCallback(() => startTransition(() => router.refresh()), [router]);
}

/** Marks a same-origin link click as pending until the route actually changes. */
function useNavigationPending() {
  const pathname = usePathname();
  const search = useSearchParams();
  const [pending, setPending] = useState(false);
  useEffect(() => setPending(false), [pathname, search]);
  useEffect(() => {
    function onClick(event: MouseEvent) {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>("a[href]") : null;
      if (!anchor || anchor.target === "_blank" || anchor.hasAttribute("download")) return;
      const url = new URL(anchor.href, window.location.href);
      if (url.origin !== window.location.origin || url.pathname.startsWith("/api/") || url.pathname.startsWith("/print/")) return;
      if (url.pathname === window.location.pathname && url.search === window.location.search) return;
      setPending(true);
    }
    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, []);
  useEffect(() => {
    if (!pending) return;
    pendingCount += 1;
    emit();
    return () => {
      pendingCount -= 1;
      emit();
    };
  }, [pending]);
}

export function RefreshIndicator() {
  useNavigationPending();
  const active = useSyncExternalStore(subscribe, () => pendingCount > 0, () => false);
  if (!active) return null;
  return (
    <div aria-live="polite" className="pointer-events-none fixed inset-x-0 top-0 z-[60] h-0.5 overflow-hidden bg-primary/20 print:hidden" role="status">
      <span className="sr-only">กำลังโหลดข้อมูล</span>
      <div className="h-full w-1/3 animate-[refresh-slide_1s_ease-in-out_infinite] bg-primary" />
    </div>
  );
}
