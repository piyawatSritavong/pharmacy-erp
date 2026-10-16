import type { Metadata } from "next";
import { PageIntro } from "@/components/sections/common";
import { SalesHistoryConsole } from "@/components/sections/sales-history-console";
import { requirePermission } from "@/lib/rbac";
import { getInvoices, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ประวัติการขาย" };

const FILTER_KEYS = ["search", "branch", "payment_status", "payment_method", "sale_type", "close_status", "removed_lines", "tax_invoice_type", "date_from", "date_to"];

export default async function SalesHistoryPage({ searchParams }: { searchParams?: Promise<Record<string, string | string[] | undefined>> }) {
  const session = requirePermission(await requireSession(), ["invoice.view"]);
  const resolved = (await searchParams) || {};
  const params: Record<string, string> = { page: "1", page_size: "20" };
  for (const key of FILTER_KEYS) {
    const value = resolved[key];
    if (typeof value === "string" && value) params[key] = value;
  }
  const invoices = await getInvoices(params);

  return (
    <div className="space-y-6">
      <PageIntro
        title="ประวัติการขาย"
        description={
          session.user.role_key === "super_admin"
            ? "รายการขายย้อนหลังทุกสาขา พร้อมเลขบิลก่อน/หลังปิดรอบและสถานะของบิล"
            : "รายการขายย้อนหลังของสาขาปัจจุบัน เปิด/พิมพ์ใบเสร็จซ้ำ หรือคืน/เปลี่ยนสินค้าให้ลูกค้าได้"
        }
      />
      <SalesHistoryConsole
        initial={invoices}
        isSuperAdmin={session.user.role_key === "super_admin"}
        showFullTimestamp={session.user.role_key === "super_admin"}
      />
    </div>
  );
}
