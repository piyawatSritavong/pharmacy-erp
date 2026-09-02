"use client";

import { useCallback, useEffect, useState } from "react";
import { usePathname } from "next/navigation";

import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

const POLL_MS = 30_000;

function seenKey(branchId: string, menu: string) {
  return `pharmapos.pos.seen.${branchId}.${menu}`;
}
function readSeen(branchId: string, menu: string) {
  try { return Number(window.localStorage.getItem(seenKey(branchId, menu)) || "0"); } catch { return 0; }
}
function writeSeen(branchId: string, menu: string, value: number) {
  try { window.localStorage.setItem(seenKey(branchId, menu), String(value)); } catch { /* private mode */ }
}

/**
 * Badge counts for the POS bottom nav.
 *
 *   พักบิล        — how many bills are suspended right now (a live count).
 *   เบิกสินค้า     — requisitions that were approved or rejected since the
 *                    cashier last opened the menu.
 *   รับโอนสินค้า   — transfers heading to this branch, waiting to be received.
 *
 * The last two are "unseen since your last visit": opening the menu clears its
 * badge, and it only returns when something new arrives.
 */
export function usePosBadges(branchId: string): Record<string, number> {
  const pathname = usePathname();
  const [parked, setParked] = useState(0);
  const [resolvedRequisitions, setResolvedRequisitions] = useState(0);
  const [incomingTransfers, setIncomingTransfers] = useState(0);
  const [tick, setTick] = useState(0);

  const load = useCallback(async () => {
    try {
      const [parkedResponse, requisitions, transfers] = await Promise.all([
        proxyClient<{ items: Option[] }>("/parked-bills").catch(() => ({ items: [] })),
        proxyClient<{ items: Option[] }>("/stock-transfer-requests").catch(() => ({ items: [] })),
        proxyClient<{ items: Option[] }>("/transfers").catch(() => ({ items: [] }))
      ]);
      setParked(parkedResponse.items.length);
      setResolvedRequisitions(requisitions.items.filter((item) => String(item.status) !== "pending").length);
      setIncomingTransfers(
        transfers.items.filter(
          (item) => String(item.destination_branch_id) === branchId && ["requested", "in_transit"].includes(String(item.status))
        ).length
      );
    } catch {
      /* a failed poll keeps the last known counts */
    }
  }, [branchId]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  // Opening a menu marks its current count as seen, clearing the badge.
  useEffect(() => {
    if (pathname.startsWith("/requisitions")) {
      writeSeen(branchId, "requisitions", resolvedRequisitions);
      setTick((value) => value + 1);
    }
    if (pathname.startsWith("/transfer-receipts")) {
      writeSeen(branchId, "transfers", incomingTransfers);
      setTick((value) => value + 1);
    }
  }, [pathname, branchId, resolvedRequisitions, incomingTransfers]);

  // `tick` forces a recompute after a seen-baseline write.
  void tick;
  const requisitionBadge = Math.max(0, resolvedRequisitions - readSeen(branchId, "requisitions"));
  const transferBadge = Math.max(0, incomingTransfers - readSeen(branchId, "transfers"));

  return {
    parked_bills: parked,
    requisitions: requisitionBadge,
    goods_transfer_receipt: transferBadge
  };
}
