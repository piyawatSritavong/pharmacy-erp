import { MonthEndReconciliationConsole } from "@/components/sections/month-end-reconciliation-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, requireSession } from "@/services/erp";

export default async function MonthEndPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const branches = await getBranches();

  return <MonthEndReconciliationConsole branches={branches.items} />;
}
