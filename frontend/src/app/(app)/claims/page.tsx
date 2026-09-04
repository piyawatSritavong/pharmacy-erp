import { PageIntro } from "@/components/sections/common";
import { ClaimsConsole } from "@/components/sections/claims-console";
import { PosClaimsConsole } from "@/components/sections/pos-claims-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getInvoices, getProductReturns, getSuppliers, requireSession } from "@/services/erp";

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

  const [returns, suppliers, branches] = await Promise.all([
    getProductReturns(),
    getSuppliers({ limit: 200 }),
    getBranches()
  ]);
  // Ghost Stock is a Superadmin concept end to end: central_admin holds real
  // stock and is never offered the choice, so it never learns Ghost exists.
  const canClaimGhost = session.user.role_key === "super_admin";
  return (
    <div className="space-y-6">
      <PageIntro
        title="เคลม/คืนสินค้า"
        description="คำขอคืนจากหน้าร้าน และเคลมสต๊อกกับคู่ค้าโดยตรง — ส่งเคลม แล้วปิดเป็นรับรุ่นเดิมหรือรุ่นทดแทน"
      />
      <ClaimsConsole
        branches={branches.items}
        canClaimGhost={canClaimGhost}
        initialItems={returns.items}
        suppliers={suppliers.items}
      />
    </div>
  );
}
