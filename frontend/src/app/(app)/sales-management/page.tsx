import type { Metadata } from "next";
import { LockedPreview, ProGate } from "@/components/sections/pro-gate";
import { requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ใบขาย" };

export default async function SalesManagementPage() {
  await requireSession();
  return (
    <ProGate feature="ใบเสนอราคาและใบขาย">
      <LockedPreview title="ใบขาย" description="สร้างและจัดการใบเสนอราคากับใบขายทั่วไปของทุกสาขา" />
    </ProGate>
  );
}
