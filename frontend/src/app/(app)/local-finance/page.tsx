import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { FinanceConsole } from "@/components/sections/finance-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getChecks, getOutstandingInvoices, requireSession } from "@/services/erp";

export default async function LocalFinancePage() {
  const session = requireRole(await requireSession(), ["branch_admin"]);
  const [branches, checks, outstandingInvoices] = await Promise.all([
    getBranches(),
    getChecks(),
    getOutstandingInvoices()
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(session.user.branch_id || ""));

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Finance"
        title="Local Finance"
        description="Match unpaid branch invoices against checks. The backend validates exact totals before applying payment."
      />
      <Grid>
        <FinanceConsole
          branches={branchOptions}
          defaultBranchId={session.user.branch_id}
          outstandingInvoices={outstandingInvoices.items}
        />
        <SectionCard title="Saved Checks" description="Branch-scoped check queue">
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
