import { redirect } from "next/navigation";

export default async function AuditRedirectPage() {
  redirect("/settings?tab=audit");
}
