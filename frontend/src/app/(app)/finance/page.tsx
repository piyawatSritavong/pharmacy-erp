import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function FinanceRedirectPage() {
  const session = await requireSession();
  if (session.user.role_key === "super_admin") {
    redirect("/finance-central");
  }
  if (session.user.role_key === "branch_admin") {
    redirect("/local-finance");
  }
  redirect("/sales");
}
