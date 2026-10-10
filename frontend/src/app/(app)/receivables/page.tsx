import type { Metadata } from "next";

import { PageIntro } from "@/components/sections/common";
import { ReceivablesConsole } from "@/components/sections/receivables-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ลูกหนี้ค้างชำระ" };

export default async function ReceivablesPage({ searchParams }: { searchParams: Promise<{ customer_id?: string }> }) {
  const session = requirePermission(await requireSession(), ["receivable.view"]);
  const params = await searchParams;
  const globalScope = session.user.scope === "global";
  const branches = globalScope ? (await getBranches()).items : [];
  return (
    <div className="space-y-6">
      <PageIntro
        title="ลูกหนี้ค้างชำระ"
        description="บิลขายเชื่อที่ยังไม่ได้รับเงิน แยกตามอายุหนี้ รับชำระได้ทั้งหมดหรือบางส่วน ลูกค้าจ่ายที่สาขาไหนก็ได้"
      />
      <ReceivablesConsole
        branches={branches}
        canCollect={session.user.permissions.includes("payment.collect")}
        globalScope={globalScope}
        initialCustomerId={params.customer_id || ""}
      />
    </div>
  );
}
