import { PageIntro } from "@/components/sections/common";
import { MonthEndSummaryReportConsole } from "@/components/sections/month-end-summary-report-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getMonthEndReconciliations, requireSession } from "@/services/erp";

function text(value: unknown) {
  return value == null ? "" : String(value);
}

export default async function MonthEndReportPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const [branches, reconciliations] = await Promise.all([
    getBranches(),
    getMonthEndReconciliations()
  ]);
  const sellingBranches = branches.items.filter((branch) => text(branch.branch_type) !== "main_warehouse");

  return (
    <div className="space-y-6">
      <PageIntro
        title="รายงานสรุปสิ้นเดือน"
        description="เปรียบเทียบเลขใบขาย ราคา สถานะ และแหล่งตัด Real Stock/Ghost Stock ก่อนกับหลังการปิดรอบ"
      />
      <MonthEndSummaryReportConsole
        branches={sellingBranches.map((branch) => ({ id: text(branch.id), name: text(branch.name) }))}
        reconciliations={reconciliations.items.map((item) => ({
          id: text(item.id),
          reconciliation_number: text(item.reconciliation_number),
          period_start: text(item.period_start).slice(0, 10),
          period_end: text(item.period_end).slice(0, 10),
          finalized_at: text(item.finalized_at)
        }))}
      />
    </div>
  );
}
