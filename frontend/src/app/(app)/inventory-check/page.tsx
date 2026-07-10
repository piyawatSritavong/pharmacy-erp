import { PageIntro } from "@/components/sections/common";
import { InventoryConsole } from "@/components/sections/inventory-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getInventory, getProducts, requireSession } from "@/services/erp";

export default async function InventoryCheckPage() {
  const session = requireRole(await requireSession(), ["branch_pos"]);
  const [branches, inventory, products] = await Promise.all([
    getBranches(),
    getInventory(session.user.branch_id),
    getProducts(session.user.branch_id)
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(session.user.branch_id || ""));

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Inventory Check"
        title="Inventory Check"
        description="Read-only inventory lookup for POS users. No stock movement actions are available on this screen."
      />
      <InventoryConsole
        branches={branchOptions}
        defaultBranchId={session.user.branch_id}
        inventory={inventory.items}
        mode="check"
        products={products.items}
      />
    </div>
  );
}
