import { redirect } from "next/navigation";

import { apiServer } from "@/services/api-server";
import type { Session } from "@/types";

export async function getSession() {
  return apiServer<Session>("/me");
}

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

export async function getBranchSales() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/dashboard/branch-sales");
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
