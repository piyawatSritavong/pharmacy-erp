import { BranchSalesGrid } from "@/components/sections/branch-sales-grid";
import { CloseComparison } from "@/components/sections/close-comparison";
import { PageIntro } from "@/components/sections/common";
import { DashboardFilters } from "@/components/sections/dashboard-filters";
import { LowStockTables } from "@/components/sections/low-stock-tables";
import { SalesTodayChart } from "@/components/sections/sales-today-chart";
import { ErrorState } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getBranchSales, getLowStock, getRevenueComparison, getTodayBranchSales, requireSession } from "@/services/erp";

export default async function DashboardPage({
  searchParams
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  const session = requirePermission(await requireSession(), ["dashboard.view.global"]);
  const params = await searchParams;
  const value = (key: string) => (typeof params?.[key] === "string" ? String(params[key]) : "");
  const scope = { dateFrom: value("date_from"), dateTo: value("date_to"), paymentStatus: value("payment_status") };

  // Only the superadmin is shown the pre-close figures; admin.central works from
  // the adjusted books alone, which is exactly what the branch grid shows.
  const isSuperAdmin = session.user.role_key === "super_admin";

  // Each block degrades on its own so one slow query never blanks the page.
  const [today, branchSales, lowStock, comparison] = await Promise.all([
    getTodayBranchSales().catch(() => null),
    getBranchSales(scope).catch(() => null),
    getLowStock().catch(() => null),
    isSuperAdmin ? getRevenueComparison(scope).catch(() => null) : Promise.resolve(null)
  ]);

  // Carry the same window into the close report, so "ดูบิล" lands on the bills
  // behind the number that was clicked.
  const reportQuery = new URLSearchParams();
  if (scope.dateFrom) reportQuery.set("date_from", scope.dateFrom);
  if (scope.dateTo) reportQuery.set("date_to", scope.dateTo);
  const reportHref = reportQuery.size ? `/month-end-report?${reportQuery.toString()}` : "/month-end-report";

  return (
    <div className="space-y-6">
      <PageIntro
        title="Dashboard"
        description="ยอดขายรวมวันนี้ ยอดขายแต่ละสาขา และแจ้งเตือนสินค้าใกล้หมด"
      />
      <DashboardFilters />
      {today ? <SalesTodayChart data={today} /> : <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดยอดขายวันนี้ไม่สำเร็จ" />}
      {isSuperAdmin && comparison ? <CloseComparison data={comparison} reportHref={reportHref} /> : null}
      {branchSales ? (
        <BranchSalesGrid items={branchSales.items as unknown as Parameters<typeof BranchSalesGrid>[0]["items"]} />
      ) : (
        <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดยอดขายรายสาขาไม่สำเร็จ" />
      )}
      {lowStock ? (
        <LowStockTables rows={lowStock.items as unknown as Parameters<typeof LowStockTables>[0]["rows"]} />
      ) : (
        <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดรายการสินค้าใกล้หมดไม่สำเร็จ" />
      )}
    </div>
  );
}
