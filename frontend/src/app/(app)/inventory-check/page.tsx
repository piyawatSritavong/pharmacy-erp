import { PageIntro } from "@/components/sections/common";
import { InventoryConsole } from "@/components/sections/inventory-console";
import { StockRequestConsole } from "@/components/sections/stock-request-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getStockTransferRequests, requireSession } from "@/services/erp";

export default async function InventoryCheckPage() {
  const session = requirePermission(await requireSession(), ["inventory.view.branch"]);
  // Note: inventory itself is not fetched here — InventoryConsole's
  // mode="check" view loads it client-side and keeps only the rows that are at
  // or below their reorder point. The full stock list with on-hand quantities
  // is deliberately not shown on the POS: a cashier who can read the system's
  // count stops counting the shelf during a physical audit.
  const [branches, products, requests] = await Promise.all([
    getBranches(),
    getProducts(session.user.branch_id),
    getStockTransferRequests()
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(session.user.branch_id || ""));

  return (
    <div className="space-y-6">
      <PageIntro
        title="เช็กสต๊อก"
        description="ดูสินค้าที่ถึงจุดแจ้งเตือนสต๊อก และส่งคำขอเบิกสินค้าให้ผู้ดูแลเลือกสาขาต้นทาง"
      />
      <StockRequestConsole mode="pos" products={products.items} requests={requests.items} />
      <InventoryConsole
        branches={branchOptions}
        defaultBranchId={session.user.branch_id}
        mode="check"
        products={products.items}
      />
    </div>
  );
}
