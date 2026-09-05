import { CloseComparison } from "@/components/sections/close-comparison";
import { PageIntro } from "@/components/sections/common";
import { DailyBreakdownBoards } from "@/components/sections/daily-breakdown";
import { DashboardFilters } from "@/components/sections/dashboard-filters";
import { LowStockTables } from "@/components/sections/low-stock-tables";
import { ErrorState } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getDailyBreakdown, getLowStock, getRevenueComparison, requireSession } from "@/services/erp";

/** Today where the shops are — the server may be running anywhere. */
function bangkokToday() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Bangkok" }).format(new Date());
}

export default async function DashboardPage({
  searchParams
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  const session = requirePermission(await requireSession(), ["dashboard.view.global"]);
  const params = await searchParams;
  const value = (key: string) => (typeof params?.[key] === "string" ? String(params[key]) : "");

  // The dashboard opens on today and stays there until asked otherwise: it is
  // the screen for watching trading happen, not an archive.
  const today = bangkokToday();
  const dateFrom = value("date_from") || today;
  const dateTo = value("date_to") || today;
  const roundId = value("round_id");
  const filters = {
    closeStatus: value("close_status"),
    paymentType: value("payment_type"),
    paymentStatus: value("payment_status")
  };

  // Ghost Stock is a Superadmin concept, and half of the breakdown is defined by
  // it. central_admin works from real stock alone.
  const isSuperAdmin = session.user.role_key === "super_admin";

  // Each block degrades on its own so one slow query never blanks the page.
  const [breakdown, lowStock, comparison] = await Promise.all([
    getDailyBreakdown({ dateFrom, dateTo }).catch(() => null),
    getLowStock().catch(() => null),
    isSuperAdmin && roundId
      ? getRevenueComparison({ dateFrom, dateTo }).catch(() => null)
      : Promise.resolve(null)
  ]);

  const reportQuery = new URLSearchParams({ date_from: dateFrom, date_to: dateTo });
  const live = dateFrom === today && dateTo === today;

  return (
    <div className="space-y-6">
      <PageIntro
        title="Dashboard"
        description="ยอดขายของวันนี้ตามเวลาจริง แยกตามวิธีชำระเงินและสิ่งที่รอบสิ้นเดือนจะทำกับมัน"
      />

      <DashboardFilters canSeeClose={isSuperAdmin} />

      {breakdown ? (
        <DailyBreakdownBoards data={breakdown} filters={filters} live={live} />
      ) : (
        <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดสรุปยอดไม่สำเร็จ" />
      )}

      {/* Before/after belongs to a closed round, not to a running day — so it
          appears only once the operator picks a round in the filter bar. */}
      {comparison ? <CloseComparison data={comparison} reportHref={`/month-end-report?${reportQuery.toString()}`} /> : null}

      {lowStock ? (
        <LowStockTables rows={lowStock.items as unknown as Parameters<typeof LowStockTables>[0]["rows"]} />
      ) : (
        <ErrorState description="ลองรีเฟรชหน้าอีกครั้ง" title="โหลดรายการสินค้าใกล้หมดไม่สำเร็จ" />
      )}
    </div>
  );
}
