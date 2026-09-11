import { cache } from "react";
import { redirect } from "next/navigation";

import { apiServer } from "@/services/api-server";
import type { Session } from "@/types";

/**
 * The session for the current request, fetched once.
 *
 * (app)/layout.tsx asks for it to draw the shell, and every page under it asks
 * again to gate its own data — a layout and a page cannot pass props to each
 * other, so each had to fetch. That was two /me round-trips per authenticated
 * page load, thirty-two pages over. React's cache() is request-scoped in
 * server components: every call in one render shares one promise, so the
 * second caller gets the first caller's result (or its rejection) without
 * touching the network. Next's own fetch dedup does not apply here because
 * apiServer sends cache: "no-store".
 */
export const getSession = cache(async () => apiServer<Session>("/me"));

export async function requireSession(): Promise<Session> {
  try {
    return await getSession();
  } catch {
    redirect("/login");
  }
}

export async function getDashboard() {
  return apiServer<Record<string, unknown>>("/dashboard");
}

export type SalesScope = { dateFrom?: string; dateTo?: string; paymentStatus?: string };

function salesScopeQuery(scope?: SalesScope) {
  const query = new URLSearchParams();
  if (scope?.dateFrom) query.set("date_from", scope.dateFrom);
  if (scope?.dateTo) query.set("date_to", scope.dateTo);
  if (scope?.paymentStatus) query.set("payment_status", scope.paymentStatus);
  return query.size ? `?${query.toString()}` : "";
}

export async function getBranchSales(scope?: SalesScope) {
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/dashboard/branch-sales${salesScopeQuery(scope)}`);
}

export type BreakdownGroup = {
  amount: number;
  invoice_count: number;
  after_amount: number;
  after_count: number;
  difference: number;
  difference_count: number;
  adjusted_amount?: number;
  reduction?: number;
};

export type TenderSplit = {
  cash_amount: number;
  transfer_amount: number;
  total_amount: number;
  invoice_count: number;
};

export type DailyBreakdown = {
  date_from: string;
  date_to: string;
  markup_percent: number;
  shows_close: boolean;
  /** True once a month-end round covers the whole window being shown. */
  closed: boolean;
  reconciliation_number?: string;
  period_start?: string;
  period_end?: string;
  generated_at: string;
  overall: Record<string, BreakdownGroup | TenderSplit>;
  branches: Array<Record<string, BreakdownGroup | TenderSplit | string>>;
};

/**
 * The day as it stands right now, sorted into what the month-end close will
 * leave alone and what it will move. Computed live from the bills — no daily
 * snapshot is kept, because a stored total is just a second number that can
 * disagree with the bills it came from.
 */
export async function getDailyBreakdown(scope?: { dateFrom?: string; dateTo?: string; markupPercent?: number }) {
  const query = new URLSearchParams();
  if (scope?.dateFrom) query.set("date_from", scope.dateFrom);
  if (scope?.dateTo) query.set("date_to", scope.dateTo);
  if (scope?.markupPercent) query.set("markup_percent", String(scope.markupPercent));
  return apiServer<DailyBreakdown>(`/dashboard/daily-breakdown${query.size ? `?${query.toString()}` : ""}`);
}

/** Superadmin only: closed rounds, newest first, twenty at a time. */
export async function getReconciliationRounds(options?: { search?: string; limit?: number; offset?: number }) {
  const query = new URLSearchParams();
  if (options?.search) query.set("search", options.search);
  query.set("limit", String(options?.limit ?? 20));
  query.set("offset", String(options?.offset ?? 0));
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/accounting/month-end/reconciliations?${query.toString()}`);
}

/** Superadmin only: what the period looked like before the month-end close, and now. */
export async function getRevenueComparison(scope?: SalesScope) {
  return apiServer<{
    before_invoice_count: number;
    before_amount: number;
    after_invoice_count: number;
    after_amount: number;
    hidden_invoice_count: number;
    hidden_amount: number;
    repriced_reduction: number;
    branches: Array<Record<string, unknown>>;
  }>(`/dashboard/revenue-comparison${salesScopeQuery(scope)}`);
}

export async function getTodayBranchSales() {
  return apiServer<{
    date: string;
    total_amount: number;
    invoice_count: number;
    branches: Array<{ branch_code: string; branch_name: string; invoice_count: number; total_amount: number }>;
  }>("/dashboard/today-branch-sales");
}

export async function getLowStock() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/dashboard/low-stock");
}

export async function getNotifications() {
  return apiServer<{ items: Array<{ type: string; label: string; detail: string; count: number; href: string }> }>("/notifications");
}

export async function getDailySales(startDate?: string, endDate?: string) {
  const query = new URLSearchParams();
  if (startDate) query.set("start_date", startDate);
  if (endDate) query.set("end_date", endDate);
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<Record<string, unknown>>(`/dashboard/daily-sales${suffix}`);
}

export async function getBranches() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/branches");
}

export async function getMonthEndReconciliations() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/accounting/month-end/reconciliations");
}

export type Pagination = { page: number; page_size: number; total: number; total_pages: number };

export async function getProducts(branchId?: string, filters?: {
  search?: string;
  categoryId?: string;
  active?: string;
  page?: number;
  pageSize?: number;
  salesChannel?: string;
  requiresFdaReport?: string;
}) {
  const query = new URLSearchParams();
  if (branchId) query.set("branch_id", branchId);
  if (filters?.search) query.set("search", filters.search);
  if (filters?.categoryId) query.set("category_id", filters.categoryId);
  if (filters?.active) query.set("active", filters.active);
  if (filters?.page) query.set("page", String(filters.page));
  if (filters?.pageSize) query.set("page_size", String(filters.pageSize));
  if (filters?.salesChannel) query.set("sales_channel", filters.salesChannel);
  if (filters?.requiresFdaReport) query.set("requires_fda_report", filters.requiresFdaReport);
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<{ items: Array<Record<string, unknown>>; pagination: Pagination }>(`/products${suffix}`);
}

export async function getProductReturns(status?: string) {
  const suffix = status ? `?status=${encodeURIComponent(status)}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/product-returns${suffix}`);
}

export async function getFdaReportSummary(filters?: { dateFrom?: string; dateTo?: string; branchId?: string; categoryId?: string; productIds?: string[] }) {
  const query = new URLSearchParams();
  if (filters?.dateFrom) query.set("date_from", filters.dateFrom);
  if (filters?.dateTo) query.set("date_to", filters.dateTo);
  if (filters?.branchId) query.set("branch_id", filters.branchId);
  if (filters?.categoryId) query.set("category_id", filters.categoryId);
  for (const id of filters?.productIds || []) query.append("product_id", id);
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<Record<string, unknown>>(`/fda-reports/summary${suffix}`);
}

export async function getProductCategories() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/product-categories");
}

export async function getPromotions(activeOnly = false) {
  const query = activeOnly ? "?active_only=true" : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/promotions${query}`);
}

export async function getAliases(filters?: { branchId?: string; productId?: string }) {
  const query = new URLSearchParams();
  if (filters?.branchId) {
    query.set("branch_id", filters.branchId);
  }
  if (filters?.productId) {
    query.set("product_id", filters.productId);
  }
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/aliases${suffix}`);
}

export async function getInventory(branchId?: string, filters?: { search?: string; page?: number; pageSize?: number }) {
  const query = new URLSearchParams();
  if (branchId) query.set("branch_id", branchId);
  if (filters?.search) query.set("search", filters.search);
  if (filters?.page) query.set("page", String(filters.page));
  if (filters?.pageSize) query.set("page_size", String(filters.pageSize));
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<{ items: Array<Record<string, unknown>>; pagination: Pagination }>(`/inventory${suffix}`);
}

export async function getSuppliers(filters?: { search?: string; active?: string; cursor?: string; limit?: number }) {
  const query = new URLSearchParams();
  if (filters?.search) query.set("search", filters.search);
  if (filters?.active) query.set("active", filters.active);
  if (filters?.cursor) query.set("cursor", filters.cursor);
  query.set("limit", String(filters?.limit || 20));
  return apiServer<{ items: Array<Record<string, unknown>>; next_cursor?: string; has_more: boolean }>(`/suppliers?${query.toString()}`);
}

export async function getPurchaseOrders(filters?: { search?: string; supplierId?: string; branchId?: string; status?: string; dateFrom?: string; dateTo?: string; page?: number; pageSize?: number }) {
  const query = new URLSearchParams();
  if (filters?.search) query.set("search", filters.search);
  if (filters?.supplierId) query.set("supplier_id", filters.supplierId);
  if (filters?.branchId) query.set("branch_id", filters.branchId);
  if (filters?.status) query.set("status", filters.status);
  if (filters?.dateFrom) query.set("date_from", filters.dateFrom);
  if (filters?.dateTo) query.set("date_to", filters.dateTo);
  query.set("page", String(filters?.page || 1));
  query.set("page_size", String(filters?.pageSize || 50));
  return apiServer<{ items: Array<Record<string, unknown>>; pagination: Pagination }>(`/purchase-orders?${query.toString()}`);
}

export async function getPurchaseOrder(id: string) {
  return apiServer<Record<string, unknown>>(`/purchase-orders/${id}`);
}

export async function getInventoryLots(branchId: string, productId: string, stockBucket: string) {
  const query = new URLSearchParams({ branch_id: branchId, product_id: productId, stock_bucket: stockBucket });
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/inventory/lots?${query.toString()}`);
}

export async function getStockTransferRequests(status?: string) {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/stock-transfer-requests${query}`);
}

export async function getQuotations(governmentMode?: boolean) {
  const query = governmentMode === undefined ? "" : `?government_mode=${governmentMode}`;
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/quotations${query}`);
}

export async function getInvoices(governmentMode?: boolean) {
  const query = governmentMode === undefined ? "" : `?government_mode=${governmentMode}`;
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/invoices${query}`);
}

export async function getInvoice(id: string) {
  return apiServer<Record<string, unknown>>(`/invoices/${id}`);
}

export async function getInvoicePrint(id: string) {
  return apiServer<Record<string, unknown>>(`/invoices/${id}/print`);
}

export async function getTransfers() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/transfers");
}

export async function getMonthEndWorkpapers() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/accounting/month-end");
}

export async function getTaxReport() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/reports/tax");
}

export async function getProfitLossReport() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/reports/profit-loss");
}

export async function getUsers() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/users");
}

export async function getRoles() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/roles");
}

export async function getPermissions() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/permissions");
}

export async function getSequences() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/branches/sequences");
}

export async function getMarketplaceProviders() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/marketplace/providers");
}

export async function getMarketplaceOrders() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/marketplace/orders");
}


export async function getAuditLogs(filters?: Record<string, string | undefined>) {
  const query = new URLSearchParams();
  Object.entries(filters || {}).forEach(([key, value]) => {
    if (value) {
      query.set(key, value);
    }
  });
  const suffix = query.size ? `?${query.toString()}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/audit-logs${suffix}`);
}
