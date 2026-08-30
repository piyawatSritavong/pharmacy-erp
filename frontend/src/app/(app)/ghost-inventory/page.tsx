import { InventoryConsole } from "@/components/sections/inventory-console";
import { PageIntro } from "@/components/sections/common";
import { ErrorState } from "@/components/ui/primitives";
import { requireRole } from "@/lib/rbac";
import { getBranches, getInventory, getProducts, requireSession } from "@/services/erp";

export default async function GhostInventoryPage({
  searchParams
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  requireRole(await requireSession(), ["super_admin"]);
  const params = await searchParams;
  const value = (key: string) => typeof params?.[key] === "string" ? String(params[key]) : "";
  const branches = await getBranches();
  const branchOptions = branches.items.filter((item) => String(item.branch_type) === "main_warehouse" && Boolean(item.active ?? true));
  const selectedBranchId = String(branchOptions[0]?.id || "");
  if (!selectedBranchId) {
    return (
      <div className="space-y-6">
        <PageIntro
          title="สต๊อกผี"
          description="ดูยอดคงเหลือ ประวัติ และ Lot ของสต๊อกผี เฉพาะผู้ดูแลระบบสูงสุด"
        />
        <ErrorState
          description="กรุณาตรวจสอบการตั้งค่าระบบก่อนเปิดหน้านี้อีกครั้ง"
          title="ไม่พบแหล่งข้อมูลสต๊อกผี"
        />
      </div>
    );
  }
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
        title="สต๊อกผี"
        description="ดูยอดคงเหลือ ประวัติ และ Lot ของสต๊อกผี เฉพาะผู้ดูแลระบบสูงสุด"
      />
      <InventoryConsole
        branches={branchOptions}
        canManageGhost
        defaultBranchId={selectedBranchId}
        filters={filters}
        inventory={inventory.items}
        manageBucket="ghost"
        pagination={inventory.pagination}
        products={products.items}
        routePath="/ghost-inventory"
        showFullTimestamp
      />
    </div>
  );
}
