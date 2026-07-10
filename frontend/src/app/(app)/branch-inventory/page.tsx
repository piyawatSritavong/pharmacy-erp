import { PageIntro } from "@/components/sections/common";
import { BranchInventoryWorkspace } from "@/components/sections/branch-inventory-workspace";
import { requireRole } from "@/lib/rbac";
import { getBranches, getInventory, getProducts, getTransfers, requireSession } from "@/services/erp";

export default async function BranchInventoryPage({
  searchParams
}: {
  searchParams?: { tab?: string };
}) {
  const session = requireRole(await requireSession(), ["branch_admin"]);
  const [branches, inventory, products, transfers] = await Promise.all([
    getBranches(),
    getInventory(session.user.branch_id),
    getProducts(session.user.branch_id),
    getTransfers()
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(session.user.branch_id || ""));
  const defaultTab = searchParams?.tab === "transfers" ? "transfers" : "inventory";

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Branch Stock"
        title="Branch Inventory"
        description="Manage real/ghost inventory within the assigned branch and handle transfer request/dispatch from the same workspace."
      />
      <BranchInventoryWorkspace
        defaultBranchId={session.user.branch_id}
        defaultTab={defaultTab}
        inventory={inventory.items}
        inventoryBranches={branchOptions}
        products={products.items}
        transferBranches={branches.items}
        transfers={transfers.items}
      />
    </div>
  );
}
