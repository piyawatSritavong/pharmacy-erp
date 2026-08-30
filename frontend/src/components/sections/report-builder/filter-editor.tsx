"use client";

/**
 * FEATURE: advanced filters (AND/OR).
 * Recursive: a group holds rules plus nested groups, and each level chooses its
 * own AND/OR. Pure — it hands a new group upward through onChange and never
 * mutates the definition itself.
 */

import { CopyPlus, Plus, Trash2, X } from "lucide-react";
import { useEffect, useState } from "react";

import { getReportFieldValues } from "@/services/report-builder";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import type { ReportCatalog, ReportDataset, ReportField, ReportFilterGroup, ReportFilterRule } from "@/types/report-builder";
import {
  ConsoleSelect,
  FieldOptions,
  defaultOperator,
  emptyFilterGroup,
  fieldMap,
  formatRuleValue,
  operatorValueCount,
  operatorsFor,
  parseRuleValue,
  uid
} from "./shared";


/** Sentinel for the "อื่นๆ" row. Real data can't collide with it — the endpoint
 *  filters out empty strings, and this is not a value any column holds. */
const CUSTOM_VALUE = "\u0000custom";

/**
 * Value control for one filter rule. Offers the field's real values as a
 * dropdown so you don't have to remember exact spelling, with "อื่นๆ" at the
 * bottom to fall back to free text (needed for partial matches, and for values
 * outside the fetched sample).
 *
 * Falls back to a plain input whenever a picker would not help: numeric and
 * date fields, multi-value operators (in / between), or a field whose values
 * could not be loaded.
 */
function RuleValueControl({
  datasetKey,
  field,
  rule,
  onChange
}: {
  datasetKey: string;
  field: ReportField;
  rule: ReportFilterRule;
  onChange: (patch: Partial<ReportFilterRule>) => void;
}) {
  const [values, setValues] = useState<string[] | null>(null);
  // "custom" is sticky per rule: once you choose อื่นๆ the input stays until
  // you pick a listed value again.
  const [custom, setCustom] = useState(false);
  const multiValue = rule.operator === "in" || rule.operator === "between";
  const pickable = field.type === "text" && !multiValue;

  useEffect(() => {
    if (!pickable) {
      setValues(null);
      return;
    }
    let cancelled = false;
    getReportFieldValues(datasetKey, field.key)
      .then((response) => { if (!cancelled) setValues(response.items); })
      .catch(() => { if (!cancelled) setValues([]); });
    return () => { cancelled = true; };
  }, [datasetKey, field.key, pickable]);

  const current = formatRuleValue(rule);
  const listed = values || [];
  // An existing value that isn't in the sample means the rule was typed by hand
  // (or the sample was capped) — show the input rather than silently losing it.
  const outsideList = Boolean(current) && listed.length > 0 && !listed.includes(current);

  if (!pickable || !values || values.length === 0 || custom || outsideList) {
    return (
      <div className="flex gap-1.5">
        <Input
          aria-label="ค่าตัวกรอง"
          className="h-9"
          onChange={(event) => onChange(parseRuleValue(rule.operator, event.target.value))}
          placeholder={rule.operator === "between" ? "ค่าเริ่ม | ค่าสิ้นสุด" : rule.operator === "in" ? "ค่า 1, ค่า 2, ..." : "ค่าที่ต้องการ"}
          type={field.type === "number" || field.type === "integer" ? "number" : "text"}
          value={current}
        />
        {pickable && listed.length > 0 ? (
          <button
            aria-label="กลับไปเลือกจากรายการ"
            className="shrink-0 rounded-lg border px-2 text-[11px] text-muted-foreground hover:bg-muted"
            onClick={() => { setCustom(false); onChange(parseRuleValue(rule.operator, "")); }}
            type="button"
          >
            เลือกจากรายการ
          </button>
        ) : null}
      </div>
    );
  }

  return (
    <ConsoleSelect
      ariaLabel="ค่าตัวกรอง"
      className="w-full"
      onChange={(value) => {
        if (value === CUSTOM_VALUE) {
          setCustom(true);
          onChange(parseRuleValue(rule.operator, ""));
          return;
        }
        onChange(parseRuleValue(rule.operator, value));
      }}
      value={current}
    >
      <option value="">เลือกค่า...</option>
      {listed.map((value) => <option key={value} value={value}>{value}</option>)}
      {/* Always last, as requested. */}
      <option value={CUSTOM_VALUE}>อื่นๆ (กรอกเอง)</option>
    </ConsoleSelect>
  );
}

export function FilterGroupEditor({ catalog, dataset, group, depth, onChange, onRemove }: {
  catalog: ReportCatalog;
  dataset: ReportDataset;
  group: ReportFilterGroup;
  depth: number;
  onChange: (next: ReportFilterGroup) => void;
  onRemove?: () => void;
}) {
  const fields = fieldMap(dataset);
  function addRule() {
    const first = dataset.fields[0];
    onChange({ ...group, rules: [...group.rules, { id: uid(), field: first.key, operator: defaultOperator(first), value: "" }] });
  }
  function updateRule(index: number, patch: Partial<ReportFilterRule>) {
    onChange({ ...group, rules: group.rules.map((rule, ruleIndex) => ruleIndex === index ? { ...rule, ...patch } : rule) });
  }
  return (
    <div className={cn("rounded-xl border p-3", depth ? "bg-white" : "bg-[#fffaf3]")}> 
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <span className="text-xs font-bold">{depth ? `กลุ่มย่อยระดับ ${depth}` : "ตัวกรอง"}</span>
        <ConsoleSelect ariaLabel="ตรรกะตัวกรอง" className="h-8 w-auto text-xs" onChange={(logic) => onChange({ ...group, logic: logic as "and" | "or" })} value={group.logic}>
          <option value="and">ตรงทุกเงื่อนไข (AND)</option>
          <option value="or">ตรงอย่างน้อยหนึ่งเงื่อนไข (OR)</option>
        </ConsoleSelect>
        <button className="ml-auto inline-flex items-center gap-1 rounded-lg border bg-white px-2 py-1.5 text-xs font-semibold hover:bg-muted" onClick={addRule} type="button"><Plus className="h-3 w-3" />เงื่อนไข</button>
        {depth < 4 ? <button className="inline-flex items-center gap-1 rounded-lg border bg-white px-2 py-1.5 text-xs font-semibold hover:bg-muted" onClick={() => onChange({ ...group, groups: [...group.groups, emptyFilterGroup()] })} type="button"><CopyPlus className="h-3 w-3" />กลุ่ม AND/OR</button> : null}
        {onRemove ? <button aria-label="ลบกลุ่มตัวกรอง" className="rounded-lg p-1.5 text-muted-foreground hover:bg-red-50 hover:text-primary" onClick={onRemove} type="button"><Trash2 className="h-3.5 w-3.5" /></button> : null}
      </div>
      <div className="space-y-2">
        {group.rules.map((rule, index) => {
          const selectedField = fields.get(rule.field) || dataset.fields[0];
          const operators = operatorsFor(selectedField, catalog);
          const count = operatorValueCount(rule.operator, catalog);
          return (
            <div className="grid gap-2 rounded-lg border bg-white p-2 sm:grid-cols-[minmax(150px,1.2fr)_minmax(130px,.8fr)_minmax(180px,1.3fr)_32px]" key={rule.id || index}>
              <ConsoleSelect ariaLabel="ฟิลด์ตัวกรอง" className="w-full" onChange={(fieldKey) => { const nextField = fields.get(fieldKey); updateRule(index, { field: fieldKey, operator: defaultOperator(nextField), value: "", values: undefined }); }} value={rule.field}><FieldOptions dataset={dataset} /></ConsoleSelect>
              <ConsoleSelect ariaLabel="ตัวดำเนินการ" className="w-full" onChange={(operator) => updateRule(index, { operator, value: "", values: undefined })} value={rule.operator}>
                {operators.map((operator) => <option key={operator.key} value={operator.key}>{operator.label}</option>)}
              </ConsoleSelect>
              {count === 0 ? <div className="flex h-9 items-center rounded-lg bg-muted px-3 text-xs text-muted-foreground">ไม่ต้องระบุค่า</div> : selectedField.type === "boolean" && count === 1 ? (
                <ConsoleSelect ariaLabel="ค่าบูลีน" className="w-full" onChange={(value) => updateRule(index, { value: value === "true" })} value={String(rule.value ?? true)}><option value="true">ใช่</option><option value="false">ไม่ใช่</option></ConsoleSelect>
              ) : (
                <RuleValueControl
                  datasetKey={dataset.key}
                  field={selectedField}
                  key={`${rule.id}-${rule.field}-${rule.operator}`}
                  onChange={(patch) => updateRule(index, patch)}
                  rule={rule}
                />
              )}
              <button aria-label="ลบเงื่อนไข" className="grid h-9 w-8 place-items-center rounded-lg text-muted-foreground hover:bg-red-50 hover:text-primary" onClick={() => onChange({ ...group, rules: group.rules.filter((_, ruleIndex) => ruleIndex !== index) })} type="button"><X className="h-4 w-4" /></button>
            </div>
          );
        })}
        {group.groups.map((nested, index) => (
          <FilterGroupEditor
            catalog={catalog}
            dataset={dataset}
            depth={depth + 1}
            group={nested}
            key={nested.id || index}
            onChange={(next) => onChange({ ...group, groups: group.groups.map((item, groupIndex) => groupIndex === index ? next : item) })}
            onRemove={() => onChange({ ...group, groups: group.groups.filter((_, groupIndex) => groupIndex !== index) })}
          />
        ))}
        {!group.rules.length && !group.groups.length ? <p className="py-2 text-center text-xs text-muted-foreground">ยังไม่มีตัวกรอง รายงานจะแสดงข้อมูลตามช่วงเวลาที่เลือก</p> : null}
      </div>
    </div>
  );
}

