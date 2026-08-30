import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function InventoryRedirectPage() {
  const session = await requireSession();
  // /inventory-management was removed in an earlier merge — real-inventory
  // is its replacement (D2).
  if (session.user.permissions.includes("inventory.manage.global")) {
    redirect("/real-inventory");
  }
  if (session.user.permissions.includes("inventory.view.branch")) {
    redirect("/inventory-check");
  }
  redirect(session.home_path);
}
