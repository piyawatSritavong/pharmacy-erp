"use client";

/**
 * FEATURE: pin/bookmark. Renders one pinned report with its own lazy result
 * fetch, plus reorder and unpin controls. The pin toggle itself lives in the
 * toolbar (report-toolbar.tsx); this is the read side.
 */

import { ArrowDown, ArrowUp, Loader2, Pin, PinOff } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { TableEmptyState } from "@/components/ui/empty-state";
import { Notice } from "@/components/ui/state-block";

import { Timeline } from "./report-chart";
import { ReportGrid } from "./report-grid";
import { executeReport } from "@/services/report-builder";
import type { ReportDataset, ReportResult, SavedReport } from "@/types/report-builder";
import { fieldLabel, filterCount, formatCell } from "./shared";

export function PinnedReportCard({ report, dataset, onEdit, onUnpin, onMove, canMoveUp, canMoveDown }: {
  report: SavedReport;
  dataset?: ReportDataset;
  onEdit: () => void;
  onUnpin: () => void;
  onMove: (direction: -1 | 1) => void;
  canMoveUp: boolean;
  canMoveDown: boolean;
}) {
  const ref = useRef<HTMLElement>(null);
  const [visible, setVisible] = useState(false);
  const [result, setResult] = useState<ReportResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  // A6: resizable columns on pinned report tables.
  const [columnWidths, setColumnWidths] = useState<Record<number, number>>({});
  const resizeColumn = useCallback((index: number, width: number) => {
    setColumnWidths((current) => ({ ...current, [index]: Math.round(width) }));
  }, []);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const observer = new IntersectionObserver((entries) => { if (entries.some((entry) => entry.isIntersecting)) setVisible(true); }, { rootMargin: "240px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!visible || result || loading) return;
    setLoading(true);
    executeReport({ report_id: report.id, page: 1, preview: true }).then(setResult).catch((reason) => setError(reason instanceof Error ? reason.message : "โหลดรายงานไม่สำเร็จ")).finally(() => setLoading(false));
  }, [loading, report.id, result, visible]);
  return (
    <section className="overflow-hidden rounded-2xl border bg-white shadow-card" ref={ref}>
      <header className="flex flex-wrap items-center gap-3 border-b px-4 py-3">
        <span className="grid h-9 w-9 place-items-center rounded-xl bg-amber-100 text-amber-800"><Pin className="h-4 w-4" /></span>
        <div className="min-w-0 flex-1"><h3 className="truncate font-bold">{report.name}</h3><p className="truncate text-xs text-muted-foreground">{report.description || "รายงานที่ปักหมุด"} · {filterCount(report.definition.filters)} ตัวกรอง</p></div>
        <div className="flex items-center gap-1"><button aria-label={`เลื่อน ${report.name} ขึ้น`} className="rounded-lg border p-2 hover:bg-muted disabled:opacity-30" disabled={!canMoveUp} onClick={() => onMove(-1)} type="button"><ArrowUp className="h-3.5 w-3.5" /></button><button aria-label={`เลื่อน ${report.name} ลง`} className="rounded-lg border p-2 hover:bg-muted disabled:opacity-30" disabled={!canMoveDown} onClick={() => onMove(1)} type="button"><ArrowDown className="h-3.5 w-3.5" /></button><button className="rounded-lg border px-3 py-2 text-xs font-bold hover:bg-muted" onClick={onEdit} type="button">เปิดแก้ไข</button><button aria-label={`ถอนหมุด ${report.name}`} className="rounded-lg border p-2 text-muted-foreground hover:bg-red-50 hover:text-primary" onClick={onUnpin} type="button"><PinOff className="h-3.5 w-3.5" /></button></div>
      </header>
      {error ? <Notice className="rounded-none" tone="error">{error}</Notice> : <><Timeline compact dataset={dataset} definition={report.definition} result={result} /><ReportGrid columnWidths={columnWidths} compact loading={loading} onColumnResize={resizeColumn} result={result} /></>}
    </section>
  );
}

