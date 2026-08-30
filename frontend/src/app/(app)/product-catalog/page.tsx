import { PageIntro } from "@/components/sections/common";
import { ProductCatalogConsole } from "@/components/sections/product-catalog-console";
import { requirePermission } from "@/lib/rbac";
import { getProductCategories, getProducts, requireSession } from "@/services/erp";

export default async function ProductCatalogPage({
  searchParams
}: {
  searchParams?: Promise<{
    search?: string;
    category_id?: string;
    sales_channel?: string;
    page?: string;
    page_size?: string;
  }>;
}) {
  requirePermission(await requireSession(), ["products.view", "products.manage"]);
  const resolved = await searchParams;
  const search = resolved?.search || "";
  const categoryId = resolved?.category_id || "";
  const salesChannel = resolved?.sales_channel || "";
  const page = Math.max(1, Number(resolved?.page) || 1);
  const pageSize = Math.max(1, Number(resolved?.page_size) || 20);

  const [products, categories] = await Promise.all([
    getProducts(undefined, { search, categoryId, salesChannel, page, pageSize }),
    getProductCategories()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="รายการสินค้า"
        description="แหล่งข้อมูลสินค้าเดียวที่ทุกสาขาดึงไปใช้ — อุปกรณ์ ยา และสินค้าออนไลน์อยู่ในที่เดียวกัน"
      />
      <ProductCatalogConsole
        categories={categories.items}
        defaultCategoryId={categoryId}
        defaultSalesChannel={salesChannel}
        defaultSearch={search}
        initialItems={products.items}
        pagination={products.pagination}
      />
    </div>
  );
}
