import { MetricGrid, PageIntro, SectionCard } from "@/components/sections/common";
import { SalesSummaryInvoices } from "@/components/sections/sales-summary-invoices";
import { Download, FileSpreadsheet } from "lucide-react";
import { Button, Input } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getDailySales, requireSession } from "@/services/erp";

export default async function DailySalesPage({
  searchParams
}: {
  searchParams?: Promise<{ start_date?: string | string[]; end_date?: string | string[] }>;
}) {
  const session = requirePermission(await requireSession(), ["dashboard.view.self"]);
  // A cashier sees their own till; a global-scope user sees every branch.
  const everyone = session.user.scope === "global";
  const resolvedSearchParams = await searchParams;
  const startDate = typeof resolvedSearchParams?.start_date === "string" ? resolvedSearchParams.start_date : undefined;
  const endDate = typeof resolvedSearchParams?.end_date === "string" ? resolvedSearchParams.end_date : undefined;
  const summary = await getDailySales(startDate, endDate);
  const metrics = (summary.metrics as Array<{ key: string; label: string; value: string | number }>) || [];
  const recentInvoices = (summary.recent_invoices as Array<Record<string, unknown>>) || [];
  const selectedStart = String(summary.start_date || summary.date || "");
  const selectedEnd = String(summary.end_date || summary.date || "");
  const exportQuery = new URLSearchParams({ start_date: selectedStart, end_date: selectedEnd });

  return (
    <div className="space-y-6">
      <PageIntro
        title="สรุปยอดขาย"
        description={`${everyone ? "ยอดขายและยอดรับชำระของทุกสาขา" : "ยอดขายและยอดรับชำระของพนักงานคนปัจจุบัน"} ช่วง ${String(summary.range_label || selectedStart)}`}
      />
      <SectionCard title="เลือกช่วงวันที่" description="รองรับการสรุปรายวัน รายสัปดาห์ รายเดือน หรือช่วงวันที่ที่กำหนดเอง">
        <form action="/daily-sales" className="grid gap-3 lg:grid-cols-[220px_220px_auto_auto_auto]">
          <label className="space-y-1 text-xs font-semibold text-muted-foreground">
            วันที่เริ่มต้น
            <Input aria-label="วันที่เริ่มต้น" defaultValue={selectedStart} name="start_date" required type="date" />
          </label>
          <label className="space-y-1 text-xs font-semibold text-muted-foreground">
            วันที่สิ้นสุด
            <Input aria-label="วันที่สิ้นสุด" defaultValue={selectedEnd} name="end_date" required type="date" />
          </label>
          <Button className="self-end" type="submit">แสดงสรุปยอด</Button>
          <a
            className="inline-flex h-10 items-center justify-center gap-2 self-end rounded-lg border bg-white px-4 text-sm font-semibold hover:bg-muted"
            href={`/api/backend/dashboard/sales-export?${exportQuery.toString()}&format=pdf`}
          >
            <Download className="h-4 w-4" />Export PDF
          </a>
          <a
            className="inline-flex h-10 items-center justify-center gap-2 self-end rounded-lg border bg-white px-4 text-sm font-semibold hover:bg-muted"
            href={`/api/backend/dashboard/sales-export?${exportQuery.toString()}&format=xlsx`}
          >
            <FileSpreadsheet className="h-4 w-4" />Export Excel
          </a>
        </form>
      </SectionCard>
      <MetricGrid items={metrics} />
      <SalesSummaryInvoices everyone={everyone} invoices={recentInvoices} />
    </div>
  );
}
