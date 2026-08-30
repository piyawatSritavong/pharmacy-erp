import { PageIntro } from "@/components/sections/common";
import { ParkedBillsConsole } from "@/components/sections/parked-bills-console";
import { requireSession } from "@/services/erp";

export default async function ParkedBillsPage() {
  // No permission gate by design: พักบิล is ordinary POS counter behaviour and
  // branch_pos holds no document permissions. The API scopes every read and
  // write to the caller's own branch, which is what actually bounds access.
  await requireSession();

  return (
    <div className="space-y-6">
      <PageIntro
        title="พักบิล"
        description="บิลที่พักไว้ระหว่างขาย — เรียกกลับมาชำระเงิน หรือทิ้งได้"
      />
      <ParkedBillsConsole />
    </div>
  );
}
