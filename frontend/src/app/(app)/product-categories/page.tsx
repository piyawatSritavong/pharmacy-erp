import { PageIntro } from "@/components/sections/common";
import { ProductCategoryConsole } from "@/components/sections/product-category-console";
import { requirePermission } from "@/lib/rbac";
import { getProductCategories, requireSession } from "@/services/erp";

export default async function ProductCategoriesPage() {
  requirePermission(await requireSession(), ["products.manage"]);
  const categories = await getProductCategories();

  return (
    <div className="space-y-6">
      <PageIntro
        description="จัดกลุ่มสินค้า กำหนดสี และเก็บหมวดที่ไม่ใช้แล้วโดยไม่กระทบประวัติสินค้า"
        title="หมวดสินค้า"
      />
      <ProductCategoryConsole initialItems={categories.items} />
    </div>
  );
}
