"use client";

import { useMemo, useState } from "react";

import { PAGE_SIZE_OPTIONS } from "@/components/ui/pagination";

/**
 * Client-side paging for a list the page already holds in full.
 *
 * Several endpoints (categories, claims, transfers, sales documents, อย.)
 * return their whole result set in one response and take no page params, so
 * these screens slice locally rather than pretending to page the API. Feed it
 * the ALREADY-filtered rows — changing a filter shrinks `rows`, and the hook
 * clamps the current page instead of stranding you on an empty page 7.
 */
export function usePagedRows<T>(rows: T[], initialPageSize: number = PAGE_SIZE_OPTIONS[0]) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(initialPageSize);

  return useMemo(() => {
    const total = rows.length;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    return {
      pageRows: rows.slice((safePage - 1) * pageSize, safePage * pageSize),
      // Spread straight onto <Pagination {...pager} />.
      pager: {
        page: safePage,
        pageSize,
        total,
        totalPages,
        onPageChange: setPage,
        onPageSizeChange: (size: number) => {
          setPageSize(size);
          setPage(1);
        }
      },
      /** Call from every filter control — a narrower list should start at page 1. */
      resetPage: () => setPage(1)
    };
  }, [page, pageSize, rows]);
}
