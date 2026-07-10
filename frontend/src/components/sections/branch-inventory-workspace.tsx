"use client";

import { DataTable, Grid, SectionCard } from "@/components/sections/common";
import { InventoryConsole } from "@/components/sections/inventory-console";
import { TransferConsole } from "@/components/sections/transfer-console";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

export function BranchInventoryWorkspace({
  inventoryBranches,
  transferBranches,
  products,
  inventory,
  transfers,
  defaultBranchId,
  defaultTab = "inventory"
}: {
  inventoryBranches: Option[];
  transferBranches: Option[];
  products: Option[];
  inventory: Option[];
  transfers: Option[];
  defaultBranchId?: string;
  defaultTab?: "inventory" | "transfers";
}) {
  return (
    <Tabs className="space-y-6" defaultValue={defaultTab}>
      <TabsList>
        <TabsTrigger value="inventory">Inventory Controls</TabsTrigger>
        <TabsTrigger value="transfers">Transfers</TabsTrigger>
      </TabsList>

      <TabsContent value="inventory">
        <Grid>
          <InventoryConsole branches={inventoryBranches} defaultBranchId={defaultBranchId} products={products} />
          <SectionCard title="Current Stock" description="Live branch inventory">
            <DataTable
              columns={[
                { key: "product_name", label: "Product" },
                { key: "sku", label: "SKU" },
                { key: "qty_real", label: "Real" },
                { key: "qty_ghost", label: "Ghost" },
                { key: "price", label: "Price", type: "currency" }
              ]}
              rows={inventory}
            />
          </SectionCard>
        </Grid>
      </TabsContent>

      <TabsContent value="transfers">
        <Grid>
          <TransferConsole
            branches={transferBranches}
            defaultBranchId={defaultBranchId}
            mode="admin"
            products={products}
            transfers={transfers}
          />
          <SectionCard title="Transfer Queue" description="Current transfer state returned by the backend">
            <DataTable
              columns={[
                { key: "transfer_code", label: "Transfer Code" },
                { key: "source_branch_name", label: "Source" },
                { key: "destination_branch_name", label: "Destination" },
                { key: "status", label: "Status" },
                { key: "requested_at", label: "Requested", type: "datetime" }
              ]}
              rows={transfers}
            />
          </SectionCard>
        </Grid>
      </TabsContent>
    </Tabs>
  );
}
