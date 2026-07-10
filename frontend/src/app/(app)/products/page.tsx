import { redirect } from "next/navigation";

export default async function ProductsRedirectPage() {
  redirect("/inventory-management");
}
