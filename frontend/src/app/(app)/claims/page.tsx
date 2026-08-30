import { PageIntro } from "@/components/sections/common";
import { ClaimsConsole } from "@/components/sections/claims-console";
import { requirePermission } from "@/lib/rbac";
import { getProductReturns, getSuppliers, requireSession } from "@/services/erp";

export default async function ClaimsPage() {
  requirePermission(await requireSession(), ["returns.manage"]);
  const [returns, suppliers] = await Promise.all([
    getProductReturns(),
    getSuppliers({ limit: 200 })
  ]);

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
