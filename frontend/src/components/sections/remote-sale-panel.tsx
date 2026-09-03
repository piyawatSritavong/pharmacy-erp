"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { MonitorSmartphone, X } from "lucide-react";
import { toast } from "sonner";

import { Field } from "@/components/ui/field";
import { Button, Dialog, DialogContent, DialogHeader, Input, Select } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type RemoteLine = {
  product_id?: unknown;
  product_name?: unknown;
  sku?: unknown;
  lot_number?: unknown;
  quantity?: unknown;
  unit_price?: unknown;
  discount_amount?: unknown;
};
type RemoteSession = {
  id: string;
  status: string;
  operator_name?: unknown;
  invoice_number?: unknown;
  cart?: {
    lines?: RemoteLine[];
    bill_discount_amount?: unknown;
    full_tax_invoice?: unknown;
    customer_name?: unknown;
  };
};

const POLL_MS = 3000;

function num(value: unknown) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}
function text(value: unknown) {
  return value == null ? "" : String(value);
}

/**
 * รีโมตหน้าร้าน, branch side. Head office rings a sale up in this branch's name
 * and the cart lands here; the cashier's only move is to take the money. The
 * lines are not editable on purpose — the server checks out the cart it holds,
 * so what the customer pays for is exactly what head office rang up, and the
 * two screens can never disagree about the price.
 */
export function RemoteSalePanel() {
  const router = useRouter();
  const [session, setSession] = useState<RemoteSession | null>(null);
  const [payOpen, setPayOpen] = useState(false);
  const [paymentType, setPaymentType] = useState<"cash" | "bank_transfer" | "mixed">("cash");
  const [tendered, setTendered] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [dismissedId, setDismissedId] = useState("");

  const load = useCallback(async () => {
    try {
      const response = await proxyClient<{ item: RemoteSession | null }>("/pos/remote-session");
      setSession(response.item);
    } catch {
      // A failed poll keeps whatever was on screen; the next tick retries.
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  const open = session && session.status === "open";
  const lines = (session?.cart?.lines || []) as RemoteLine[];
  const linesTotal = lines.reduce((sum, line) => sum + num(line.unit_price) * num(line.quantity) - num(line.discount_amount), 0);
  const billDiscount = num(session?.cart?.bill_discount_amount);
  const dueBeforeTax = Math.max(0, linesTotal - billDiscount);

  async function pay() {
    if (!open) return;
    setSubmitting(true);
    try {
      const result = await proxyClient<Option>("/pos/remote-session/checkout", {
        method: "POST",
        body: JSON.stringify({
          payment_type: paymentType,
          tendered_amount: Number(tendered) || 0,
          transfer_amount: Number(transferAmount) || 0
        })
      });
      toast.success(`รับชำระแล้ว · ${text(result.invoice_number)}`);
      setPayOpen(false);
      setTendered("");
      setTransferAmount("");
      await load();
      startTransition(() => router.refresh());
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "รับชำระไม่สำเร็จ");
    } finally {
      setSubmitting(false);
    }
  }

  if (!open || dismissedId === session?.id) return null;

  return (
    <>
      <aside className="fixed inset-x-3 bottom-24 z-40 mx-auto max-w-3xl rounded-2xl border-2 border-primary bg-white p-4 shadow-2xl xl:inset-x-auto xl:right-6 xl:w-[420px]">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 grid h-10 w-10 shrink-0 place-items-center rounded-2xl bg-primary/10 text-primary">
              <MonitorSmartphone className="h-5 w-5" />
            </span>
            <div>
              <p className="font-bold">สำนักงานใหญ่เปิดบิลรอไว้</p>
              <p className="text-xs text-muted-foreground">
                {text(session?.operator_name) || "สำนักงานใหญ่"} · รับชำระได้เลย รายการแก้ไขที่นี่ไม่ได้
              </p>
            </div>
          </div>
          <button
            aria-label="ซ่อนรายการรีโมต"
            className="grid h-8 w-8 place-items-center rounded-full text-muted-foreground transition hover:bg-muted"
            onClick={() => setDismissedId(text(session?.id))}
            type="button"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <ul className="mt-3 max-h-48 space-y-1.5 overflow-y-auto">
          {lines.map((line, index) => (
            <li className="flex items-baseline justify-between gap-3 rounded-xl bg-muted/60 px-3 py-2 text-sm" key={`${text(line.product_id)}-${index}`}>
              <span className="min-w-0">
                <span className="block truncate font-medium">{text(line.product_name)}</span>
                <span className="block text-xs text-muted-foreground">
                  {text(line.sku)}
                  {line.lot_number ? ` · Lot ${text(line.lot_number)}` : ""} × {num(line.quantity).toLocaleString("th-TH")}
                </span>
              </span>
              <span className="shrink-0 font-semibold tabular-nums">
                {currency(num(line.unit_price) * num(line.quantity) - num(line.discount_amount))}
              </span>
            </li>
          ))}
        </ul>

        <div className="mt-3 flex items-center justify-between border-t pt-3">
          <span className="text-sm text-muted-foreground">ยอดก่อนภาษี</span>
          <span className="text-lg font-bold tabular-nums">{currency(dueBeforeTax)}</span>
        </div>
        <Button className="mt-3 w-full" disabled={lines.length === 0} onClick={() => setPayOpen(true)} type="button">
          รับชำระเงิน
        </Button>
      </aside>

      <Dialog onOpenChange={setPayOpen} open={payOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader
            description="ยอดสุทธิรวมภาษีจะคำนวณตอนออกบิล — ตะกร้านี้สำนักงานใหญ่เป็นผู้จัด"
            title="รับชำระบิลจากสำนักงานใหญ่"
          />
          <div className="space-y-3">
            <Field label="วิธีชำระ">
              <Select
                aria-label="วิธีชำระ"
                onChange={(event) => setPaymentType(event.target.value as "cash" | "bank_transfer" | "mixed")}
                value={paymentType}
              >
                <option value="cash">เงินสด</option>
                <option value="bank_transfer">เงินโอน</option>
                <option value="mixed">เงินสด + เงินโอน</option>
              </Select>
            </Field>
            {paymentType === "mixed" ? (
              <Field label="ยอดเงินโอน">
                <Input aria-label="ยอดเงินโอน" min="0" onChange={(event) => setTransferAmount(event.target.value)} type="number" value={transferAmount} />
              </Field>
            ) : null}
            {paymentType !== "bank_transfer" ? (
              <Field hint="ต้องไม่น้อยกว่ายอดที่ต้องชำระ" label="รับเงินสด">
                <Input aria-label="รับเงินสด" min="0" onChange={(event) => setTendered(event.target.value)} type="number" value={tendered} />
              </Field>
            ) : null}
          </div>
          <div className="mt-5 flex justify-end gap-2">
            <Button onClick={() => setPayOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
            <Button disabled={submitting} onClick={() => void pay()} type="button">
              {submitting ? "กำลังบันทึก..." : "ยืนยันรับชำระ"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
