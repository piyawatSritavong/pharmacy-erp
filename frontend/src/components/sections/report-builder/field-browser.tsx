"use client";

/**
 * FEATURE: All Fields panel — search across the dataset's 40+ fields, and the
 * DRAG SOURCE half of drag-and-drop. Dragging a field starts a "field" drag;
 * report-grid.tsx is the matching drop target. Also owns its own width resize
 * and collapse-to-rail behaviour.
 */

import { GripVertical, PanelLeftClose, PanelLeftOpen, Plus, Search } from "lucide-react";
import type { DragEvent as ReactDragEvent, PointerEvent as ReactPointerEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { ReportDataset, ReportDefinition, ReportField } from "@/types/report-builder";
import { MAX_FIELD_SIDEBAR_WIDTH, MIN_FIELD_SIDEBAR_WIDTH, relationBadge } from "./shared";

export function FieldBrowser({ collapsed, dataset, definition, sidebarWidth, onAdd, onFieldDragStart, onFieldDragEnd, onResize, onToggleCollapse }: {
  collapsed: boolean;
  dataset: ReportDataset;
  definition: ReportDefinition;
  sidebarWidth: number;
  onAdd: (field: ReportField) => void;
  onFieldDragStart: (field: ReportField, event: ReactDragEvent<HTMLElement>) => void;
  onFieldDragEnd: () => void;
  onResize: (width: number, commit: boolean) => void;
  onToggleCollapse: () => void;
}) {
  const [search, setSearch] = useState("");
  const resizeState = useRef<{ startX: number; startWidth: number } | null>(null);
  const selected = new Set(definition.columns.map((column) => column.field));
  const groups = Array.from(new Set(dataset.fields.map((field) => field.group)));
  const term = search.trim().toLocaleLowerCase("th");

  useEffect(() => () => {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }, []);

  function resizedWidth(event: ReactPointerEvent<HTMLElement>) {
    const state = resizeState.current;
    return state ? state.startWidth + event.clientX - state.startX : sidebarWidth;
  }

  function startResize(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    resizeState.current = { startX: event.clientX, startWidth: sidebarWidth };
    event.currentTarget.setPointerCapture(event.pointerId);
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  }

  function finishResize(event: ReactPointerEvent<HTMLDivElement>) {
    if (!resizeState.current) return;
    onResize(resizedWidth(event), true);
    resizeState.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }

  // D9: same collapse behavior as the side nav bar (Part C) — an icon-only
  // rail with just the toggle, instead of the full searchable field list.
  if (collapsed) {
    return (
      <aside className="flex min-h-0 min-w-0 flex-col items-center border-r bg-white py-3">
        <button
          aria-label="ขยายแผง All Fields"
          className="grid h-9 w-9 place-items-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground"
          onClick={onToggleCollapse}
          title="All Fields"
          type="button"
        >
          <PanelLeftOpen className="h-4 w-4" />
        </button>
      </aside>
    );
  }

  return (
    <aside className="relative flex min-h-0 min-w-0 flex-col border-r bg-white">
      <div className="border-b p-3">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-bold">All Fields</h3>
          <div className="flex items-center gap-2">
            <span className="text-[11px] text-muted-foreground">{dataset.fields.length} fields</span>
            <button aria-label="ย่อแผง All Fields" className="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground hover:bg-muted hover:text-foreground" onClick={onToggleCollapse} type="button">
              <PanelLeftClose className="h-4 w-4" />
            </button>
          </div>
        </div>
        <label className="relative block"><Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input aria-label="ค้นหา field" className="h-9 pl-8" onChange={(event) => setSearch(event.target.value)} placeholder="ค้นหา field..." value={search} /></label>
      </div>
      <div className="max-h-72 flex-1 overflow-y-auto p-2 lg:max-h-none">
        {groups.map((group) => {
          const items = dataset.fields.filter((field) => field.group === group && (!term || `${field.label} ${field.key}`.toLocaleLowerCase("th").includes(term)));
          if (!items.length) return null;
          return (
            <details className="mb-1" key={`${dataset.key}-${group}-${term ? "search" : "default"}`} open={term ? true : undefined}>
              <summary className="cursor-pointer rounded-lg px-2 py-2 text-xs font-bold text-muted-foreground hover:bg-muted">{group} <span className="font-normal">({items.length})</span></summary>
              <div className="space-y-1 pl-1">
                {items.map((field) => {
                  const badge = relationBadge(dataset, field);
                  return (
                    <button
                      className="flex w-full cursor-grab items-start gap-2 rounded-lg px-2 py-2 text-left text-xs hover:bg-amber-50 active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-45"
                      disabled={selected.has(field.key)}
                      draggable={!selected.has(field.key)}
                      key={field.key}
                      onClick={() => onAdd(field)}
                      onDragEnd={onFieldDragEnd}
                      onDragStart={(event) => onFieldDragStart(field, event)}
                      type="button"
                    >
                      <span className="mt-0.5 flex shrink-0 items-center text-primary"><GripVertical className="h-3.5 w-3.5" /><Plus className="h-3 w-3" /></span>
                      <span className="min-w-0"><span className="block font-semibold">{field.label}</span><span className="block truncate text-[10px] text-muted-foreground">{field.key} · {field.type}</span>{badge ? <span className={cn("mt-1 inline-block rounded px-1.5 py-0.5 text-[9px]", badge === "ขยายหลายแถว" ? "bg-red-100 text-red-700" : "bg-emerald-100 text-emerald-700")}>{badge}</span> : null}</span>
                    </button>
                  );
                })}
              </div>
            </details>
          );
        })}
      </div>
      <div
        aria-label="ปรับความกว้าง All Fields"
        aria-orientation="vertical"
        aria-valuemax={MAX_FIELD_SIDEBAR_WIDTH}
        aria-valuemin={MIN_FIELD_SIDEBAR_WIDTH}
        aria-valuenow={Math.round(sidebarWidth)}
        className="group absolute -right-1 top-0 z-30 hidden h-full w-2 cursor-col-resize touch-none lg:block"
        onKeyDown={(event) => {
          if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
          event.preventDefault();
          onResize(sidebarWidth + (event.key === "ArrowRight" ? 16 : -16), true);
        }}
        onPointerCancel={finishResize}
        onPointerDown={startResize}
        onPointerMove={(event) => { if (resizeState.current) onResize(resizedWidth(event), false); }}
        onPointerUp={finishResize}
        role="separator"
        tabIndex={0}
      >
        <span className="absolute bottom-0 left-1/2 top-0 w-px -translate-x-1/2 bg-transparent transition group-hover:bg-primary group-focus:bg-primary" />
      </div>
    </aside>
  );
}

