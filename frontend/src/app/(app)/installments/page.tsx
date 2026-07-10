import { PageIntro } from "@/components/sections/common";
import { InstallmentConsole } from "@/components/sections/installment-console";
import { requireRole } from "@/lib/rbac";
import { getInstallments, getInvoices, requireSession } from "@/services/erp";

export default async function InstallmentsPage() {
  const session = await requireSession();
  requireRole(session, ["super_admin", "branch_admin", "branch_pos"]);
  const canManage = session.user.permissions.includes("installment.manage");
  const canCollect = session.user.permissions.includes("installment.collect");

  const [installments, invoices] = await Promise.all([
    getInstallments(),
    canManage ? getInvoices() : Promise.resolve({ items: [] })
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Billing"
        title="Installment Plans"
        description="Split unpaid invoices into monthly installments, collect payments per due date, and track overdue balances."
      />
      <InstallmentConsole
        canCollect={canCollect}
        canManage={canManage}
        invoices={invoices.items}
        plans={installments.items}
        summary={installments.summary}
      />
    </div>
  );
}
