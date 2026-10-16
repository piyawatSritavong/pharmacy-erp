import type { Metadata } from "next";
import Link from "next/link";
import { MetricGrid, PageIntro, SectionCard } from "@/components/sections/common";
import { currency } from "@/lib/utils";
import { SalesSummaryInvoices } from "@/components/sections/sales-summary-invoices";
import { Download, FileSpreadsheet } from "lucide-react";
import { Button, buttonVariants, Input } from "@/components/ui/primitives";
import { requirePermission } from "@/lib/rbac";
import { getDailySales, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "สรุปยอดขาย" };

/** One-tap ranges, in Bangkok business days. */
function quickRanges() {
  const day = (date: Date) => date.toLocaleDateString("en-CA", { timeZone: "Asia/Bangkok" });
  const now = new Date();
  const today = day(now);
  const weekAgo = day(new Date(now.getTime() - 6 * 86_400_000));
  const monthStart = `${today.slice(0, 8)}01`;
  return [
    { label: "วันนี้", start: today, end: today },
    { label: "7 วันล่าสุด", start: weekAgo, end: today },
    { label: "เดือนนี้", start: monthStart, end: today }
  ];
}

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
      <MetricGrid
        items={metrics.map((metric) => ({
          ...metric,
          value: metric.key === "invoice_count" ? Number(metric.value || 0).toLocaleString("th-TH") : currency(Number(metric.value || 0))
        }))}
      />
      <SectionCard title="ช่วงวันที่" description={String(summary.range_label || selectedStart)}>
        <div className="mb-3 flex flex-wrap gap-2">
          {quickRanges().map((range) => (
            <Link
              className={buttonVariants({
                variant: range.start === selectedStart && range.end === selectedEnd ? "default" : "secondary",
                className: "h-10 rounded-full px-4"
              })}
              href={`/daily-sales?start_date=${range.start}&end_date=${range.end}`}
              key={range.label}
            >
              {range.label}
            </Link>
          ))}
        </div>
        <form action="/daily-sales" className="grid grid-cols-2 gap-3 lg:grid-cols-[220px_220px_auto_auto_auto]">
          <label className="space-y-1 text-xs font-semibold text-muted-foreground">
            วันที่เริ่มต้น
            <Input aria-label="วันที่เริ่มต้น" defaultValue={selectedStart} name="start_date" required type="date" />
          </label>
          <label className="space-y-1 text-xs font-semibold text-muted-foreground">
            วันที่สิ้นสุด
            <Input aria-label="วันที่สิ้นสุด" defaultValue={selectedEnd} name="end_date" required type="date" />
          </label>
          <Button className="col-span-2 self-end lg:col-span-1" type="submit">แสดงสรุปยอด</Button>
          <a
            className={buttonVariants({ variant: "secondary", className: "h-11 self-end sm:h-10" })}
            href={`/api/backend/dashboard/sales-export?${exportQuery.toString()}&format=pdf`}
          >
            <Download className="h-4 w-4" />ส่งออก PDF
          </a>
          <a
            className={buttonVariants({ variant: "secondary", className: "h-11 self-end sm:h-10" })}
            href={`/api/backend/dashboard/sales-export?${exportQuery.toString()}&format=xlsx`}
          >
            <FileSpreadsheet className="h-4 w-4" />ส่งออก Excel
          </a>
        </form>
      </SectionCard>
      <SalesSummaryInvoices
        dateFrom={selectedStart}
        dateTo={selectedEnd}
        everyone={everyone}
        groups={(summary.invoice_groups as Array<Record<string, unknown>>) || []}
        invoices={recentInvoices}
        total={Number(summary.invoice_total || recentInvoices.length)}
      />
    </div>
  );
}
