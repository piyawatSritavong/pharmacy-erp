import { redirect } from "next/navigation";

import { requireRole } from "@/lib/rbac";
import { requireSession } from "@/services/erp";

export default async function TransfersPage() {
  requireRole(await requireSession(), ["branch_admin"]);
  redirect("/branch-inventory?tab=transfers");
}
