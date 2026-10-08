import type { Metadata } from "next";
import { LockedPreview, ProGate } from "@/components/sections/pro-gate";
import { requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ตลาดออนไลน์" };

export default async function MarketplacePage() {
  await requireSession();
  return (
    <ProGate feature="ตลาดออนไลน์">
      <LockedPreview title="ตลาดออนไลน์" description="เชื่อมต่อและจัดการคำสั่งซื้อจากตลาดออนไลน์" />
    </ProGate>
  );
}
