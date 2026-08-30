import { BranchSalesGrid } from "@/components/sections/branch-sales-grid";
import { PageIntro } from "@/components/sections/common";
import { ReportPinSlot } from "@/components/sections/report-pin-slot";
import { ErrorState } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getBranchSales, requireSession } from "@/services/erp";

export default async function DashboardPage() {
  requirePermission(await requireSession(), ["dashboard.view.global"]);
  // Degrade to an error block rather than taking the whole dashboard down —
  // the pinned reports below are independent of this fetch.
  const branchSales = await getBranchSales().catch(() => null);

  return (
    <div className="space-y-6">
      <PageIntro
        title="Dashboard"
        description="ยอดขายแต่ละสาขา และรายงานที่ปักหมุดไว้ — สร้างเพิ่มได้จากเมนู Generate Report"
      />
      {/* Fixed 3-column branch summary — the one deliberately hardcoded block
          on this page; everything below is user-pinned. */}
      {branchSales ? (
        <BranchSalesGrid
          items={branchSales.items as unknown as Parameters<typeof BranchSalesGrid>[0]["items"]}
        />
      ) : (
        <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดยอดขายรายสาขาไม่สำเร็จ" />
      )}
      <ReportPinSlot emptyPrompt pageKey="dashboard" />
    </div>
  );
}
