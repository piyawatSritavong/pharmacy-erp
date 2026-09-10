import { PageIntro } from "@/components/sections/common";
import { PromotionConsole } from "@/components/sections/promotion-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getPromotions, requireSession } from "@/services/erp";

export default async function PromotionsPage() {
  const session = requirePermission(await requireSession(), ["promotion.manage"]);
  const [promotions, products, branches] = await Promise.all([
    getPromotions(),
    getProducts(undefined, { pageSize: 500 }),
    getBranches()
  ]);

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
        products={products.items}
        promotions={promotions.items}
      />
    </div>
  );
}
