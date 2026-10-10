import type { Metadata } from "next";

import { PageIntro } from "@/components/sections/common";
import { CustomerConsole } from "@/components/sections/customer-console";
import { requirePermission } from "@/lib/rbac";
import { getLoyaltySettings, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ลูกค้าและสมาชิก" };

export default async function CustomersPage() {
  const session = requirePermission(await requireSession(), ["customer.view"]);
  const permissions = session.user.permissions;
  const loyalty = await getLoyaltySettings();
  return (
    <div className="space-y-6">
      <PageIntro
        title="ลูกค้าและสมาชิก"
        description="สมาชิกใช้ร่วมกันทุกสาขา สะสมและใช้แต้มได้ทุกที่ ลูกค้าขายส่งได้ราคาส่ง และบัญชีเครดิตซื้อก่อนจ่ายทีหลังได้ตามวงเงิน"
      />
      <CustomerConsole
        canCollect={permissions.includes("payment.collect")}
        canManage={permissions.includes("customer.manage")}
        canTerms={permissions.includes("customer.credit.manage")}
        loyalty={loyalty}
      />
    </div>
  );
}
