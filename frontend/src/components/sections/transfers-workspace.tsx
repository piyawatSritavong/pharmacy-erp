"use client";

import { useState, type ReactNode } from "react";
import { Plus } from "lucide-react";

import { StockRequestConsole } from "@/components/sections/stock-request-console";
import { TransferConsole } from "@/components/sections/transfer-console";
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

/**
 * Owns the transfers screen's tab state so both tabs and the create action sit
 * on a single row — the create dialog itself still lives in TransferConsole,
 * driven from here.
 */
export function TransfersWorkspace({
  branches,
  products,
  transfers,
  requests,
  defaultTab,
  historySlot,
  canUseGhost = false
}: {
  branches: Option[];
  products: Option[];
  transfers: Option[];
  requests: Option[];
  defaultTab: string;
  /** Server-rendered history table, passed through so it stays off the client bundle. */
  historySlot: ReactNode;
  canUseGhost?: boolean;
}) {
  const [createOpen, setCreateOpen] = useState(false);
  const pendingRequests = requests.filter((item) => String(item.status) === "pending").length;

  return (
    <Tabs className="space-y-6" defaultValue={defaultTab}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <TabsList>
          <TabsTrigger value="transfers">รายการโอนสินค้า</TabsTrigger>
          <TabsTrigger value="requests">คำขอรับสินค้า ({pendingRequests})</TabsTrigger>
        </TabsList>
        <Button onClick={() => setCreateOpen(true)} type="button">
          <Plus className="h-4 w-4" />
          สร้างรายการโอนสินค้า
        </Button>
      </div>
      <TabsContent className="space-y-6" value="transfers">
        <TransferConsole
          branches={branches}
          canUseGhost={canUseGhost}
          createOpen={createOpen}
          defaultBranchId={String(branches[0]?.id || "")}
          mode="all"
          onCreateOpenChange={setCreateOpen}
          products={products}
          transfers={transfers}
        />
        {historySlot}
      </TabsContent>
      <TabsContent value="requests">
        <StockRequestConsole branches={branches} canUseGhost={canUseGhost} mode="admin" products={products} requests={requests} />
      </TabsContent>
    </Tabs>
  );
}
