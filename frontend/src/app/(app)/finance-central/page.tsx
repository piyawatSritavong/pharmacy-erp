import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { FinanceConsole } from "@/components/sections/finance-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getChecks, getOutstandingInvoices, requireSession } from "@/services/erp";

export default async function FinanceCentralPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const [branches, checks, outstandingInvoices] = await Promise.all([
    getBranches(),
    getChecks(),
    getOutstandingInvoices()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Finance"
        title="Finance Central"
        description="Central clearing for all branches. Matching logic and invoice state updates happen only on the backend."
      />
      <Grid>
        <FinanceConsole
          branches={branches.items}
          defaultBranchId={String(branches.items[0]?.id || "")}
          outstandingInvoices={outstandingInvoices.items}
        />
        <SectionCard title="Saved Checks" description="Enterprise-wide check queue">
          <DataTable
            columns={[
              { key: "check_number", label: "Check" },
              { key: "payer_name", label: "Payer" },
              { key: "bank_name", label: "Bank" },
              { key: "status", label: "Status" },
              { key: "amount", label: "Amount", type: "currency" }
            ]}
            rows={checks.items}
          />
        </SectionCard>
      </Grid>
    </div>
  );
}
