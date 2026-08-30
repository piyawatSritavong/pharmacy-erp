import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function InvoicesRedirectPage() {
  const session = await requireSession();
  if (session.user.permissions.includes("invoice.create.pos")) {
    redirect("/sales");
  }
  if (session.user.permissions.includes("quotation.manage")) {
    redirect("/sales-management");
  }
  redirect(session.home_path);
}
