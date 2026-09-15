"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { PauseCircle, PlayCircle, Trash2 } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  ErrorState,
  LoadingState,
  Notice
} from "@/components/ui/primitives";
import { currency, dateTime } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type ParkedBill = {
  id: string;
  customer_name: string;
  note: string;
  item_count: number;
  estimated_total: number;
  expires_at: string;
  created_at: string;
  created_by_name: string;
  is_mine: boolean;
};

/** Key the POS screen reads its resumed cart from. */
export const RESUME_KEY = "pharmacy-erp:resume-parked-bill";

function hoursLeft(expiresAt: string) {
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (ms <= 0) return "หมดอายุแล้ว";
  const hours = Math.floor(ms / 3_600_000);
  const minutes = Math.floor((ms % 3_600_000) / 60_000);
  return hours > 0 ? `เหลือ ${hours} ชม. ${minutes} นาที` : `เหลือ ${minutes} นาที`;
}

/**
 * พักบิล — bills the counter suspended mid-sale. Nothing here has touched
 * stock: resuming just loads the lines back into the POS cart, where prices
 * and lot availability are re-checked from scratch.
 */
export function ParkedBillsConsole() {
  const router = useRouter();
  const [items, setItems] = useState<ParkedBill[] | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busyId, setBusyId] = useState("");
  const [abandon, setAbandon] = useState<ParkedBill | null>(null);

  const load = useCallback(async () => {
    try {
      const response = await proxyClient<{ items: ParkedBill[] }>("/parked-bills");
      setItems(response.items);
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "โหลดบิลที่พักไว้ไม่สำเร็จ");
      setItems([]);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function resume(bill: ParkedBill) {
    setBusyId(bill.id);
    try {
      const full = await proxyClient<Record<string, unknown>>(`/parked-bills/${bill.id}`);
      // Hand the lines to the POS screen, then drop the parked copy so the
      // same cart can't be resumed twice onto two terminals.
      window.sessionStorage.setItem(RESUME_KEY, JSON.stringify(full));
      await proxyClient(`/parked-bills/${bill.id}`, { method: "DELETE" });
      router.push("/sales");
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "เรียกบิลกลับมาไม่สำเร็จ");
      setBusyId("");
    }
  }

  async function confirmAbandon() {
    if (!abandon) return;
    setBusyId(abandon.id);
    try {
      const result = await proxyClient<{ message: string }>(`/parked-bills/${abandon.id}`, { method: "DELETE" });
      setMessage(result.message);
      setAbandon(null);
      await load();
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ยกเลิกบิลไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
  }

  return (
    <div className="space-y-4">
      {message ? <Notice tone="info">{message}</Notice> : null}
      <SectionCard
        description="บิลที่พักไว้ยังไม่ตัดสต๊อกและยังไม่ออกใบเสร็จ · ระบบจะลบอัตโนมัติเมื่อครบ 24 ชั่วโมง"
        title="บิลที่พักไว้"
      >
        {items === null ? (
          <LoadingState label="กำลังโหลดบิลที่พักไว้..." />
        ) : error ? (
          <ErrorState description={error} title="โหลดบิลที่พักไว้ไม่สำเร็จ" />
        ) : items.length === 0 ? (
          <EmptyState description="พักบิลได้จากหน้าขายหน้าร้าน เมื่อลูกค้ายังไม่พร้อมชำระเงิน" icon={PauseCircle} />
        ) : (
          <div className="space-y-3">
            {items.map((bill) => (
              <article className="flex flex-wrap items-center gap-3 rounded-2xl border bg-card p-4" key={bill.id}>
                <div className="min-w-0 basis-full sm:basis-auto sm:min-w-[220px] flex-1">
                  <p className="font-semibold">
                    {bill.customer_name || "ลูกค้าหน้าร้าน"}
                    <span className="ml-2 font-normal text-muted-foreground">
                      {bill.item_count.toLocaleString("th-TH")} ชิ้น · {currency(Number(bill.estimated_total))}
                    </span>
                  </p>
                  {bill.note ? <p className="text-sm text-muted-foreground">{bill.note}</p> : null}
                  <p className="text-xs text-muted-foreground">
                    พักเมื่อ {dateTime(bill.created_at)} · โดย {bill.created_by_name || "-"}
                    {bill.is_mine ? " (คุณ)" : ""}
                  </p>
                </div>
                <span className="rounded-full bg-warning-50 px-3 py-1 text-xs font-semibold text-warning-800">
                  {hoursLeft(bill.expires_at)}
                </span>
                <div className="flex gap-2">
                  <Button disabled={Boolean(busyId)} onClick={() => void resume(bill)} type="button">
                    <PlayCircle className="h-4 w-4" />
                    {busyId === bill.id ? "กำลังเรียกกลับ..." : "เรียกบิลกลับมาขาย"}
                  </Button>
                  <Button disabled={Boolean(busyId)} onClick={() => setAbandon(bill)} type="button" variant="destructive">
                    <Trash2 className="h-4 w-4" />
                    ทิ้งบิล
                  </Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </SectionCard>

      <Dialog onOpenChange={(open) => !open && setAbandon(null)} open={Boolean(abandon)}>
        <DialogContent className="max-w-md">
          <DialogHeader
            description="บิลนี้จะถูกลบทิ้ง ไม่มีผลกับสต๊อกเพราะยังไม่เคยตัดสต๊อก"
            title={`ทิ้งบิลของ ${abandon?.customer_name || "ลูกค้าหน้าร้าน"}?`}
          />
          <div className="flex justify-end gap-2">
            <Button onClick={() => setAbandon(null)} type="button" variant="secondary">ยกเลิก</Button>
            <Button disabled={Boolean(busyId)} onClick={() => void confirmAbandon()} type="button" variant="destructive">
              ทิ้งบิล
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
