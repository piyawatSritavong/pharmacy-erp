"use client";

import Link from "next/link";
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { Loader2, Pin, Plus } from "lucide-react";

import { SeriesBarChart } from "@/components/ui/bar-chart";
import { buttonVariants } from "@/components/ui/button";
import { formatReportCell } from "@/lib/report-format";
import { cn } from "@/lib/utils";
import { executeReport, listSavedReports } from "@/services/report-builder";
import type { ReportResult, SavedReport } from "@/types/report-builder";

/**
 * Reserved top-of-page slot for reports pinned here (A6) via Generate
 * Report's pin-target selector. Pass the page's NavigationItem `key` —
 * renders nothing when nothing is pinned to it, so pages don't need to
 * special-case the empty state themselves.
 *
 *   <ReportPinSlot pageKey="real_inventory" />
 *
 * Pages built entirely out of pinned reports (the dashboard) pass
 * `emptyPrompt` so the slot shows a placeholder inviting you to add one
 * instead of collapsing to nothing.
 */
export function ReportPinSlot({
  pageKey,
  className,
  emptyPrompt = false
}: {
  pageKey: string;
  className?: string;
  emptyPrompt?: boolean;
}) {
  const [reports, setReports] = useState<SavedReport[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    listSavedReports(pageKey)
      .then((response) => {
        if (!cancelled) setReports(response.items);
      })
      .catch(() => {
        if (!cancelled) setReports([]);
      });
    return () => {
      cancelled = true;
    };
  }, [pageKey]);

  // null = still loading; don't flash the placeholder before we know.
  if (!reports) return null;
  if (reports.length === 0) return emptyPrompt ? <AddReportPrompt className={className} /> : null;

  return (
    <div aria-label="รายงานที่ปักหมุด" className={cn("space-y-3", className)}>
      {reports.map((report) => (
        <PinnedReportPanel key={report.id} report={report} />
      ))}
    </div>
  );
}

/** Empty canvas for a page whose whole content is pinned reports. */
function AddReportPrompt({ className }: { className?: string }) {
  return (
    <div
      aria-label="ยังไม่มีรายงานที่ปักหมุด"
      className={cn(
        "flex min-h-[360px] flex-col items-center justify-center gap-5 rounded-2xl border-2 border-dashed border-border p-10 text-center",
        className
      )}
    >
      <span className="grid h-14 w-14 place-items-center rounded-2xl border-2 border-dashed border-border text-muted-foreground">
        <Pin className="h-6 w-6" />
      </span>
      <div className="space-y-1.5">
        <p className="text-base font-semibold">ยังไม่มีรายงานในหน้านี้</p>
        <p className="max-w-md text-sm leading-6 text-muted-foreground">
          สร้างรายงานที่ต้องการจากเมนู Generate Report แล้วปักหมุดมาที่หน้านี้ เพื่อจัดหน้าจอเองได้ตามที่ใช้งานจริง
        </p>
      </div>
      <Link className={buttonVariants()} href="/generate-report">
        <Plus className="h-4 w-4" />
        เพิ่มรายงาน
      </Link>
    </div>
  );
}

function PinnedReportPanel({ report }: { report: SavedReport }) {
  const ref = useRef<HTMLElement>(null);
  const [visible, setVisible] = useState(false);
  const [result, setResult] = useState<ReportResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [columnWidths, setColumnWidths] = useState<Record<number, number>>({});

  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) setVisible(true);
      },
      { rootMargin: "240px" }
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!visible || result || loading) return;
    setLoading(true);
    executeReport({ report_id: report.id, page: 1, preview: true })
      .then(setResult)
      .catch((reason) => setError(reason instanceof Error ? reason.message : "โหลดรายงานไม่สำเร็จ"))
      .finally(() => setLoading(false));
  }, [loading, report.id, result, visible]);

  return (
    <section className="overflow-hidden rounded-2xl border bg-white shadow-card" ref={ref}>
      <header className="flex items-center gap-3 border-b px-4 py-3">
        <span className="grid h-9 w-9 place-items-center rounded-xl bg-amber-100 text-amber-800">
          <Pin className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="truncate font-bold">{report.name}</h3>
          {report.description ? <p className="truncate text-xs text-muted-foreground">{report.description}</p> : null}
        </div>
      </header>
      {error ? (
        <p className="px-4 py-3 text-xs text-error">{error}</p>
      ) : loading && !result ? (
        <div className="flex h-32 items-center justify-center text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin" />
        </div>
      ) : (
        <>
          <ReportChart result={result} />
          <ResizableResultTable
            columnWidths={columnWidths}
            onResize={(index, width) => setColumnWidths((current) => ({ ...current, [index]: width }))}
            result={result}
          />
        </>
      )}
    </section>
  );
}

/**
 * Chart above a pinned report's table.
 *
 * Axis choice matters more than it looks. Plotting one bar per ROW produced
 * charts like "MES, โกดัง, MES, MES, คณาเภสัช, คณาเภสัช…" — the x-axis was row
 * order with the category repeated, which compares nothing. So:
 *
 *   X axis — a date/datetime column if the report has one (bucketed per day),
 *            otherwise the first text column.
 *   Y axis — up to two numeric measures (e.g. สต๊อกจริง and สต๊อกผี), drawn as
 *            grouped bars with a legend.
 *   Rows sharing an x value are SUMMED, so each branch or day appears once.
 *
 * Renders nothing when the shape doesn't suit a chart.
 */
function ReportChart({ result }: { result: ReportResult | null }) {
  if (!result || result.rows.length < 2) return null;

  const dateColumn = result.columns.find((column) => column.type === "date" || column.type === "datetime");
  // Money first — on a sales report "ยอดขาย" is the point, not a row count that
  // happens to sit in an earlier column.
  const numericColumns = [
    ...result.columns.filter((column) => column.format === "currency"),
    ...result.columns.filter((column) => column.format !== "currency" && (column.type === "number" || column.type === "integer"))
  ].slice(0, 2);
  const categoryColumn = result.columns.find(
    (column) => column !== dateColumn && !numericColumns.includes(column) && column.type === "text"
  );
  const axisColumn = dateColumn || categoryColumn;
  if (!axisColumn || !numericColumns.length) return null;

  const dayFormatter = new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short", timeZone: "Asia/Bangkok" });
  function axisLabel(raw: unknown) {
    if (!dateColumn) return String(raw ?? "—");
    const parsed = new Date(String(raw));
    return Number.isNaN(parsed.getTime()) ? String(raw ?? "—") : dayFormatter.format(parsed);
  }

  // Sum every row that lands on the same x value. `order` keeps first-seen
  // order for categories; time buckets get sorted chronologically below.
  const buckets = new Map<string, { label: string; sortKey: string; values: number[] }>();
  for (const row of result.rows) {
    const raw = row[axisColumn.key];
    const label = axisLabel(raw);
    const existing = buckets.get(label) || { label, sortKey: String(raw ?? ""), values: numericColumns.map(() => 0) };
    numericColumns.forEach((column, index) => {
      existing.values[index] += Number(row[column.key]) || 0;
    });
    buckets.set(label, existing);
  }

  let points = Array.from(buckets.values());
  if (dateColumn) points = points.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
  // Too many clusters is unreadable — keep the most recent/last 24.
  if (points.length > 24) points = points.slice(-24);
  if (points.length < 2) return null;
  if (points.every((point) => point.values.every((value) => value === 0))) return null;

  const series = numericColumns.map((column) => ({ key: column.key, label: column.label }));
  const chartPoints = points.map((point) => ({
    label: point.label,
    values: point.values,
    hints: point.values.map((value, index) => `${point.label} · ${numericColumns[index].label}: ${formatReportCell(value, numericColumns[index])}`)
  }));

  return (
    <div className="border-b bg-surface-warm px-4 py-3">
      <p className="mb-2 text-[11px] font-semibold text-muted-foreground">
        {series.map((entry) => entry.label).join(" · ")} ตาม {axisColumn.label}
      </p>
      <SeriesBarChart
        ariaLabel={`กราฟ ${series.map((entry) => entry.label).join(" และ ")} ตาม ${axisColumn.label}`}
        format={numericColumns[0].format === "currency" ? "currency" : "number"}
        points={chartPoints}
        series={series}
        xAxisLabel={`แกน X: ${axisColumn.label}${dateColumn ? " (รายวัน)" : ""}`}
      />
    </div>
  );
}

/** Read-only report grid with drag-to-resize columns — same pointer-drag interaction as the "All Fields" panel border in Generate Report. */
function ResizableResultTable({
  result,
  columnWidths,
  onResize
}: {
  result: ReportResult | null;
  columnWidths: Record<number, number>;
  onResize: (index: number, width: number) => void;
}) {
  const resizeState = useRef<{ index: number; startX: number; startWidth: number } | null>(null);

  useEffect(
    () => () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    },
    []
  );

  function widthFor(index: number, event?: ReactPointerEvent<HTMLElement>) {
    const state = resizeState.current;
    if (event && state && state.index === index) return Math.max(80, state.startWidth + event.clientX - state.startX);
    return columnWidths[index] || 140;
  }

  function startResize(index: number, event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    resizeState.current = { index, startX: event.clientX, startWidth: columnWidths[index] || 140 };
    event.currentTarget.setPointerCapture(event.pointerId);
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  }

  function finishResize(index: number, event: ReactPointerEvent<HTMLDivElement>) {
    if (!resizeState.current) return;
    onResize(index, widthFor(index, event));
    resizeState.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }

  if (!result || !result.rows.length) {
    return <p className="p-6 text-center text-sm text-muted-foreground">ไม่มีรายการแสดง</p>;
  }

  return (
    <div className="max-h-64 overflow-auto">
      <table className="min-w-full border-separate border-spacing-0 text-left text-xs">
        <thead className="sticky top-0 z-10 bg-[#f5eadb]">
          <tr>
            {result.columns.map((column, index) => (
              <th
                className="relative whitespace-nowrap border-b border-r px-3 py-2 font-bold"
                key={column.key}
                style={{ minWidth: columnWidths[index] || 140, width: columnWidths[index] }}
              >
                {column.label}
                <div
                  aria-label={`ปรับความกว้างคอลัมน์ ${column.label}`}
                  aria-orientation="vertical"
                  className="absolute -right-1 top-0 z-30 h-full w-2 cursor-col-resize touch-none"
                  onKeyDown={(event) => {
                    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
                    event.preventDefault();
                    onResize(index, (columnWidths[index] || 140) + (event.key === "ArrowRight" ? 16 : -16));
                  }}
                  onPointerCancel={(event) => finishResize(index, event)}
                  onPointerDown={(event) => startResize(index, event)}
                  onPointerMove={(event) => {
                    if (resizeState.current?.index === index) onResize(index, widthFor(index, event));
                  }}
                  onPointerUp={(event) => finishResize(index, event)}
                  role="separator"
                  tabIndex={0}
                />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {result.rows.map((row, rowIndex) => (
            <tr className="odd:bg-white even:bg-[#fffaf4]" key={rowIndex}>
              {result.columns.map((column) => (
                <td className="max-w-[24rem] truncate border-b border-r px-3 py-2" key={column.key} title={formatReportCell(row[column.key], column)}>
                  {formatReportCell(row[column.key], column)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
