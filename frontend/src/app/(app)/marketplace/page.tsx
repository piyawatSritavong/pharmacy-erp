import { redirect } from "next/navigation";

export default async function MarketplaceRedirectPage() {
  redirect("/settings?tab=marketplace");
}
