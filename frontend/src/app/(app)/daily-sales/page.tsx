import { DataTable, MetricGrid, PageIntro, SectionCard } from "@/components/sections/common";
import { Button, Input } from "@/components/ui/primitives";
import { requireRole } from "@/lib/rbac";
import { getDailySales, requireSession } from "@/services/erp";

export default async function DailySalesPage({
  searchParams
}: {
  searchParams?: { date?: string | string[] };
}) {
  requireRole(await requireSession(), ["branch_pos"]);
  const date = typeof searchParams?.date === "string" ? searchParams.date : undefined;
  const summary = await getDailySales(date);
  const metrics = (summary.metrics as Array<{ key: string; label: string; value: string | number }>) || [];
  const recentInvoices = (summary.recent_invoices as Array<Record<string, unknown>>) || [];

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Daily Summary"
        title={`Daily Sales Summary · ${String(summary.date || "")}`}
        description="สรุปยอดขายรายวันของพนักงานคนปัจจุบันโดยอิงจาก invoices.created_by และ invoice_payments.created_by"
      />
      <SectionCard title="Select Date" description="เลือกวันที่ตาม Bangkok timezone เพื่อดูยอดขายของพนักงานคนปัจจุบัน">
        <form action="/daily-sales" className="grid gap-3 sm:grid-cols-[220px_auto]">
          <Input defaultValue={typeof summary.date === "string" ? summary.date : ""} name="date" type="date" />
          <Button type="submit">Load Summary</Button>
        </form>
      </SectionCard>
      <MetricGrid items={metrics} />
      <SectionCard title="Your Recent Invoices" description="Only invoices created by the current POS user for the selected day">
        <DataTable
          columns={[
            { key: "invoice_number", label: "Invoice" },
            { key: "customer_name", label: "Customer" },
            { key: "payment_status", label: "Status" },
            { key: "total_amount", label: "Total", type: "currency" },
            { key: "issued_at", label: "Issued At", type: "datetime" }
          ]}
          rows={recentInvoices}
        />
      </SectionCard>
    </div>
  );
}
