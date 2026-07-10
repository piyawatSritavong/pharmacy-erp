import { DataTable, MetricGrid, PageIntro, SectionCard } from "@/components/sections/common";
import { requireRole } from "@/lib/rbac";
import { getDashboard, getMarketplaceOrders, requireSession } from "@/services/erp";

export default async function BranchDashboardPage() {
  const session = requireRole(await requireSession(), ["branch_admin"]);
  const [dashboard, marketplaceOrders] = await Promise.all([getDashboard(), getMarketplaceOrders()]);
  const metrics = (dashboard.metrics as Array<{ key: string; label: string; value: string | number }>) || [];
  const recentInvoices = (dashboard.recent_invoices as Array<Record<string, unknown>>) || [];

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Branch Overview"
        title={`Branch Dashboard${session.user.branch_name ? ` · ${session.user.branch_name}` : ""}`}
        description="ยอดขาย, สต็อก, และรายการค้างทั้งหมดมาจาก backend ตาม scope ของสาขาเท่านั้น"
      />
      <MetricGrid items={metrics} />
      <SectionCard title="Recent Branch Invoices" description="Latest invoices issued within this branch">
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
      <SectionCard title="Marketplace Inbox" description="Read-only marketplace orders routed to this branch">
        <DataTable
          columns={[
            { key: "provider_name", label: "Provider" },
            { key: "external_order_id", label: "Order" },
            { key: "customer_name", label: "Customer" },
            { key: "status", label: "Status" },
            { key: "order_total", label: "Total", type: "currency" }
          ]}
          rows={marketplaceOrders.items}
        />
      </SectionCard>
    </div>
  );
}
