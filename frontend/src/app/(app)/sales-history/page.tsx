import { PageIntro } from "@/components/sections/common";
import { SalesHistoryConsole } from "@/components/sections/sales-history-console";
import { requirePermission } from "@/lib/rbac";
import { getInvoices, requireSession } from "@/services/erp";

export default async function SalesHistoryPage() {
  const session = requirePermission(await requireSession(), ["invoice.view"]);
  const invoices = await getInvoices();

  return (
    <div className="space-y-6">
      <PageIntro
        title="ประวัติ"
        description={
          session.user.role_key === "super_admin"
            ? "รายการขายย้อนหลังทุกสาขา พร้อมเลขบิลก่อน/หลังปิดรอบและสถานะของบิล"
            : "รายการขายย้อนหลังของสาขาปัจจุบัน เปิด/พิมพ์ใบเสร็จซ้ำ หรือคืน/เปลี่ยนสินค้าให้ลูกค้าได้"
        }
      />
      <SalesHistoryConsole
        initialItems={invoices.items.map((item) => ({ ...item, tax_invoice_label: item.tax_invoice_type === "full" ? "เต็มรูป" : "อย่างย่อ" }))}
        isSuperAdmin={session.user.role_key === "super_admin"}
        showFullTimestamp={session.user.role_key === "super_admin"}
      />
    </div>
  );
}
