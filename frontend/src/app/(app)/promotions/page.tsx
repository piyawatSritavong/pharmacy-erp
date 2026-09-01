import { PageIntro } from "@/components/sections/common";
import { PromotionConsole } from "@/components/sections/promotion-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getPromotions, requireSession } from "@/services/erp";

export default async function PromotionsPage() {
  requirePermission(await requireSession(), ["promotion.manage"]);
  const [promotions, products, branches] = await Promise.all([
    getPromotions(),
    getProducts(undefined, { pageSize: 500 }),
    getBranches()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        description="ตั้งส่วนลด ของแถม และราคาชุด ให้หน้าร้านคิดให้อัตโนมัติ พร้อมตัดสต๊อกของแถมทุกครั้งที่ขาย"
        title="โปรโมชั่น"
      />
      <PromotionConsole branches={branches.items} products={products.items} promotions={promotions.items} />
    </div>
  );
}
