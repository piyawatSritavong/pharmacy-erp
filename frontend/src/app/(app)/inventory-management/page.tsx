import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { InventoryConsole } from "@/components/sections/inventory-console";
import { ProductConsole } from "@/components/sections/product-console";
import { requireRole } from "@/lib/rbac";
import { getAliases, getBranches, getInventory, getProducts, requireSession } from "@/services/erp";

export default async function InventoryManagementPage() {
  requireRole(await requireSession(), ["super_admin"]);
  const [branches, products, aliases, inventory] = await Promise.all([
    getBranches(),
    getProducts(),
    getAliases(),
    getInventory()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Inventory"
        title="Inventory Management"
        description="Global stock control, master products, and alias mapping for government mode."
      />
      <Grid className="xl:grid-cols-2">
        <ProductConsole branches={branches.items} products={products.items} />
        <InventoryConsole
          branches={branches.items}
          defaultBranchId={String(branches.items[0]?.id || "")}
          products={products.items}
        />
      </Grid>
      <SectionCard title="Alias Registry" description="Government-mode alias display names and default override price">
        <DataTable
          columns={[
            { key: "alias_code", label: "Alias Code" },
            { key: "alias_name", label: "Alias Name" },
            { key: "product_name", label: "Actual Product" },
            { key: "default_government_price", label: "Gov Price", type: "currency" }
          ]}
          rows={aliases.items}
        />
      </SectionCard>
      <SectionCard title="Global Inventory" description="Current inventory across all branches">
        <DataTable
          columns={[
            { key: "branch_name", label: "Branch" },
            { key: "product_name", label: "Product" },
            { key: "sku", label: "SKU" },
            { key: "qty_real", label: "Real" },
            { key: "qty_ghost", label: "Ghost" },
            { key: "price", label: "Price", type: "currency" }
          ]}
          rows={inventory.items}
        />
      </SectionCard>
    </div>
  );
}
