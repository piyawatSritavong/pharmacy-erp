import { redirect } from "next/navigation";

import { getSession } from "@/services/erp";

export default async function RootPage() {
  // redirect() works by throwing, so it must stay outside the try: inside it,
  // the catch swallowed the redirect home and sent every signed-in visitor of
  // "/" (the 404 page's "กลับหน้าหลัก" included) to /login.
  let home = "/login";
  try {
    home = (await getSession()).home_path || "/dashboard";
  } catch {
    // No session: /login it is. getSession has already logged why.
  }
  redirect(home);
}
