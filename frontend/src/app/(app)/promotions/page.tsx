import type { Metadata } from "next";
import { PageIntro } from "@/components/sections/common";
import { PromotionConsole } from "@/components/sections/promotion-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getPromotions, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "โปรโมชั่น" };

export default async function PromotionsPage() {
  const session = requirePermission(await requireSession(), ["promotion.manage"]);
  // Products are searched on demand by the form's picker, not preloaded:
  // the API caps a page at 200, which hid the rest of the catalogue.
  const [promotions, branches] = await Promise.all([getPromotions(), getBranches()]);

  // A shop runs its own promotions and only its own, so it is told which shop
  // rather than asked. Head office keeps the choice, including "every branch".
  const ownBranchName = session.user.scope === "global" ? "" : session.user.branch_name || "";

  return (
    <div className="space-y-6">
      <PageIntro
        description={
          ownBranchName
            ? `ตั้งส่วนลด ของแถม และราคาชุดของสาขา${ownBranchName} — หน้าร้านคิดให้อัตโนมัติและตัดสต๊อกของแถมทุกครั้งที่ขาย`
            : "ตั้งส่วนลด ของแถม และราคาชุด ให้หน้าร้านคิดให้อัตโนมัติ พร้อมตัดสต๊อกของแถมทุกครั้งที่ขาย"
        }
        title="โปรโมชั่น"
      />
      <PromotionConsole
        branches={branches.items}
        ownBranchName={ownBranchName}
        promotions={promotions.items}
      />
    </div>
  );
}
