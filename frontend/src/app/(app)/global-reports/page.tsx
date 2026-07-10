import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { requireRole } from "@/lib/rbac";
import { getProfitLossReport, getTaxReport, requireSession } from "@/services/erp";

export default async function GlobalReportsPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const [tax, profitLoss] = await Promise.all([getTaxReport(), getProfitLossReport()]);

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Reports"
        title="Global Reports"
        description="Tax and profit/loss reports aggregated centrally from persisted invoice and cost snapshot data."
      />
      <Grid>
        <SectionCard title="Tax Report" description="Branch summary for issued invoices">
          <DataTable
            columns={[
              { key: "branch_name", label: "Branch" },
              { key: "invoice_count", label: "Invoices" },
              { key: "subtotal", label: "Subtotal", type: "currency" },
              { key: "tax_amount", label: "VAT", type: "currency" },
              { key: "total_amount", label: "Total", type: "currency" }
            ]}
            rows={tax.items}
          />
        </SectionCard>
        <SectionCard title="Profit / Loss" description="Revenue less cost snapshot from invoice lines">
          <DataTable
            columns={[
              { key: "branch_name", label: "Branch" },
              { key: "revenue", label: "Revenue", type: "currency" },
              { key: "cost", label: "Cost", type: "currency" },
              { key: "profit", label: "Profit", type: "currency" }
            ]}
            rows={profitLoss.items}
          />
        </SectionCard>
      </Grid>
    </div>
  );
}
