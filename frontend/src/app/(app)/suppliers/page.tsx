import { PageIntro } from "@/components/sections/common";
import { SupplierConsole } from "@/components/sections/supplier-console";
import { requirePermission } from "@/lib/rbac";
import { getSuppliers, requireSession } from "@/services/erp";

export default async function SuppliersPage() {
  requirePermission(await requireSession(), ["suppliers.view.global", "suppliers.manage.global"]);
  // The console filters and pages client-side now (the shared Pagination
  // pattern), so the list arrives whole rather than 20 at a time behind a
  // "load more" button.
  const suppliers = await getSuppliers({ limit: 500 });
  return <div className="space-y-6"><PageIntro title="บริษัทคู่ค้า" description="จัดเก็บข้อมูลบริษัทผู้จำหน่ายส่วนกลาง เพื่อนำไปใช้ซ้ำในใบสั่งซื้อทุกสาขา" /><SupplierConsole initialItems={suppliers.items} /></div>;
}
