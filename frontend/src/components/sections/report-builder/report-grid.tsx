"use client";

/**
 * FEATURE: data table.
 * Owns four things, kept separate so each can be exercised on its own:
 *   1. COLUMN DRAG-REORDER — the drop TARGET half of drag-and-drop. Accepts a
 *      "field" drag from field-browser.tsx (insert a new column) and a "column"
 *      drag from its own headers (move an existing one). onColumnDrop decides.
 *   2. COLUMN RESIZE — pointer-drag on the header's right edge.
 *   3. SORT — click a header to cycle direction; the menu in column-menu.tsx
 *      sets the same field.
 *   4. PROGRESSIVE DISCLOSURE — `leadColumnCount` keeps the table readable by
 *      showing a lead slice inline and revealing the rest per row on click.
 */

import { ArrowDown, ArrowUp, GripVertical, Loader2, MoreHorizontal } from "lucide-react";
import type { DragEvent as ReactDragEvent, PointerEvent as ReactPointerEvent } from "react";
import { useEffect, useRef, useState } from "react";

import { TableEmptyState } from "@/components/ui/empty-state";
import { cn } from "@/lib/utils";
import type { ReportDataset, ReportDefinition, ReportResult, ReportResultColumn } from "@/types/report-builder";
import type { ColumnDragItem, ColumnDropSide } from "./shared";
import { dragItemFromTransfer, fieldLabel, formatCell } from "./shared";

export type GridProps = {
  result?: ReportResult | null;
  definition?: ReportDefinition;
  dataset?: ReportDataset;
  loading?: boolean;
  compact?: boolean;
  columnWidths?: Record<number, number>;
  dragItem?: ColumnDragItem;
  onHeaderMenu?: (index: number, x: number, y: number) => void;
  onColumnDragStart?: (index: number, event: ReactDragEvent<HTMLElement>) => void;
  onColumnDragEnd?: () => void;
  onColumnDrop?: (targetIndex: number, side: ColumnDropSide, item?: ColumnDragItem) => void;
  /** A6: pinned report tables get drag-to-resize columns — same pointer-drag interaction as the "All Fields" panel border (see FieldBrowser above). */
  onColumnResize?: (index: number, width: number, commit: boolean) => void;
  /** Progressive disclosure: show at most this many columns inline and move the
   *  rest into a per-row detail panel. 0 disables it and shows every column. */
  leadColumnCount?: number;
};

export function ReportGrid({ result, definition, dataset, loading, compact, columnWidths, dragItem, onHeaderMenu, onColumnDragStart, onColumnDragEnd, onColumnDrop, onColumnResize, leadColumnCount = 0 }: GridProps) {
  const [dropTarget, setDropTarget] = useState<{ index: number; side: ColumnDropSide } | null>(null);
  // PROGRESSIVE DISCLOSURE: which row is expanded. Only one at a time — this is
  // "show me the rest of this row", not a multi-select.
  const [expandedRow, setExpandedRow] = useState<number | null>(null);
  const resizeState = useRef<{ index: number; startX: number; startWidth: number } | null>(null);

  useEffect(() => () => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }, []);

  function resizedWidth(index: number, event: ReactPointerEvent<HTMLElement>) {
    const state = resizeState.current;
    return state && state.index === index ? Math.max(80, state.startWidth + event.clientX - state.startX) : columnWidths?.[index] || 140;
  }

  function startColumnResize(index: number, event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    event.stopPropagation();
    resizeState.current = { index, startX: event.clientX, startWidth: columnWidths?.[index] || 140 };
    event.currentTarget.setPointerCapture(event.pointerId);
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  }

  function finishColumnResize(index: number, event: ReactPointerEvent<HTMLDivElement>) {
    if (!resizeState.current) return;
    onColumnResize?.(index, resizedWidth(index, event), true);
    resizeState.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }
  const definitionColumns: ReportResultColumn[] = definition && dataset
    ? definition.columns.map((column, index) => {
      const field = dataset.fields.find((item) => item.key === column.field);
      const executed = result?.columns[index];
      return executed && executed.field === column.field
        ? executed
        : {
          key: `${column.field}-${column.aggregate || "raw"}-${index}`,
          field: column.field,
          label: column.label || field?.label || column.field,
          type: field?.type || "text",
          format: column.format || field?.formats[0] || "text",
          aggregate: column.aggregate,
          group: column.group
        };
    })
    : (result?.columns || []);

  function handleDragOver(event: ReactDragEvent<HTMLTableCellElement>, index: number) {
    if (!onColumnDrop) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = dragItem?.kind === "column" ? "move" : "copy";
    const box = event.currentTarget.getBoundingClientRect();
    setDropTarget({ index, side: event.clientX < box.left + box.width / 2 ? "before" : "after" });
  }

  function handleDrop(event: ReactDragEvent<HTMLTableCellElement>, index: number) {
    if (!onColumnDrop) return;
    event.preventDefault();
    const target = dropTarget || { index, side: "after" as const };
    onColumnDrop(target.index, target.side, dragItemFromTransfer(event.dataTransfer));
    setDropTarget(null);
  }

  // Progressive disclosure split. Column DRAG-REORDER and RESIZE below operate
  // on the visible set, whose indexes line up with definition.columns because
  // the lead slice is taken from the front.
  const visibleColumns = leadColumnCount > 0 ? definitionColumns.slice(0, leadColumnCount) : definitionColumns;
  const hiddenColumns = leadColumnCount > 0 ? definitionColumns.slice(leadColumnCount) : [];

  return (
    <div className={cn("relative overflow-auto", compact ? "max-h-64" : "max-h-[52vh]")}>
      {loading && result ? <div className="sticky right-2 top-2 z-30 ml-auto flex w-fit items-center gap-1.5 rounded-full border bg-white/95 px-2.5 py-1 text-[10px] font-semibold text-muted-foreground shadow"><Loader2 className="h-3 w-3 animate-spin text-primary" />กำลังอัปเดตข้อมูล</div> : null}
      <table className="min-w-full border-separate border-spacing-0 text-left text-xs">
        <thead className="sticky top-0 z-10 bg-[#f5eadb]">
          <tr>
            <th className="sticky left-0 z-20 w-12 border-b border-r bg-[#f5eadb] px-3 py-2 text-center font-bold">#</th>
            {visibleColumns.map((column, index) => (
              <th
                className={cn(
                  "group relative whitespace-nowrap border-b border-r px-3 py-2.5 font-bold text-foreground",
                  onHeaderMenu && "cursor-context-menu select-none",
                  dragItem?.kind === "column" && dragItem.index === index && "opacity-40"
                )}
                key={column.key}
                onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropTarget(null); }}
                onDragOver={onColumnDrop ? (event) => handleDragOver(event, index) : undefined}
                onDrop={onColumnDrop ? (event) => handleDrop(event, index) : undefined}
                onContextMenu={onHeaderMenu ? (event) => { event.preventDefault(); onHeaderMenu(index, event.clientX, event.clientY); } : undefined}
                onKeyDown={onHeaderMenu ? (event) => {
                  if ((event.shiftKey && event.key === "F10") || event.key === "ContextMenu") {
                    event.preventDefault();
                    const box = event.currentTarget.getBoundingClientRect();
                    onHeaderMenu(index, box.left + 16, box.bottom);
                  }
                } : undefined}
                style={{ minWidth: columnWidths?.[index] || 140, width: columnWidths?.[index] }}
                tabIndex={onHeaderMenu ? 0 : undefined}
              >
                <span className="flex items-center justify-between gap-2">
                  {onColumnDragStart ? (
                    <span
                      aria-label={`ลากคอลัมน์ ${column.label}`}
                      className="flex min-w-0 flex-1 cursor-grab items-center gap-1.5 active:cursor-grabbing"
                      draggable
                      onDragEnd={() => { setDropTarget(null); onColumnDragEnd?.(); }}
                      onDragStart={(event) => onColumnDragStart(index, event)}
                    >
                      <GripVertical className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <span className="truncate">{column.label}</span>
                    </span>
                  ) : <span>{column.label}</span>}
                  {column.aggregate ? <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[9px] uppercase text-primary">{column.aggregate}</span> : null}
                  {column.group ? <span className="rounded bg-amber-200 px-1.5 py-0.5 text-[9px]">GROUP</span> : null}
                  {onHeaderMenu ? (
                    <button
                      aria-label={`เมนูคอลัมน์ ${column.label}`}
                      className="rounded p-0.5 opacity-60 hover:bg-black/10 group-hover:opacity-100 sm:opacity-0"
                      onClick={(event) => { const box = event.currentTarget.getBoundingClientRect(); onHeaderMenu(index, box.left, box.bottom + 4); }}
                      type="button"
                    ><MoreHorizontal className="h-3.5 w-3.5" /></button>
                  ) : null}
                </span>
                {dropTarget?.index === index ? <span className={cn("pointer-events-none absolute inset-y-0 z-30 w-1 bg-primary shadow-[0_0_0_1px_white]", dropTarget.side === "before" ? "left-0" : "right-0")} /> : null}
                {onColumnResize ? (
                  <div
                    aria-label={`ปรับความกว้างคอลัมน์ ${column.label}`}
                    aria-orientation="vertical"
                    className="absolute -right-1 top-0 z-30 h-full w-2 cursor-col-resize touch-none"
                    onKeyDown={(event) => {
                      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
                      event.preventDefault();
                      onColumnResize(index, (columnWidths?.[index] || 140) + (event.key === "ArrowRight" ? 16 : -16), true);
                    }}
                    onPointerCancel={(event) => finishColumnResize(index, event)}
                    onPointerDown={(event) => startColumnResize(index, event)}
                    onPointerMove={(event) => { if (resizeState.current?.index === index) onColumnResize(index, resizedWidth(index, event), false); }}
                    onPointerUp={(event) => finishColumnResize(index, event)}
                    role="separator"
                    tabIndex={0}
                  />
                ) : null}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading && !result ? (
            <tr><td className={cn("text-center text-sm text-muted-foreground", compact ? "h-44" : "h-72")} colSpan={visibleColumns.length + 1}><Loader2 className="mr-2 inline h-5 w-5 animate-spin" />กำลังประมวลผลข้อมูลแบบ read-only</td></tr>
          ) : result ? result.rows.flatMap((row, rowIndex) => [
            <tr
              aria-expanded={hiddenColumns.length ? expandedRow === rowIndex : undefined}
              className={cn(
                "odd:bg-white even:bg-[#fffaf4] hover:bg-amber-50",
                hiddenColumns.length && "cursor-pointer",
                expandedRow === rowIndex && "bg-amber-50"
              )}
              key={rowIndex}
              onClick={hiddenColumns.length ? () => setExpandedRow((current) => (current === rowIndex ? null : rowIndex)) : undefined}
            >
              <td className="sticky left-0 border-b border-r bg-inherit px-3 py-2 text-center font-medium text-muted-foreground">{(result.pagination.page - 1) * result.pagination.page_size + rowIndex + 1}</td>
              {visibleColumns.map((column) => (
                <td className="max-w-[28rem] truncate whitespace-nowrap border-b border-r px-3 py-2" key={column.key} title={formatCell(row[column.key], column)}>{formatCell(row[column.key], column)}</td>
              ))}
            </tr>,
            // PROGRESSIVE DISCLOSURE: the columns that did not fit, shown as a
            // definition list under the row rather than forcing a sideways scroll.
            hiddenColumns.length && expandedRow === rowIndex ? (
              <tr key={`${rowIndex}-detail`}>
                <td className="border-b bg-surface-warm" />
                <td className="border-b bg-surface-warm px-3 py-3" colSpan={visibleColumns.length}>
                  <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
                    {hiddenColumns.map((column) => (
                      <div className="flex min-w-0 gap-2" key={column.key}>
                        <dt className="shrink-0 text-[11px] font-semibold text-muted-foreground">{column.label}</dt>
                        <dd className="min-w-0 truncate text-[11px] font-medium" title={formatCell(row[column.key], column)}>
                          {formatCell(row[column.key], column)}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </td>
              </tr>
            ) : null
          ]) : (
            <tr><td className={cn("px-6 text-center text-sm text-muted-foreground", compact ? "h-44" : "h-72")} colSpan={visibleColumns.length + 1}>ระบบกำลังโหลดรายงานจากค่าเริ่มต้นโดยอัตโนมัติ</td></tr>
          )}
          {result && !result.rows.length ? <TableEmptyState colSpan={visibleColumns.length + 1} description="ไม่พบข้อมูลตามเงื่อนไข" /> : null}
        </tbody>
      </table>
    </div>
  );
}

