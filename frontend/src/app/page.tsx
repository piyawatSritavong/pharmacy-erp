import { redirect } from "next/navigation";

import { getSession } from "@/services/erp";

export default async function RootPage() {
  try {
    const session = await getSession();
    redirect(session.home_path);
  } catch {
    redirect("/login");
  }
}
