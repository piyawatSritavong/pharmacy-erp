import { PageIntro } from "@/components/sections/common";
import { AdminSalesConsole } from "@/components/sections/admin-sales-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, requireSession } from "@/services/erp";

export default async function AdminSalesPage() {
  const session = requirePermission(await requireSession(), ["invoice.create.remote"]);
  const branches = await getBranches();
  const sellingBranches = branches.items
    .filter((branch) => Boolean(branch.active ?? true) && Boolean(branch.sales_enabled ?? true) && String(branch.branch_type) !== "main_warehouse")
    .map((branch) => ({ id: String(branch.id), name: String(branch.name) }));

  return (
    <div className="space-y-6">
      <PageIntro
        title="ขายหน้าร้าน"
        description="สำนักงานใหญ่เปิดการขายในนามสาขา — เลือกสาขา เลือกวิธีขาย แล้วขายเหมือนหน้าร้าน"
      />
      <AdminSalesConsole branches={sellingBranches} operatorName={String(session.user.name || "")} />
    </div>
  );
}
