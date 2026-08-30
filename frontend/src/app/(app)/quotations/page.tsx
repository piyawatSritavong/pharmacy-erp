import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function QuotationsRedirectPage() {
  const session = await requireSession();
  if (session.user.permissions.includes("quotation.manage")) {
    redirect("/sales-management");
  }
  redirect(session.home_path);
}
