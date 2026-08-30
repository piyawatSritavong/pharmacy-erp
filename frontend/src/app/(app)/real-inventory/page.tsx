import { InventoryConsole } from "@/components/sections/inventory-console";
import { PageIntro } from "@/components/sections/common";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getInventory, getProducts, requireSession } from "@/services/erp";

export default async function RealInventoryPage({
  searchParams
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  const session = requirePermission(await requireSession(), ["inventory.manage.global"]);
  const params = await searchParams;
  const value = (key: string) => typeof params?.[key] === "string" ? String(params[key]) : "";
  const requestedBranchId = value("inventory_branch");
  const branches = await getBranches();
  // A branch-scoped account (แอดมินระบบสาขา) may only read its own branch, so
  // default to that instead of whichever branch happens to sort first — asking
  // the API for someone else's branch is a 403 that takes the whole page down.
  const ownBranchId = String(session.user.branch_id || "");
  const branchOptions = ownBranchId
    ? branches.items.filter((item) => String(item.id) === ownBranchId)
    : branches.items;
  const selectedBranchId = requestedBranchId || ownBranchId || String(branches.items[0]?.id || "");
  const filters = {
    search: value("inventory_q"),
    branchId: selectedBranchId,
    page: Math.max(1, Number(value("inventory_page")) || 1),
    pageSize: Math.max(1, Number(value("inventory_size")) || 20)
  };
  const [products, inventory] = await Promise.all([
    getProducts(undefined, { page: 1, pageSize: 100 }),
    getInventory(selectedBranchId, filters)
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="สต๊อกจริง"
        description="ดู รับเข้า และปรับยอดสต๊อกจริงสำหรับการดำเนินงานประจำวัน"
      />
      <InventoryConsole
        branches={branchOptions}
        canManageGhost={session.user.role_key === "super_admin"}
        defaultBranchId={selectedBranchId}
        filters={filters}
        inventory={inventory.items}
        manageBucket="real"
        pagination={inventory.pagination}
        products={products.items}
        routePath="/real-inventory"
        showFullTimestamp={session.user.role_key === "super_admin"}
      />
    </div>
  );
}
