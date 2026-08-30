"use client";

/**
 * Report builder — state owner and layout.
 *
 * Feature ownership (each lives in its own file so it can be exercised alone):
 *   SAVE / SAVE-AS / DELETE ....... saveCurrent, saveAs, removeReport (below)
 *   PIN / BOOKMARK / REORDER ...... togglePin, movePin (below) + pinned-report-card.tsx
 *   DRAG-AND-DROP ................. startFieldDrag/startColumnDrag/dropOnColumn (below)
 *                                   source: field-browser.tsx · target: report-grid.tsx
 *   ALL FIELDS PANEL .............. field-browser.tsx
 *   ADVANCED FILTERS (AND/OR) ..... filter-editor.tsx
 *   CHART ......................... report-chart.tsx
 *   TABLE + SORT + DISCLOSURE ..... report-grid.tsx, column-menu.tsx
 *
 * Layout follows the redesign: a sticky toolbar, then ตั้งค่ารายงาน as three
 * left-to-right steps (data → time → filters), then the chart, then the table.
 * To add a fourth config step, add a <ConfigStep> to the grid in ตั้งค่ารายงาน —
 * nothing else needs to know about it.
 */

import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CopyPlus,
  Loader2,
  Pin,
  PinOff,
  Plus,
  Save,
  SlidersHorizontal,
  Trash2
} from "lucide-react";
import type { CSSProperties, DragEvent as ReactDragEvent, ReactNode } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Input } from "@/components/ui/input";
import { ErrorState, LoadingState, Notice } from "@/components/ui/state-block";
import { cn } from "@/lib/utils";
import {
  createSavedReport,
  deleteSavedReport,
  executeReport,
  getReportCatalog,
  listSavedReports,
  reorderPinnedReports,
  setSavedReportPinned,
  updateSavedReport
} from "@/services/report-builder";
import type {
  ReportCatalog,
  ReportDataset,
  ReportDefinition,
  ReportField,
  ReportResult,
  SavedReport
} from "@/types/report-builder";
import type { NavigationItem } from "@/types";

import { ColumnContextMenu, type ContextState } from "./column-menu";
import { FieldBrowser } from "./field-browser";
import { FilterGroupEditor } from "./filter-editor";
import { PinnedReportCard } from "./pinned-report-card";
import { Timeline } from "./report-chart";
import { ReportGrid } from "./report-grid";
import {
  COLLAPSED_FIELD_SIDEBAR_WIDTH,
  ConsoleSelect,
  DEFAULT_FIELD_SIDEBAR_WIDTH,
  MAX_FIELD_SIDEBAR_WIDTH,
  MIN_FIELD_SIDEBAR_WIDTH,
  MIN_REPORT_WORKSPACE_WIDTH,
  NUMBER_FORMAT,
  cloneDefinition,
  defaultDefinition,
  ensureEditorIDs,
  fieldLabel,
  filterCount,
  formatRuleValue,
  type ColumnDragItem,
  type ColumnDropSide
} from "./shared";

/** One numbered step in ตั้งค่ารายงาน. Purely presentational — add another and
 *  the grid reflows on its own. */
function ConfigStep({ step, title, hint, children }: { step: number; title: string; hint?: string; children: ReactNode }) {
  return (
    <div className="min-w-0 space-y-3 p-4">
      <div className="flex items-baseline gap-2">
        <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground">
          {step}
        </span>
        <h3 className="text-sm font-bold">{title}</h3>
        {hint ? <span className="truncate text-[11px] text-muted-foreground">{hint}</span> : null}
      </div>
      {children}
    </div>
  );
}

/** Field label above a control inside a step. */
function StepField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[11px] font-semibold text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export function GenerateReportConsole({ navigation = [] }: { navigation?: NavigationItem[] }) {
  // A6 pin-target selector needs one entry per actual page — since Part C
  // grouped the sidebar into parent menus with children, the raw
  // navigation prop is no longer flat (a bare .map here would only offer
  // the 4 top-level groups, hiding every individual page, dashboard
  // included). Flatten group children in alongside top-level leaves.
  const pinnableNavigation = useMemo(
    () =>
      navigation.flatMap((item) =>
        item.children && item.children.length ? item.children : [item]
      ),
    [navigation]
  );
  const [catalog, setCatalog] = useState<ReportCatalog | null>(null);
  const [reports, setReports] = useState<SavedReport[]>([]);
  const [definition, setDefinition] = useState<ReportDefinition | null>(null);
  const [selectedReportID, setSelectedReportID] = useState("");
  const [reportName, setReportName] = useState("");
  const [description, setDescription] = useState("");
  const [result, setResult] = useState<ReportResult | null>(null);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [dirty, setDirty] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [context, setContext] = useState<ContextState>(null);
  const [columnWidths, setColumnWidths] = useState<Record<number, number>>({});
  const [fieldSidebarWidth, setFieldSidebarWidth] = useState(DEFAULT_FIELD_SIDEBAR_WIDTH);
  // D9: same collapse behavior as the side nav bar (Part C) — shrink to an
  // icon-only rail instead of the full searchable field list.
  const [fieldSidebarCollapsed, setFieldSidebarCollapsed] = useState(false);
  const [dragItem, setDragItem] = useState<ColumnDragItem>(null);
  const dragItemRef = useRef<ColumnDragItem>(null);
  // A6: which page a pinned report renders on — defaults to this page itself.
  const [pinTargetKey, setPinTargetKey] = useState("generate_report");
  const [saveAsOpen, setSaveAsOpen] = useState(false);
  const [saveAsName, setSaveAsName] = useState("");
  const [saveAsDescription, setSaveAsDescription] = useState("");
  const builderLayoutRef = useRef<HTMLDivElement>(null);

  const refreshReports = useCallback(async () => {
    const response = await listSavedReports();
    setReports(response.items);
    return response.items;
  }, []);

  useEffect(() => {
    Promise.all([getReportCatalog(), listSavedReports()]).then(([catalogResponse, reportResponse]) => {
      setCatalog(catalogResponse);
      setReports(reportResponse.items);
      const dataset = catalogResponse.datasets[0];
      const next = defaultDefinition(catalogResponse, dataset);
      setDefinition(next);
      setFieldSidebarWidth(next.layout?.field_sidebar_width || DEFAULT_FIELD_SIDEBAR_WIDTH);
    }).catch((reason) => setError(reason instanceof Error ? reason.message : "โหลด Generate Report ไม่สำเร็จ")).finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    function warn(event: BeforeUnloadEvent) { if (dirty) { event.preventDefault(); event.returnValue = ""; } }
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  useEffect(() => {
    function guardLink(event: MouseEvent) {
      if (!dirty || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = (event.target as HTMLElement).closest("a");
      if (!anchor || anchor.target === "_blank" || !anchor.href || new URL(anchor.href).pathname === window.location.pathname) return;
      if (!window.confirm("มีการแก้ไขรายงานที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่?")) event.preventDefault();
    }
    document.addEventListener("click", guardLink, true);
    return () => document.removeEventListener("click", guardLink, true);
  }, [dirty]);

  const dataset = useMemo(() => catalog?.datasets.find((item) => item.key === definition?.dataset_key), [catalog, definition?.dataset_key]);
  const pinned = useMemo(
    () =>
      reports
        .filter((report) => report.is_pinned && (report.pin_target_key || "generate_report") === "generate_report")
        .sort((a, b) => (a.pin_order ?? 0) - (b.pin_order ?? 0)),
    [reports]
  );
  const queryKey = useMemo(() => {
    if (!definition) return "";
    const queryDefinition = cloneDefinition(definition);
    delete queryDefinition.layout;
    return JSON.stringify(queryDefinition);
  }, [definition]);

  useEffect(() => {
    if (loading || !queryKey) return;
    const controller = new AbortController();
    setRunning(true);
    setError("");
    const timer = window.setTimeout(() => {
      const queryDefinition = JSON.parse(queryKey) as ReportDefinition;
      executeReport({ definition: queryDefinition, page }, { signal: controller.signal }).then((response) => {
        if (controller.signal.aborted) return;
        setResult(response);
      }).catch((reason) => {
        if (controller.signal.aborted) return;
        const message = reason instanceof Error ? reason.message : "อัปเดตรายงานอัตโนมัติไม่สำเร็จ";
        setError(message);
        toast.error(message);
      }).finally(() => {
        if (!controller.signal.aborted) setRunning(false);
      });
    }, 350);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [loading, page, queryKey]);

  function changeDefinition(next: ReportDefinition) {
    const currentOrder = definition?.columns.map((column) => column.field).join("\u0000");
    const nextOrder = next.columns.map((column) => column.field).join("\u0000");
    if (currentOrder !== nextOrder) setColumnWidths({});
    setDefinition(next);
    setDirty(true);
    setPage(1);
  }

  function mutateDefinition(mutator: (next: ReportDefinition) => void) {
    if (!definition) return;
    const next = cloneDefinition(definition);
    mutator(next);
    changeDefinition(next);
  }

  function normalizedSidebarWidth(reportDefinition: ReportDefinition) {
    const width = reportDefinition.layout?.field_sidebar_width || DEFAULT_FIELD_SIDEBAR_WIDTH;
    return Math.max(MIN_FIELD_SIDEBAR_WIDTH, Math.min(MAX_FIELD_SIDEBAR_WIDTH, width));
  }

  function constrainSidebarWidth(width: number) {
    const available = builderLayoutRef.current?.getBoundingClientRect().width;
    const responsiveMaximum = available
      ? Math.max(MIN_FIELD_SIDEBAR_WIDTH, Math.min(MAX_FIELD_SIDEBAR_WIDTH, available - MIN_REPORT_WORKSPACE_WIDTH))
      : MAX_FIELD_SIDEBAR_WIDTH;
    return Math.round(Math.max(MIN_FIELD_SIDEBAR_WIDTH, Math.min(responsiveMaximum, width)));
  }

  function resizeFieldSidebar(width: number, commit: boolean) {
    const nextWidth = constrainSidebarWidth(width);
    setFieldSidebarWidth(nextWidth);
    if (!commit) return;
    setDefinition((current) => current ? { ...current, layout: { field_sidebar_width: nextWidth } } : current);
    if (definition?.layout?.field_sidebar_width !== nextWidth) setDirty(true);
  }

  function addField(field: ReportField) {
    if (!definition || definition.columns.some((column) => column.field === field.key)) {
      toast.error("Field นี้อยู่ในตารางแล้ว");
      return;
    }
    if (catalog && definition.columns.length >= (catalog.limits.max_columns || 50)) {
      toast.error("จำนวนคอลัมน์ถึงขีดจำกัดแล้ว");
      return;
    }
    mutateDefinition((next) => next.columns.push({ field: field.key }));
  }

  function startFieldDrag(field: ReportField, event: ReactDragEvent<HTMLElement>) {
    event.dataTransfer.effectAllowed = "copy";
    event.dataTransfer.setData("text/plain", `report-field:${field.key}`);
    const item: ColumnDragItem = { kind: "field", field: field.key };
    dragItemRef.current = item;
    setDragItem(item);
  }

  function startColumnDrag(index: number, event: ReactDragEvent<HTMLElement>) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", `report-column:${index}`);
    const item: ColumnDragItem = { kind: "column", index };
    dragItemRef.current = item;
    setDragItem(item);
  }

  function finishDrag() {
    dragItemRef.current = null;
    setDragItem(null);
  }

  function dropOnColumn(targetIndex: number, side: ColumnDropSide, transferredItem?: ColumnDragItem) {
    const activeDragItem = transferredItem || dragItemRef.current || dragItem;
    if (!definition || !activeDragItem) return;
    if (activeDragItem.kind === "field") {
      if (definition.columns.some((column) => column.field === activeDragItem.field)) {
        toast.error("Field นี้อยู่ในตารางแล้ว");
        finishDrag();
        return;
      }
      if (catalog && definition.columns.length >= (catalog.limits.max_columns || 50)) {
        toast.error("จำนวนคอลัมน์ถึงขีดจำกัดแล้ว");
        finishDrag();
        return;
      }
      mutateDefinition((next) => next.columns.splice(targetIndex + (side === "after" ? 1 : 0), 0, { field: activeDragItem.field }));
    } else {
      const insertionPoint = targetIndex + (side === "after" ? 1 : 0);
      const adjustedInsertionPoint = activeDragItem.index < insertionPoint ? insertionPoint - 1 : insertionPoint;
      if (adjustedInsertionPoint === activeDragItem.index) {
        finishDrag();
        return;
      }
      mutateDefinition((next) => {
        const [column] = next.columns.splice(activeDragItem.index, 1);
        next.columns.splice(adjustedInsertionPoint, 0, column);
      });
    }
    finishDrag();
  }

  function loadReport(report: SavedReport) {
    if (dirty && !window.confirm("มีการแก้ไขที่ยังไม่ได้บันทึก ต้องการเปลี่ยนรายงานหรือไม่?")) return;
    setSelectedReportID(report.id);
    setReportName(report.name);
    setDescription(report.description);
    const next = { ...cloneDefinition(report.definition), filters: ensureEditorIDs(report.definition.filters) };
    const width = normalizedSidebarWidth(next);
    next.layout = { field_sidebar_width: width };
    setDefinition(next);
    setFieldSidebarWidth(width);
    setPage(1);
    setDirty(false);
    requestAnimationFrame(() => document.getElementById("report-builder")?.scrollIntoView({ behavior: "smooth", block: "start" }));
  }

  function newReport() {
    if (!catalog) return;
    if (dirty && !window.confirm("ละทิ้งการแก้ไขที่ยังไม่ได้บันทึกหรือไม่?")) return;
    setSelectedReportID("");
    setReportName("");
    setDescription("");
    const next = defaultDefinition(catalog, catalog.datasets[0]);
    setDefinition(next);
    setFieldSidebarWidth(DEFAULT_FIELD_SIDEBAR_WIDTH);
    setPage(1);
    setDirty(false);
  }

  async function saveCurrent() {
    if (!definition || !reportName.trim()) { toast.error("กรุณาตั้งชื่อรายงานก่อนบันทึก"); return; }
    setSaving(true);
    try {
      const payload = { name: reportName.trim(), description: description.trim(), definition };
      const saved = selectedReportID ? await updateSavedReport(selectedReportID, payload) : await createSavedReport(payload);
      await refreshReports();
      setSelectedReportID(saved.id);
      setReportName(saved.name);
      setDescription(saved.description);
      setDefinition({ ...cloneDefinition(saved.definition), filters: ensureEditorIDs(saved.definition.filters) });
      setFieldSidebarWidth(normalizedSidebarWidth(saved.definition));
      setDirty(false);
      toast.success(selectedReportID ? "บันทึกการแก้ไขรายงานแล้ว" : "สร้างรายงานแล้ว");
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "บันทึกรายงานไม่สำเร็จ"); } finally { setSaving(false); }
  }

  async function saveAs() {
    if (!definition || !saveAsName.trim()) { toast.error("กรุณาตั้งชื่อรายงานใหม่"); return; }
    setSaving(true);
    try {
      const saved = await createSavedReport({ name: saveAsName.trim(), description: saveAsDescription.trim(), definition });
      await refreshReports();
      setSelectedReportID(saved.id);
      setReportName(saved.name);
      setDescription(saved.description);
      setDefinition({ ...cloneDefinition(saved.definition), filters: ensureEditorIDs(saved.definition.filters) });
      setFieldSidebarWidth(normalizedSidebarWidth(saved.definition));
      setDirty(false);
      setSaveAsOpen(false);
      toast.success("บันทึกเป็นรายงานใหม่แล้ว");
    } catch (reason) { toast.error(reason instanceof Error ? reason.message : "บันทึกรายงานใหม่ไม่สำเร็จ"); } finally { setSaving(false); }
  }

  async function togglePin(report: SavedReport, pinnedState: boolean, targetKey?: string) {
    try {
      await setSavedReportPinned(report.id, pinnedState, pinnedState ? targetKey || pinTargetKey : undefined);
      await refreshReports();
      toast.success(pinnedState ? "ปักหมุดรายงานแล้ว" : "ถอนหมุดรายงานแล้ว");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "เปลี่ยนสถานะหมุดไม่สำเร็จ");
    }
  }

  async function removeReport() {
    if (!selectedReportID || !window.confirm(`ลบรายงาน “${reportName}” หรือไม่?`)) return;
    try {
      await deleteSavedReport(selectedReportID);
      await refreshReports();
      if (catalog) {
        setSelectedReportID("");
        setReportName("");
        setDescription("");
        setDefinition(defaultDefinition(catalog, catalog.datasets[0]));
        setFieldSidebarWidth(DEFAULT_FIELD_SIDEBAR_WIDTH);
        setResult(null);
        setPage(1);
        setDirty(false);
      }
      toast.success("ลบรายงานแล้ว");
    }
    catch (reason) { toast.error(reason instanceof Error ? reason.message : "ลบรายงานไม่สำเร็จ"); }
  }

  async function movePin(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= pinned.length) return;
    const next = [...pinned];
    [next[index], next[target]] = [next[target], next[index]];
    setReports((current) => current.map((report) => {
      const pinIndex = next.findIndex((item) => item.id === report.id);
      return pinIndex >= 0 ? { ...report, pin_order: pinIndex } : report;
    }));
    try { await reorderPinnedReports(next.map((item) => item.id), "generate_report"); await refreshReports(); }
    catch (reason) { await refreshReports(); toast.error(reason instanceof Error ? reason.message : "จัดลำดับหมุดไม่สำเร็จ"); }
  }

  if (loading) return <LoadingState label="กำลังเตรียม Semantic Report Engine" />;
  if (!catalog || !definition || !dataset)
    return <ErrorState description={error || "ไม่พบ catalog ของรายงาน"} title="เปิด Generate Report ไม่สำเร็จ" />;

  const activeSaved = reports.find((report) => report.id === selectedReportID);
  const hasAggregate = definition.columns.some((column) => column.aggregate || column.group);

  // How many columns stay inline before the rest fold into the row detail.
  // Wide reports become unreadable long before they run out of columns.
  const leadColumnCount = definition.columns.length > 8 ? 7 : 0;

  return (
    <div className="text-foreground">
      <div className="space-y-5">
        {/* ---------- PIN / BOOKMARK (read side) ---------- */}
        {pinned.length ? (
          <section aria-labelledby="pinned-reports-heading" className="space-y-3">
            <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
              <h2 className="text-xl font-semibold tracking-tight" id="pinned-reports-heading">รายงานหลักของฉัน</h2>
              <span className="text-xs text-muted-foreground">โหลดเมื่อเลื่อนมาถึง · เรียงบนลงล่าง</span>
            </div>
            {pinned.map((report, index) => (
              <PinnedReportCard
                canMoveDown={index < pinned.length - 1}
                canMoveUp={index > 0}
                dataset={catalog.datasets.find((item) => item.key === report.definition.dataset_key)}
                key={`${report.id}-${report.updated_at}`}
                onEdit={() => loadReport(report)}
                onMove={(direction) => movePin(index, direction)}
                onUnpin={() => togglePin(report, false)}
                report={report}
              />
            ))}
          </section>
        ) : (
          <section className="rounded-2xl border border-dashed">
            <EmptyState description="สร้างและบันทึกรายงานด้านล่าง แล้วกด “ปักหมุด” เพื่อให้เปิดมาพบรายงานนี้ทุกครั้ง" icon={Pin} />
          </section>
        )}

        <section className="scroll-mt-20 space-y-4" id="report-builder">
          {/* ---------- SAVE / SAVE-AS / PIN / DELETE toolbar ---------- */}
          <div className="sticky top-0 z-30 rounded-2xl border bg-[#211f1c] px-4 py-3 text-white shadow-card">
            <div className="flex flex-wrap items-center gap-2">
              <ConsoleSelect
                ariaLabel="รายงานที่บันทึก"
                className="min-w-48"
                onChange={(id) => { const report = reports.find((item) => item.id === id); if (report) loadReport(report); else newReport(); }}
                value={selectedReportID}
              >
                <option value="">รายงานใหม่</option>
                {reports.map((report) => <option key={report.id} value={report.id}>{report.is_pinned ? "📌 " : ""}{report.name}</option>)}
              </ConsoleSelect>
              <Button className="h-9 px-3" onClick={newReport} type="button" variant="secondary"><Plus className="h-4 w-4" />ใหม่</Button>
              <Button className="h-9 px-3" disabled={saving} onClick={saveCurrent} type="button"><Save className="h-4 w-4" />บันทึก</Button>
              <Button className="h-9 px-3" onClick={() => { setSaveAsName(`${reportName || "รายงานใหม่"} - สำเนา`); setSaveAsDescription(description); setSaveAsOpen(true); }} type="button" variant="secondary"><CopyPlus className="h-4 w-4" />Save As</Button>
              {!activeSaved?.is_pinned && pinnableNavigation.length ? (
                <ConsoleSelect ariaLabel="ปักหมุดไปที่หน้า" className="min-w-40" onChange={setPinTargetKey} value={pinTargetKey}>
                  {pinnableNavigation.map((item) => (
                    <option key={item.key} value={item.key}>{item.key === "generate_report" ? "ปักที่หน้านี้" : `ปักที่ ${item.title}`}</option>
                  ))}
                </ConsoleSelect>
              ) : null}
              <Button className="h-9 px-3" disabled={!activeSaved} onClick={() => activeSaved && togglePin(activeSaved, !activeSaved.is_pinned, pinTargetKey)} type="button" variant="secondary">
                {activeSaved?.is_pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}{activeSaved?.is_pinned ? "ถอนหมุด" : "ปักหมุด"}
              </Button>
              <Button aria-label="ลบรายงาน" className="h-9 px-3 text-white/70 hover:bg-white/10 hover:text-white active:bg-white/20" disabled={!selectedReportID} onClick={removeReport} type="button" variant="ghost"><Trash2 className="h-4 w-4" /></Button>
              <span aria-live="polite" className="ml-auto flex h-9 items-center gap-2 rounded-full border border-white/15 bg-white/10 px-3 text-[11px] font-semibold text-white/80">
                {running ? <><Loader2 className="h-3.5 w-3.5 animate-spin" />กำลังอัปเดต</> : <><span className="h-1.5 w-1.5 rounded-full bg-primary" />อัปเดตอัตโนมัติ</>}
              </span>
            </div>
            <div className="mt-3 grid gap-3 lg:grid-cols-2">
              <label className="space-y-1"><span className="text-[11px] font-bold text-white/65">ชื่อรายงาน</span><Input aria-label="ชื่อรายงาน" className="h-9 border-white/15 bg-white/10 text-white placeholder:text-white/40" onChange={(event) => { setReportName(event.target.value); setDirty(true); }} placeholder="เช่น ถุงมือทุกสาขา" value={reportName} /></label>
              <label className="space-y-1"><span className="text-[11px] font-bold text-white/65">คำอธิบาย</span><Input aria-label="คำอธิบายรายงาน" className="h-9 border-white/15 bg-white/10 text-white placeholder:text-white/40" onChange={(event) => { setDescription(event.target.value); setDirty(true); }} placeholder="วัตถุประสงค์ของรายงาน" value={description} /></label>
            </div>
          </div>

          {/* ---------- ตั้งค่ารายงาน: three steps, left to right ---------- */}
          <div className="overflow-hidden rounded-2xl border bg-white shadow-card">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b bg-surface-warm px-4 py-3">
              <h2 className="text-sm font-bold">ตั้งค่ารายงาน</h2>
              <span className="text-xs text-muted-foreground">ทำ 3 ขั้นตอนจากซ้ายไปขวา</span>
              <span className="ml-auto truncate text-xs text-muted-foreground">
                {dataset.label} · {definition.columns.length} คอลัมน์ · ตัวกรอง {filterCount(definition.filters)}
              </span>
            </div>
            <div className="grid divide-y bg-border md:grid-cols-2 md:divide-y-0 xl:grid-cols-3 [&>*]:bg-white md:[&>*+*]:border-l">
              {/* STEP 1 — dataset */}
              <ConfigStep hint="แหล่งข้อมูลตั้งต้น" step={1} title="เลือกข้อมูล">
                <StepField label="ชุดข้อมูลหลัก">
                  <ConsoleSelect
                    ariaLabel="ชุดข้อมูลหลัก"
                    className="w-full"
                    onChange={(key) => {
                      const nextDataset = catalog.datasets.find((item) => item.key === key);
                      if (nextDataset && (!dirty || window.confirm("เปลี่ยน dataset และเริ่มโครงรายงานใหม่หรือไม่?"))) {
                        const next = defaultDefinition(catalog, nextDataset);
                        setFieldSidebarWidth(DEFAULT_FIELD_SIDEBAR_WIDTH);
                        changeDefinition(next);
                      }
                    }}
                    value={definition.dataset_key}
                  >
                    {catalog.datasets.map((item) => <option key={item.key} value={item.key}>{item.label}</option>)}
                  </ConsoleSelect>
                </StepField>
                <p className="text-[11px] leading-5 text-muted-foreground">{dataset.description} · Grain: {dataset.grain}</p>
              </ConfigStep>

              {/* STEP 2 — time range + bucket granularity */}
              <ConfigStep hint="ไม่บังคับ" step={2} title="เลือกช่วงเวลา">
                <div className="grid gap-3 sm:grid-cols-2">
                  <StepField label="อ้างอิงเวลาจาก">
                    <ConsoleSelect
                      ariaLabel="ฟิลด์เวลา"
                      className="w-full"
                      onChange={(field) => mutateDefinition((next) => { next.time_config = field ? { ...(next.time_config || {}), field, range_type: next.time_config?.range_type || "relative", relative: next.time_config?.relative || "last_30_days", bucket: next.time_config?.bucket || "auto" } : undefined; })}
                      value={definition.time_config?.field || ""}
                    >
                      <option value="">ไม่ใช้ Timeline</option>
                      {dataset.fields.filter((field) => field.type === "date" || field.type === "datetime").map((field) => <option key={field.key} value={field.key}>{field.label}</option>)}
                    </ConsoleSelect>
                  </StepField>
                  <StepField label="ช่วงที่ต้องการดู">
                    <ConsoleSelect
                      ariaLabel="ช่วงเวลา"
                      className="w-full"
                      disabled={!definition.time_config?.field}
                      onChange={(value) => mutateDefinition((next) => { if (next.time_config) next.time_config.range_type = value as "all" | "relative" | "custom"; })}
                      value={definition.time_config?.range_type || "all"}
                    >
                      <option value="all">ทั้งหมด</option>
                      <option value="relative">ช่วงเวลาสำเร็จรูป</option>
                      <option value="custom">กำหนดเอง</option>
                    </ConsoleSelect>
                  </StepField>
                  {definition.time_config?.range_type === "relative" ? (
                    <StepField label="ช่วงสำเร็จรูป">
                      <ConsoleSelect ariaLabel="ช่วงเวลาสำเร็จรูป" className="w-full" onChange={(value) => mutateDefinition((next) => { if (next.time_config) next.time_config.relative = value as NonNullable<ReportDefinition["time_config"]>["relative"]; })} value={definition.time_config.relative || "last_30_days"}>
                        <option value="today">วันนี้</option>
                        <option value="last_7_days">7 วันล่าสุด</option>
                        <option value="this_week">สัปดาห์นี้</option>
                        <option value="this_month">เดือนนี้</option>
                        <option value="last_30_days">30 วันล่าสุด</option>
                      </ConsoleSelect>
                    </StepField>
                  ) : null}
                  {definition.time_config?.range_type === "custom" ? (
                    <>
                      <StepField label="เวลาเริ่มต้น"><Input aria-label="เวลาเริ่มต้น" className="h-9" onChange={(event) => mutateDefinition((next) => { if (next.time_config) next.time_config.start = event.target.value; })} type="datetime-local" value={definition.time_config.start || ""} /></StepField>
                      <StepField label="เวลาสิ้นสุด"><Input aria-label="เวลาสิ้นสุด" className="h-9" onChange={(event) => mutateDefinition((next) => { if (next.time_config) next.time_config.end = event.target.value; })} type="datetime-local" value={definition.time_config.end || ""} /></StepField>
                    </>
                  ) : null}
                  <StepField label="รวมข้อมูลแบบ">
                    <ConsoleSelect ariaLabel="Timeline bucket" className="w-full" disabled={!definition.time_config?.field} onChange={(value) => mutateDefinition((next) => { if (next.time_config) next.time_config.bucket = value as NonNullable<ReportDefinition["time_config"]>["bucket"]; })} value={definition.time_config?.bucket || "auto"}>
                      {catalog.timeline_buckets.map((bucket) => <option key={bucket.key} value={bucket.key}>{bucket.label}</option>)}
                    </ConsoleSelect>
                  </StepField>
                </div>
              </ConfigStep>

              {/* STEP 3 — ADVANCED FILTERS (AND/OR), collapsed until needed */}
              <ConfigStep hint="ไม่บังคับ" step={3} title="เพิ่มตัวกรอง">
                <button className="flex w-full items-center gap-2 rounded-xl border bg-surface-warm px-3 py-2 text-left text-xs font-bold hover:bg-muted" onClick={() => setShowAdvanced((current) => !current)} type="button">
                  <SlidersHorizontal className="h-4 w-4 text-primary" />
                  ตัวกรองขั้นสูง ({filterCount(definition.filters)})
                  <ChevronDown className={cn("ml-auto h-4 w-4 transition", showAdvanced && "rotate-180")} />
                </button>
                {showAdvanced ? null : definition.filters.rules.length || definition.filters.groups.length ? (
                  <div className="flex flex-wrap gap-1.5">
                    {definition.filters.rules.slice(0, 6).map((rule) => (
                      <span className="rounded-full bg-amber-100 px-2.5 py-1 text-[10px] font-semibold" key={rule.id}>{fieldLabel(dataset, rule.field)} {rule.operator} {formatRuleValue(rule)}</span>
                    ))}
                    {filterCount(definition.filters) > 6 ? <span className="rounded-full bg-muted px-2.5 py-1 text-[10px]">+{filterCount(definition.filters) - 6}</span> : null}
                  </div>
                ) : (
                  <p className="text-[11px] text-muted-foreground">ยังไม่มีตัวกรอง — รายงานจะแสดงทุกแถวของชุดข้อมูล</p>
                )}
              </ConfigStep>
            </div>
            {/* The AND/OR editor is full width: nested groups need the room. */}
            {showAdvanced ? (
              <div className="border-t bg-surface-warm p-4">
                <FilterGroupEditor catalog={catalog} dataset={dataset} depth={0} group={definition.filters} onChange={(filters) => mutateDefinition((next) => { next.filters = filters; })} />
              </div>
            ) : null}
          </div>

          {hasAggregate && definition.columns.some((column) => !column.aggregate && !column.group) ? (
            <Notice tone="warning">เมื่อใช้ Aggregate ต้องตั้ง Group By ให้ทุกคอลัมน์ที่ไม่ได้คำนวณ ระบบจะปฏิเสธ query ที่ grain ไม่ชัดเจน</Notice>
          ) : null}
          {error ? <Notice tone="error">{error}</Notice> : null}

          {/* ---------- CHART ---------- */}
          <div className="overflow-hidden rounded-2xl border bg-white shadow-card">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-3">
              {/* Bucket and row count are drawn inside Timeline itself — the
                  header only names the section and explains an empty chart. */}
              <h2 className="text-sm font-bold">แนวโน้มตามช่วงเวลา</h2>
              {definition.time_config?.field ? null : (
                <span className="text-xs text-muted-foreground">เลือก “อ้างอิงเวลาจาก” ในขั้นตอนที่ 2 เพื่อดูกราฟ</span>
              )}
            </div>
            <Timeline dataset={dataset} definition={definition} result={result} />
          </div>

          {/* ---------- ALL FIELDS (drag source) + TABLE (drop target) ---------- */}
          <div className="overflow-hidden rounded-2xl border bg-white shadow-card">
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-3">
              <h2 className="text-sm font-bold">รายการข้อมูล</h2>
              <span className="text-xs text-muted-foreground">
                {leadColumnCount ? "คลิกที่แถวเพื่อดูคอลัมน์ที่เหลือทั้งหมด" : "ลากฟิลด์จากแถบซ้ายมาวางบนหัวตารางเพื่อเพิ่มคอลัมน์"}
              </span>
            </div>
            <div
              className="report-builder-workspace grid min-h-[560px]"
              ref={builderLayoutRef}
              style={{ "--field-sidebar-width": `${fieldSidebarCollapsed ? COLLAPSED_FIELD_SIDEBAR_WIDTH : fieldSidebarWidth}px` } as CSSProperties}
            >
              <FieldBrowser collapsed={fieldSidebarCollapsed} dataset={dataset} definition={definition} onAdd={addField} onFieldDragEnd={finishDrag} onFieldDragStart={startFieldDrag} onResize={resizeFieldSidebar} onToggleCollapse={() => setFieldSidebarCollapsed((current) => !current)} sidebarWidth={fieldSidebarWidth} />
              <div className="flex min-w-0 flex-col">
                <ReportGrid columnWidths={columnWidths} dataset={dataset} definition={definition} dragItem={dragItem} leadColumnCount={leadColumnCount} loading={running} onColumnDragEnd={finishDrag} onColumnDragStart={startColumnDrag} onColumnDrop={dropOnColumn} onHeaderMenu={(index) => setContext({ index })} result={result} />
                <footer className="mt-auto flex flex-wrap items-center gap-3 border-t bg-white px-3 py-2.5 text-xs">
                  <span>{result ? `${NUMBER_FORMAT.format(result.pagination.total)} แถว · ${result.meta.duration_ms} ms${result.meta.read_only ? " · read-only" : ""}${running ? " · กำลังอัปเดต" : ""}` : `${definition.columns.length} columns · กำลังโหลดอัตโนมัติ`}</span>
                  <label className="ml-auto flex items-center gap-2">แถวต่อหน้า<ConsoleSelect ariaLabel="แถวต่อหน้า" className="h-8 w-24" onChange={(value) => mutateDefinition((next) => { next.page_size = Number(value); })} value={String(definition.page_size)}>{catalog.page_sizes.map((size) => <option key={size} value={size}>{size}</option>)}</ConsoleSelect></label>
                  <button aria-label="หน้าก่อน" className="rounded-lg border p-2 hover:bg-muted disabled:opacity-30" disabled={!result || page <= 1 || running} onClick={() => setPage((current) => Math.max(1, current - 1))} type="button"><ChevronLeft className="h-3.5 w-3.5" /></button>
                  <span>หน้า {result?.pagination.page || page} / {result?.pagination.total_pages || 1}</span>
                  <button aria-label="หน้าถัดไป" className="rounded-lg border p-2 hover:bg-muted disabled:opacity-30" disabled={!result || page >= result.pagination.total_pages || running} onClick={() => setPage((current) => current + 1)} type="button"><ChevronRight className="h-3.5 w-3.5" /></button>
                </footer>
              </div>
            </div>
          </div>
        </section>
      </div>

      {/* ---------- column header menu (label / aggregate / group / sort) ---------- */}
      {context && definition.columns[context.index] ? (
        <ColumnContextMenu
          dataset={dataset}
          definition={definition}
          onAutoFit={(index) => setColumnWidths((current) => ({ ...current, [index]: Math.max(120, Math.min(360, ((definition.columns[index].label || fieldLabel(dataset, definition.columns[index].field)).length + 6) * 10)) }))}
          onChange={changeDefinition}
          onClose={() => setContext(null)}
          state={context}
        />
      ) : null}

      {/* ---------- SAVE AS ---------- */}
      <Dialog onOpenChange={setSaveAsOpen} open={saveAsOpen}>
        <DialogContent>
          <DialogHeader description="สร้างสำเนาที่แก้ไขต่อได้โดยไม่กระทบรายงานเดิม" title="Save Report As" />
          <div className="space-y-4">
            <label className="block space-y-1.5 text-sm font-semibold">ชื่อรายงานใหม่<Input aria-label="ชื่อรายงานใหม่" onChange={(event) => setSaveAsName(event.target.value)} value={saveAsName} /></label>
            <label className="block space-y-1.5 text-sm font-semibold">คำอธิบาย<Input aria-label="คำอธิบายรายงานใหม่" onChange={(event) => setSaveAsDescription(event.target.value)} value={saveAsDescription} /></label>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setSaveAsOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={saving || !saveAsName.trim()} onClick={saveAs} type="button">{saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}บันทึกเป็นรายงานใหม่</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
