import { PageIntro } from "@/components/sections/common";
import { StockRequestConsole } from "@/components/sections/stock-request-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getStockTransferRequests, requireSession } from "@/services/erp";

export default async function RequisitionsPage() {
  const session = requirePermission(await requireSession(), ["inventory.view.branch"]);
  // Head office reviews requisitions and can raise them for any branch; a POS
  // cashier raises and tracks their own.
  const isReviewer = (session.user.permissions || []).includes("transfer.approve");
  const [branches, products, requests] = await Promise.all([
    getBranches(),
    getProducts(isReviewer ? undefined : session.user.branch_id, { pageSize: 500 }),
    getStockTransferRequests()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="เบิกสินค้า"
        description={isReviewer ? "ตรวจคำขอเบิกจากสาขา สร้างใบโอน หรือออกใบเบิกแทนสาขา" : "ขอเติมสต๊อกจากผู้ดูแล และติดตามสถานะใบโอน"}
      />
      <StockRequestConsole
        branches={branches.items}
        mode={isReviewer ? "admin" : "pos"}
        products={products.items}
        requests={requests.items}
      />
    </div>
  );
}
