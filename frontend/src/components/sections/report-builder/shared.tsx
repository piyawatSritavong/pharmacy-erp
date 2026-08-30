"use client";

/**
 * Shared vocabulary for the report builder: formatters, definition helpers,
 * and the two primitives every panel reuses. Nothing here holds state or
 * talks to the API — it is the layer all feature files sit on.
 */

import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";

import { formatReportCell } from "@/lib/report-format";
import { cn } from "@/lib/utils";
import type {
  ReportCatalog,
  ReportDataset,
  ReportDefinition,
  ReportField,
  ReportFilterGroup,
  ReportFilterRule
} from "@/types/report-builder";

export const DATE_TIME_FORMAT = new Intl.DateTimeFormat("th-TH", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Asia/Bangkok"
});
export const DATE_FORMAT = new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeZone: "Asia/Bangkok" });
export const NUMBER_FORMAT = new Intl.NumberFormat("th-TH", { maximumFractionDigits: 2 });
export const CURRENCY_FORMAT = new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" });
export const DEFAULT_FIELD_SIDEBAR_WIDTH = 288;
export const MIN_FIELD_SIDEBAR_WIDTH = 220;
export const MAX_FIELD_SIDEBAR_WIDTH = 560;
export const COLLAPSED_FIELD_SIDEBAR_WIDTH = 52;
export const MIN_REPORT_WORKSPACE_WIDTH = 520;

export type ColumnDragItem =
  | { kind: "field"; field: string }
  | { kind: "column"; index: number }
  | null;

export type ColumnDropSide = "before" | "after";

export function dragItemFromTransfer(dataTransfer: DataTransfer): ColumnDragItem {
  const payload = dataTransfer.getData("text/plain");
  if (payload.startsWith("report-field:")) {
    return { kind: "field", field: payload.slice("report-field:".length) };
  }
  if (payload.startsWith("report-column:")) {
    const index = Number(payload.slice("report-column:".length));
    return Number.isInteger(index) && index >= 0 ? { kind: "column", index } : null;
  }
  return null;
}

export function uid() {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export function emptyFilterGroup(): ReportFilterGroup {
  return { id: uid(), logic: "and", rules: [], groups: [] };
}

export function defaultDefinition(catalog: ReportCatalog, dataset: ReportDataset): ReportDefinition {
  return {
    version: catalog.definition_version,
    dataset_key: dataset.key,
    columns: dataset.default_columns.map((field) => ({ field })),
    filters: emptyFilterGroup(),
    sorts: [],
    time_config: dataset.default_time_field
      ? { field: dataset.default_time_field, range_type: "relative", relative: "last_30_days", bucket: "auto" }
      : undefined,
    layout: { field_sidebar_width: DEFAULT_FIELD_SIDEBAR_WIDTH },
    page_size: 100
  };
}

export function cloneDefinition(definition: ReportDefinition): ReportDefinition {
  return structuredClone(definition);
}

export function ensureEditorIDs(group: ReportFilterGroup): ReportFilterGroup {
  return {
    ...group,
    id: group.id || uid(),
    rules: (group.rules || []).map((rule) => ({ ...rule, id: rule.id || uid() })),
    groups: (group.groups || []).map(ensureEditorIDs)
  };
}

export function fieldMap(dataset?: ReportDataset) {
  return new Map((dataset?.fields || []).map((field) => [field.key, field]));
}

export function operatorValueCount(operator: string, catalog: ReportCatalog) {
  return catalog.operators.find((item) => item.key === operator)?.value_count ?? 1;
}

export function operatorsFor(field: ReportField | undefined, catalog: ReportCatalog) {
  if (!field) return [];
  return catalog.operators.filter((operator) => operator.types.includes(field.type));
}

export function defaultOperator(field?: ReportField) {
  return field?.type === "text" ? "contains" : "eq";
}

export function fieldLabel(dataset: ReportDataset | undefined, key: string) {
  return dataset?.fields.find((field) => field.key === key)?.label || key;
}

export function relationBadge(dataset: ReportDataset, field: ReportField) {
  if (!field.relation_key) return null;
  const relation = dataset.relations.find((item) => item.key === field.relation_key);
  if (!relation) return null;
  if (relation.cardinality.includes("aggregated")) return "สรุปก่อน Join";
  if (relation.cardinality === "one-to-many") return "ขยายหลายแถว";
  return null;
}

export const formatCell = formatReportCell;

export function formatRuleValue(rule: ReportFilterRule) {
  if (rule.operator === "in") return (rule.values || []).join(", ");
  if (rule.operator === "between") return (rule.values || []).join(" | ");
  return rule.value === undefined ? "" : String(rule.value);
}

export function parseRuleValue(operator: string, value: string): Pick<ReportFilterRule, "value" | "values"> {
  if (operator === "in") return { values: value.split(",").map((item) => item.trim()).filter(Boolean), value: undefined };
  if (operator === "between") return { values: value.split("|").map((item) => item.trim()).slice(0, 2), value: undefined };
  return { value, values: undefined };
}

export function filterCount(group: ReportFilterGroup): number {
  return (group.rules || []).length + (group.groups || []).reduce((total, nested) => total + filterCount(nested), 0);
}

export type ConsoleSelectProps = {
  value: string;
  onChange: (value: string) => void;
  children: ReactNode;
  className?: string;
  ariaLabel: string;
  disabled?: boolean;
};

export function ConsoleSelect({ value, onChange, children, className, ariaLabel, disabled }: ConsoleSelectProps) {
  return (
    <select
      aria-label={ariaLabel}
      // text-foreground explicit, not inherited — this now also renders on
      // the report-builder toolbar's dark card (D9), same reasoning as the
      // shared Input/Select/Textarea fix (see their comments).
      className={cn("h-9 rounded-lg border bg-white px-3 text-sm font-medium text-foreground outline-none focus:ring-2 focus:ring-primary/20 disabled:opacity-50", className)}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      value={value}
    >
      {children}
    </select>
  );
}

export function FieldOptions({ dataset, exclude }: { dataset: ReportDataset; exclude?: string[] }) {
  const excluded = new Set(exclude || []);
  const groups = Array.from(new Set(dataset.fields.map((field) => field.group)));
  return (
    <>
      {groups.map((group) => (
        <optgroup key={group} label={group}>
          {dataset.fields.filter((field) => field.group === group && !excluded.has(field.key)).map((field) => (
            <option key={field.key} value={field.key}>{field.label}</option>
          ))}
        </optgroup>
      ))}
    </>
  );
}

export function timelineTickLabel(value: string, bucket?: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  if (bucket === "hour") {
    return new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", timeZone: "Asia/Bangkok" }).format(date);
  }
  if (bucket === "month") {
    return new Intl.DateTimeFormat("th-TH", { month: "short", year: "2-digit", timeZone: "Asia/Bangkok" }).format(date);
  }
  return new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short", year: "2-digit", timeZone: "Asia/Bangkok" }).format(date);
}

