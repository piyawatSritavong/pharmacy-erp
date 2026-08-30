"use client";

import { useCallback, useEffect, useRef, useState, type DependencyList } from "react";
import { Loader2 } from "lucide-react";

import { cn } from "@/lib/utils";

export type InfinitePage<T> = { items: T[]; hasMore: boolean };

/**
 * Shared infinite-scroll data loader (A4). Fetches `pageSize` records at a
 * time instead of one large one-shot fetch; pair with
 * `<InfiniteScrollTrigger>` placed at the bottom of the rendered list — once
 * it scrolls into view, the next page loads automatically.
 *
 * `fetchPage` is backend-agnostic on purpose (page-number pagination,
 * cursor pagination, etc. all differ across endpoints in this codebase) —
 * the caller adapts its own endpoint's response into `{ items, hasMore }`.
 */
export function useInfiniteList<T>({
  pageSize = 20,
  fetchPage,
  deps = []
}: {
  pageSize?: number;
  fetchPage: (page: number, pageSize: number) => Promise<InfinitePage<T>>;
  /** Reset to page 1 and refetch whenever these change (e.g. search/filter values). */
  deps?: DependencyList;
}) {
  const [items, setItems] = useState<T[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const pageRef = useRef(0);
  const loadingRef = useRef(false);
  const hasMoreRef = useRef(true);
  const requestId = useRef(0);
  const fetchPageRef = useRef(fetchPage);
  fetchPageRef.current = fetchPage;

  const loadMore = useCallback(() => {
    if (loadingRef.current || !hasMoreRef.current) return;
    const id = requestId.current;
    const nextPage = pageRef.current + 1;
    loadingRef.current = true;
    setLoading(true);
    setError("");
    fetchPageRef
      .current(nextPage, pageSize)
      .then((result) => {
        if (id !== requestId.current) return; // superseded by a reset
        setItems((current) => (nextPage === 1 ? result.items : [...current, ...result.items]));
        hasMoreRef.current = result.hasMore;
        setHasMore(result.hasMore);
        pageRef.current = nextPage;
      })
      .catch((caught: unknown) => {
        if (id !== requestId.current) return;
        setError(caught instanceof Error ? caught.message : "โหลดข้อมูลไม่สำเร็จ");
        hasMoreRef.current = false;
        setHasMore(false);
      })
      .finally(() => {
        if (id !== requestId.current) return;
        loadingRef.current = false;
        setLoading(false);
      });
  }, [pageSize]);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    requestId.current += 1; // invalidate any in-flight request from before the reset
    pageRef.current = 0;
    hasMoreRef.current = true;
    loadingRef.current = false;
    setItems([]);
    setHasMore(true);
    setLoading(false);
    setError("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { items, loading, hasMore, error, loadMore };
}

/** Drop at the bottom of an infinite list; loads the next page once visible. */
export function InfiniteScrollTrigger({
  hasMore,
  loading,
  onLoadMore,
  className
}: {
  hasMore: boolean;
  loading: boolean;
  onLoadMore: () => void;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const node = ref.current;
    if (!node || !hasMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMore();
      },
      { rootMargin: "200px" }
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasMore, onLoadMore]);

  if (!hasMore) return null;
  return (
    <div className={cn("flex items-center justify-center p-4", className)} ref={ref}>
      {loading ? <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /> : null}
    </div>
  );
}
