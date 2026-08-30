"use client";

/**
 * FEATURE: per-column header menu — label, aggregate, group-by, sort direction,
 * auto-fit width and removal. Sorting a column is applied from here and from a
 * header click in report-grid.tsx; both funnel through onChange.
 */

import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Filter, Trash2, X } from "lucide-react";
import { useState } from "react";

import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { ReportDataset, ReportDefinition } from "@/types/report-builder";
import { ConsoleSelect, FieldOptions, cloneDefinition, defaultOperator, fieldLabel, fieldMap, uid } from "./shared";

export type ContextState = { index: number } | null;

export function ColumnContextMenu({ state, dataset, definition, onClose, onChange, onAutoFit }: {
  state: NonNullable<ContextState>;
  dataset: ReportDataset;
  definition: ReportDefinition;
  onClose: () => void;
  onChange: (definition: ReportDefinition) => void;
  onAutoFit: (index: number) => void;
}) {
  const column = definition.columns[state.index];
  const field = dataset.fields.find((item) => item.key === column.field) || dataset.fields[0];
  const [insertField, setInsertField] = useState(dataset.fields.find((item) => !definition.columns.some((column) => column.field === item.key))?.key || dataset.fields[0].key);
  const [replacement, setReplacement] = useState(column.field);
  const [label, setLabel] = useState(column.label || field.label);

  function apply(mutator: (next: ReportDefinition) => void, close = true) {
    const next = cloneDefinition(definition);
    mutator(next);
    onChange(next);
    if (close) onClose();
  }
  function setSort(direction?: "asc" | "desc") {
    apply((next) => {
      next.sorts = next.sorts.filter((sort) => !(sort.field === column.field && (sort.aggregate || "") === (column.aggregate || "")));
      if (direction) next.sorts.unshift({ field: column.field, direction, aggregate: column.aggregate });
      next.sorts = next.sorts.slice(0, 5);
    });
  }
  function move(offset: number) {
    apply((next) => {
      const target = state.index + offset;
      if (target < 0 || target >= next.columns.length) return;
      [next.columns[state.index], next.columns[target]] = [next.columns[target], next.columns[state.index]];
    });
  }
  return (
    <Dialog onOpenChange={(open) => { if (!open) onClose(); }} open>
      <DialogContent className="max-h-[calc(100vh-2rem)] max-w-lg overflow-hidden p-0 text-xs">
        <div aria-label={`การทำงานคอลัมน์ ${field.label}`}>
          <div className="border-b bg-[#f5eadb] px-4 pt-4">
            <DialogHeader
              actions={<button aria-label="ปิดเมนูคอลัมน์" className="rounded-lg p-2 hover:bg-black/5" onClick={onClose} type="button"><X className="h-4 w-4" /></button>}
              description={`${field.group} · ${field.type}`}
              title={column.label || field.label}
            />
          </div>
          <div className="max-h-[calc(100vh-8rem)] space-y-3 overflow-y-auto p-4">
        <section><p className="mb-1.5 font-bold">เพิ่ม / แทนที่คอลัมน์</p><div className="flex gap-1"><ConsoleSelect ariaLabel="field สำหรับเพิ่ม" className="min-w-0 flex-1" onChange={setInsertField} value={insertField}><FieldOptions dataset={dataset} exclude={definition.columns.map((item) => item.field)} /></ConsoleSelect><button aria-label="เพิ่มคอลัมน์ด้านซ้าย" className="rounded-lg border p-2 hover:bg-muted" onClick={() => apply((next) => next.columns.splice(state.index, 0, { field: insertField }))} type="button"><ArrowLeft className="h-4 w-4" /></button><button aria-label="เพิ่มคอลัมน์ด้านขวา" className="rounded-lg border p-2 hover:bg-muted" onClick={() => apply((next) => next.columns.splice(state.index + 1, 0, { field: insertField }))} type="button"><ArrowRight className="h-4 w-4" /></button></div><div className="mt-1.5 flex gap-1"><ConsoleSelect ariaLabel="field สำหรับแทนที่" className="min-w-0 flex-1" onChange={setReplacement} value={replacement}><FieldOptions dataset={dataset} /></ConsoleSelect><button className="rounded-lg border px-3 font-semibold hover:bg-muted" onClick={() => apply((next) => { next.columns[state.index] = { field: replacement }; })} type="button">แทนที่</button></div></section>
        <section className="grid grid-cols-3 gap-1 border-y py-3"><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted" onClick={() => setSort("asc")} type="button"><ArrowUp className="mx-auto mb-1 h-3.5 w-3.5" />ASC</button><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted" onClick={() => setSort("desc")} type="button"><ArrowDown className="mx-auto mb-1 h-3.5 w-3.5" />DESC</button><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted" onClick={() => setSort()} type="button"><X className="mx-auto mb-1 h-3.5 w-3.5" />ล้าง sort</button></section>
        <section className="grid grid-cols-2 gap-2"><label className="space-y-1"><span className="font-bold">Group By</span><ConsoleSelect ariaLabel="Group By" className="w-full" onChange={(value) => apply((next) => { next.columns[state.index].group = value === "true"; if (value === "true") next.columns[state.index].aggregate = undefined; }, false)} value={String(Boolean(column.group))}><option value="false">ไม่ Group</option><option value="true">Group field นี้</option></ConsoleSelect></label><label className="space-y-1"><span className="font-bold">Aggregate</span><ConsoleSelect ariaLabel="Aggregate" className="w-full" onChange={(value) => apply((next) => { next.columns[state.index].aggregate = value || undefined; if (value) next.columns[state.index].group = false; }, false)} value={column.aggregate || ""}><option value="">ไม่คำนวณ</option>{field.aggregates.map((aggregate) => <option key={aggregate} value={aggregate}>{aggregate.toUpperCase()}</option>)}</ConsoleSelect></label></section>
        <section className="space-y-2"><label className="block space-y-1"><span className="font-bold">ชื่อหัวคอลัมน์</span><div className="flex gap-1"><Input className="h-9" onChange={(event) => setLabel(event.target.value)} value={label} /><button className="rounded-lg border px-3 font-semibold hover:bg-muted" onClick={() => apply((next) => { next.columns[state.index].label = label.trim() || undefined; })} type="button">ใช้</button></div></label><label className="block space-y-1"><span className="font-bold">รูปแบบ</span><ConsoleSelect ariaLabel="รูปแบบคอลัมน์" className="w-full" onChange={(format) => apply((next) => { next.columns[state.index].format = format; }, false)} value={column.format || field.formats[0]}>{field.formats.map((format) => <option key={format} value={format}>{format}</option>)}</ConsoleSelect></label></section>
        <section className="grid grid-cols-2 gap-1"><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted disabled:opacity-40" disabled={state.index === 0} onClick={() => move(-1)} type="button"><ArrowLeft className="mr-1 inline h-3.5 w-3.5" />ย้ายซ้าย</button><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted disabled:opacity-40" disabled={state.index === definition.columns.length - 1} onClick={() => move(1)} type="button">ย้ายขวา<ArrowRight className="ml-1 inline h-3.5 w-3.5" /></button><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-muted" onClick={() => { onAutoFit(state.index); onClose(); }} type="button">ปรับขนาดพอดี</button><button className="rounded-lg border px-2 py-2 font-semibold hover:bg-amber-50" onClick={() => apply((next) => { const exists = next.filters.rules.some((rule) => rule.field === column.field); if (!exists) next.filters.rules.push({ id: uid(), field: column.field, operator: defaultOperator(field), value: "" }); })} type="button"><Filter className="mr-1 inline h-3.5 w-3.5" />Filter field นี้</button></section>
            <button className="flex w-full items-center justify-center gap-2 rounded-lg border border-red-200 px-3 py-2 font-bold text-primary hover:bg-red-50 disabled:opacity-40" disabled={definition.columns.length <= 1} onClick={() => apply((next) => { next.columns.splice(state.index, 1); next.sorts = next.sorts.filter((sort) => sort.field !== column.field); })} type="button"><Trash2 className="h-3.5 w-3.5" />ลบคอลัมน์นี้</button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

