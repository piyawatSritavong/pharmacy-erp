"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

export const PAGE_SIZE_OPTIONS = [20, 50, 100];

export type PaginationState = {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

/**
 * Shared pager for every list screen: jump straight to a page, change how
 * many rows load at a time, and always see the total. Stacks into one column
 * when it sits in a narrow container (e.g. the stock list aside).
 */
export function Pagination({
  className,
  onPageChange,
  onPageSizeChange,
  page,
  pageSize,
  stacked = false,
  total,
  totalPages
}: {
  className?: string;
  onPageChange: (page: number) => void;
  /** Omit to hide the rows-per-page control (lists with a fixed page size). */
  onPageSizeChange?: (pageSize: number) => void;
  page: number;
  pageSize: number;
  /** Forces the vertical layout regardless of viewport — for narrow columns. */
  stacked?: boolean;
  total: number;
  totalPages: number;
}) {
  const safeTotalPages = Math.max(1, totalPages);
  const safePage = Math.min(Math.max(1, page), safeTotalPages);
  const pages = Array.from({ length: safeTotalPages }, (_, index) => index + 1);

  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-3",
        stacked && "flex-col items-stretch",
        className
      )}
    >
      <p className={cn("text-xs text-muted-foreground", stacked && "text-center")}>
        ทั้งหมด {total.toLocaleString("th-TH")} รายการ
      </p>
      <div className={cn("grid w-full grid-cols-2 items-center gap-2 sm:flex sm:w-auto sm:flex-wrap", stacked && "justify-center")}>
        {/* Fixed widths: the label grows with the page number ("หน้า 1 / 9" vs
            "หน้า 38 / 38"), and an auto-width trigger would resize under the
            cursor as you page through. */}
        <Select
          aria-label="เลือกหน้า"
          className="h-9 w-full sm:w-[9.5rem] sm:shrink-0"
          onChange={(event) => onPageChange(Number(event.target.value))}
          value={String(safePage)}
        >
          {pages.map((value) => (
            <option key={value} value={String(value)}>
              หน้า {value.toLocaleString("th-TH")} / {safeTotalPages.toLocaleString("th-TH")}
            </option>
          ))}
        </Select>
        {onPageSizeChange ? (
          <Select
            aria-label="จำนวนรายการต่อหน้า"
            className="h-9 w-full sm:w-[9.5rem] sm:shrink-0"
            onChange={(event) => onPageSizeChange(Number(event.target.value))}
            value={String(pageSize)}
          >
            {PAGE_SIZE_OPTIONS.map((value) => (
              <option key={value} value={String(value)}>
                {value} รายการ/หน้า
              </option>
            ))}
          </Select>
        ) : null}
        <div className="col-span-2 flex justify-end gap-2">
          <Button
            aria-label="หน้าก่อนหน้า"
            className="h-9"
            disabled={safePage <= 1}
            onClick={() => onPageChange(safePage - 1)}
            type="button"
            variant="secondary"
          >
            <ChevronLeft className="h-4 w-4" />
            ก่อนหน้า
          </Button>
          <Button
            aria-label="หน้าถัดไป"
            className="h-9"
            disabled={safePage >= safeTotalPages}
            onClick={() => onPageChange(safePage + 1)}
            type="button"
            variant="secondary"
          >
            ถัดไป
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
