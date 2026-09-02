import { LockedPreview, ProGate } from "@/components/sections/pro-gate";
import { requireSession } from "@/services/erp";

export default async function GovernmentSalesPage() {
  await requireSession();
  return (
    <ProGate feature="เอกสาร รพ.สต.">
      <LockedPreview title="รพ.สต." description="ใบเสนอราคาและใบขายสำหรับหน่วยงานราชการโดยเฉพาะ" />
    </ProGate>
  );
}
