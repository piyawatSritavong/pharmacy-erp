import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function InvoicesRedirectPage() {
  const session = await requireSession();
  if (session.user.role_key === "branch_admin") {
    redirect("/sales-invoices");
  }
  if (session.user.role_key === "branch_pos") {
    redirect("/sales");
  }
  redirect("/dashboard");
}
