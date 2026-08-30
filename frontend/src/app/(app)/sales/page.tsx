import { PosWorkspace } from "@/components/sections/pos-workspace";
import { requirePermission } from "@/lib/rbac";
import {
  getInventory,
  getProducts,
  requireSession
} from "@/services/erp";

export default async function SalesPage() {
  const session = requirePermission(await requireSession(), ["invoice.create.pos"]);
  const branchId = String(session.user.branch_id || "");
  const [products, inventory] = await Promise.all([
    getProducts(branchId),
    getInventory(branchId)
  ]);

  return (
    <PosWorkspace
      branchId={branchId}
      branchName={String(session.user.branch_name || "สาขาปัจจุบัน")}
      inventory={inventory.items}
      products={products.items}
    />
  );
}
