import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { PageIntro } from "@/components/sections/common";
import { requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ใบขาย" };

export default async function SalesManagementPage() {
  const session = await requireSession();
  // The superadmin is the only one with somewhere to go: straight there, rather
  // than to a page whose one job is a link onward.
  if (session.user.role_key === "super_admin") redirect("/v2");
  return (
    <div className="space-y-4">
      <PageIntro title="ใบขาย" description="ใบเสนอราคาและใบขายเครดิตในพื้นที่ V2" />
      <p className="rounded-xl border bg-card p-4">พื้นที่ V2 อยู่ระหว่างทดลองใช้งานโดย Superadmin</p>
    </div>
  );
}
