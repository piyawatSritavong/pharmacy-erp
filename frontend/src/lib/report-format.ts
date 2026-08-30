import type { ReportResultColumn } from "@/types/report-builder";

const DATE_TIME_FORMAT = new Intl.DateTimeFormat("th-TH", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Asia/Bangkok"
});
const DATE_FORMAT = new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeZone: "Asia/Bangkok" });
const NUMBER_FORMAT = new Intl.NumberFormat("th-TH", { maximumFractionDigits: 2 });
const CURRENCY_FORMAT = new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" });

/**
 * Renders one report cell according to its column's format/type. Shared so a
 * report reads identically wherever it appears — inside Generate Report and
 * pinned onto the dashboard or any other page (they used to disagree: the
 * builder formatted currency, the pinned copy printed the raw number).
 */
export function formatReportCell(value: unknown, column: ReportResultColumn) {
  if (value === null || value === undefined || value === "") return "—";
  if (column.format === "currency") return CURRENCY_FORMAT.format(Number(value));
  if (column.format === "number" || column.type === "number" || column.type === "integer") {
    return NUMBER_FORMAT.format(Number(value));
  }
  if (column.format === "datetime" || column.type === "datetime") {
    const date = new Date(String(value));
    return Number.isNaN(date.getTime()) ? String(value) : DATE_TIME_FORMAT.format(date);
  }
  if (column.format === "date" || column.type === "date") {
    const date = new Date(String(value));
    return Number.isNaN(date.getTime()) ? String(value) : DATE_FORMAT.format(date);
  }
  if (column.type === "boolean") return value ? "ใช่" : "ไม่ใช่";
  return String(value);
}
