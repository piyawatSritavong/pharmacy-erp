import { redirect } from "next/navigation";
import { requireSession, getBranches } from "@/services/erp";
import { V2Workspace } from "@/components/v2/workspace";

export default async function V2Page() {
  const session = await requireSession();
  if (session.user.role_key !== "super_admin") redirect("/dashboard");
  const branches = await getBranches();
  return <V2Workspace branches={branches.items.filter((item) => Boolean(item.active)).map((item) => ({ id: String(item.id), name: String(item.name) }))} />;
}
