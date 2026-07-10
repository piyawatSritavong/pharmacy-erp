import { redirect } from "next/navigation";

import { requireSession } from "@/services/erp";

export default async function ReportsRedirectPage() {
  const session = await requireSession();
  if (session.user.role_key === "super_admin") {
    redirect("/global-reports");
  }
  redirect(session.home_path);
}
