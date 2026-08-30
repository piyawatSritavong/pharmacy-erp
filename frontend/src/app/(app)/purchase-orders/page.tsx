import { PageIntro } from "@/components/sections/common";
import { PurchaseOrderConsole } from "@/components/sections/purchase-order-console";
import { requirePermission } from "@/lib/rbac";
import {
  getBranches,
  getPurchaseOrders,
  getSuppliers,
  requireSession,
} from "@/services/erp";

export default async function PurchaseOrdersPage({
  searchParams
}: {
  searchParams?: Promise<{ page?: string; page_size?: string }>;
}) {
  const session = requirePermission(await requireSession(), ["purchase_orders.view.global", "purchase_orders.manage.global"]);
  const resolved = await searchParams;
  const page = Math.max(1, Number(resolved?.page) || 1);
  const pageSize = Math.max(1, Number(resolved?.page_size) || 20);
  const [orders, suppliers, branches] = await Promise.all([
    getPurchaseOrders({ page, pageSize }),
    getSuppliers({ active: "true", limit: 20 }),
    getBranches(),
  ]);
  return (
    <div className="space-y-6">
      {/* print:hidden — only the PO detail dialog's own print document
          should appear in the exported PDF, not this list page's header. */}
      <div className="print:hidden">
        <PageIntro
          title="ใบสั่งซื้อเข้า"
          description={session.user.role_key === "super_admin" ? "สร้างเอกสารซื้อสินค้าและรับเข้า Lot ของสต๊อกจริงหรือสต๊อกผี พร้อมติดตามบริษัทคู่ค้าและวันหมดอายุ" : "ดูและจัดการใบสั่งซื้อเข้าสำหรับสต๊อกจริง โดยไม่แสดงข้อมูลสต๊อกผี"}
        />
      </div>
      <PurchaseOrderConsole
        branches={branches.items}
        canUseGhost={session.user.role_key === "super_admin"}
        orders={orders.items}
        pagination={orders.pagination}
        supplierCursor={suppliers.next_cursor}
        supplierHasMore={suppliers.has_more}
        suppliers={suppliers.items}
      />
    </div>
  );
}
