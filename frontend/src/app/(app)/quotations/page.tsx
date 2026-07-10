import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function QuotationsRedirectPage() {
  const session = await requireSession();
  if (session.user.role_key === "branch_admin") {
    redirect("/sales-invoices");
  }
  redirect(session.home_path);
}
