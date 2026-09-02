"use client";

import { useState, type ReactNode } from "react";
import { Plus } from "lucide-react";

import { TransferConsole } from "@/components/sections/transfer-console";
import { Button } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

/**
 * The transfers screen. Requisitions moved out to their own เบิกสินค้า menu, so
 * this is just the transfer list, its history, and the create action.
 */
export function TransfersWorkspace({
  branches,
  products,
  transfers,
  historySlot,
  canUseGhost = false
}: {
  branches: Option[];
  products: Option[];
  transfers: Option[];
  /** Server-rendered history table, passed through so it stays off the client bundle. */
  historySlot: ReactNode;
  canUseGhost?: boolean;
}) {
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Button onClick={() => setCreateOpen(true)} type="button">
          <Plus className="h-4 w-4" />
          สร้างรายการโอนสินค้า
        </Button>
      </div>
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
      <div>{historySlot}</div>
    </div>
  );
}
