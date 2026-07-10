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

export async function getDailySales(date?: string) {
  const query = date ? `?date=${encodeURIComponent(date)}` : "";
  return apiServer<Record<string, unknown>>(`/dashboard/daily-sales${query}`);
}

export async function getBranches() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/branches");
}

export async function getProducts(branchId?: string) {
  const query = branchId ? `?branch_id=${branchId}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/products${query}`);
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

export async function getInventory(branchId?: string) {
  const query = branchId ? `?branch_id=${branchId}` : "";
  return apiServer<{ items: Array<Record<string, unknown>> }>(`/inventory${query}`);
}

export async function getQuotations() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/quotations");
}

export async function getInvoices() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/invoices");
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

export async function getChecks() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/checks");
}

export async function getOutstandingInvoices() {
  return apiServer<{ items: Array<Record<string, unknown>> }>("/checks/outstanding-invoices");
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
