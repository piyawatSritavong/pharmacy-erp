import type { Metadata } from "next";
import { LockedPreview, ProGate } from "@/components/sections/pro-gate";
import { requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "เอกสารนำส่ง อย." };

export default async function FdaReportsPage() {
  await requireSession();
  return (
    <ProGate feature="เอกสารนำส่ง อย.">
      <LockedPreview title="เอกสารนำส่ง อย." description="เลือกช่วงเวลาและสินค้าที่ต้องรายงาน แล้วพิมพ์เอกสารนำส่ง อย." />
    </ProGate>
  );
}
