import { PageIntro } from "@/components/sections/common";
import { ClaimsConsole } from "@/components/sections/claims-console";
import { PosClaimsConsole } from "@/components/sections/pos-claims-console";
import { requirePermission } from "@/lib/rbac";
import { getInvoices, getProductReturns, getSuppliers, requireSession } from "@/services/erp";

export default async function ClaimsPage() {
  // Admin manages the full claim (send to supplier, resolve); a POS cashier
  // only raises returns for their own branch.
  const session = requirePermission(await requireSession(), ["returns.manage", "invoice.view"]);
  const isManager = (session.user.permissions || []).includes("returns.manage");

  if (!isManager) {
    const [returns, invoices] = await Promise.all([getProductReturns(), getInvoices()]);
    return (
      <div className="space-y-6">
        <PageIntro title="เคลม/คืนสินค้า" description="แจ้งคืนหรือเคลมสินค้าที่ขายไปแล้ว และติดตามสถานะ" />
        <PosClaimsConsole initialItems={returns.items} invoices={invoices.items} />
      </div>
    );
  }

  const [returns, suppliers] = await Promise.all([getProductReturns(), getSuppliers({ limit: 200 })]);
  return (
    <div className="space-y-6">
      <PageIntro
        title="เคลม/คืนสินค้า"
        description="คำขอคืนสินค้าจากหน้าร้าน — ส่งเคลมให้คู่ค้า แล้วปิดเคลมเป็นรับรุ่นเดิมหรือรุ่นทดแทน"
      />
      <ClaimsConsole initialItems={returns.items} suppliers={suppliers.items} />
    </div>
  );
}
