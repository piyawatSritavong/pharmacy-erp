import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function ReportsRedirectPage() {
  const session = await requireSession();
  if (session.user.permissions.includes("reports.view.global")) {
    redirect("/global-reports");
  }
  redirect(session.home_path);
}
