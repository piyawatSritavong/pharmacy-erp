import { proxyClient } from "@/services/api";
import type { ReportCatalog, ReportDefinition, ReportResult, SavedReport } from "@/types/report-builder";

export function getReportCatalog() {
  return proxyClient<ReportCatalog>("/report-builder/catalog");
}

export function listSavedReports(pinTarget?: string) {
  const suffix = pinTarget ? `?pin_target=${encodeURIComponent(pinTarget)}` : "";
  return proxyClient<{ items: SavedReport[] }>(`/report-builder/reports${suffix}`);
}

/** Distinct values a filterable field actually holds, for the filter builder's
 *  value picker. Returns [] for types where a picker makes no sense. */
export function getReportFieldValues(dataset: string, field: string) {
  const query = new URLSearchParams({ dataset, field });
  return proxyClient<{ items: string[] }>(`/report-builder/field-values?${query.toString()}`);
}

export function executeReport(input: { definition?: ReportDefinition; report_id?: string; page?: number; preview?: boolean }, options?: { signal?: AbortSignal }) {
  return proxyClient<ReportResult>("/report-builder/execute", {
    method: "POST",
    body: JSON.stringify(input),
    signal: options?.signal
  });
}

export function createSavedReport(input: { name: string; description: string; definition: ReportDefinition }) {
  return proxyClient<SavedReport>("/report-builder/reports", {
    method: "POST",
    body: JSON.stringify(input)
  });
}

export function updateSavedReport(id: string, input: { name: string; description: string; definition: ReportDefinition }) {
  return proxyClient<SavedReport>(`/report-builder/reports/${id}`, {
    method: "PUT",
    body: JSON.stringify(input)
  });
}

export function deleteSavedReport(id: string) {
  return proxyClient<{ message: string }>(`/report-builder/reports/${id}`, { method: "DELETE" });
}

export function setSavedReportPinned(id: string, pinned: boolean, pinTargetKey?: string) {
  return proxyClient<SavedReport>(`/report-builder/reports/${id}/pin`, {
    method: "PATCH",
    body: JSON.stringify({ pinned, pin_target_key: pinTargetKey })
  });
}

export function reorderPinnedReports(reportIds: string[], pinTargetKey?: string) {
  return proxyClient<{ message: string }>("/report-builder/pins/order", {
    method: "PUT",
    body: JSON.stringify({ report_ids: reportIds, pin_target_key: pinTargetKey })
  });
}
