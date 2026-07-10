import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function InventoryRedirectPage() {
  const session = await requireSession();
  if (session.user.role_key === "super_admin") {
    redirect("/inventory-management");
  }
  if (session.user.role_key === "branch_admin") {
    redirect("/branch-inventory");
  }
  redirect("/inventory-check");
}
