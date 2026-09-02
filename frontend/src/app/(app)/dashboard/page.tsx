import { BranchSalesGrid } from "@/components/sections/branch-sales-grid";
import { PageIntro } from "@/components/sections/common";
import { LowStockTables } from "@/components/sections/low-stock-tables";
import { SalesTodayChart } from "@/components/sections/sales-today-chart";
import { ErrorState } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getBranchSales, getLowStock, getTodayBranchSales, requireSession } from "@/services/erp";

export default async function DashboardPage() {
  requirePermission(await requireSession(), ["dashboard.view.global"]);
  // Each block degrades on its own so one slow query never blanks the page.
  const [today, branchSales, lowStock] = await Promise.all([
    getTodayBranchSales().catch(() => null),
    getBranchSales().catch(() => null),
    getLowStock().catch(() => null)
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="Dashboard"
        description="ยอดขายรวมวันนี้ ยอดขายแต่ละสาขา และแจ้งเตือนสินค้าใกล้หมด"
      />
      {today ? <SalesTodayChart data={today} /> : <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดยอดขายวันนี้ไม่สำเร็จ" />}
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
