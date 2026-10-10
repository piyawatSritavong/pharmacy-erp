import type { Metadata } from "next";
import Link from "next/link";
import { PageIntro } from "@/components/sections/common";
import { requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ใบขาย" };

export default async function SalesManagementPage() {
  const session = await requireSession();
  return (
    <div className="space-y-4">
      <PageIntro title="ใบขาย" description="ใบเสนอราคาและใบขายเครดิตในพื้นที่ V2" />
      {session.user.role_key === "super_admin" ? <Link className="inline-flex min-h-11 items-center rounded-xl bg-primary px-4 font-semibold text-primary-foreground" href="/v2">เปิดใบเสนอราคาและใบขาย V2</Link> : <p className="rounded-xl border bg-card p-4">พื้นที่ V2 อยู่ระหว่างทดลองใช้งานโดย Superadmin</p>}
    </div>
  );
}
