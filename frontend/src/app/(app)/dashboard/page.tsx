import { DataTable, MetricGrid, PageIntro, SectionCard } from "@/components/sections/common";
import { requireRole } from "@/lib/rbac";
import { getDashboard, requireSession } from "@/services/erp";

export default async function DashboardPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const dashboard = await getDashboard();
  const metrics = (dashboard.metrics as Array<{ key: string; label: string; value: string | number }>) || [];
  const recentInvoices = (dashboard.recent_invoices as Array<Record<string, unknown>>) || [];

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Enterprise"
        title="Dashboard"
        description="Global dashboard for all branches. Every metric and recent item is computed on the backend and rendered without client-side business logic."
      />
      <MetricGrid items={metrics} />
      <SectionCard title="Recent Invoices" description="Latest invoices from every branch">
        <DataTable
          columns={[
            { key: "invoice_number", label: "Invoice" },
            { key: "customer_name", label: "Customer" },
            { key: "branch_name", label: "Branch" },
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
