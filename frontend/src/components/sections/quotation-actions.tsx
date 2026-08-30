"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Button, Input, Notice, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

export function ConvertQuotationButton({ id }: { id: string }) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null);
  const [lots, setLots] = useState<Record<string, Array<Record<string, unknown>>>>({});
  const [allocations, setAllocations] = useState<Array<{ key: string; quotation_item_id: string; product_id: string; stock_bucket: string; inventory_lot_id: string; quantity: string }>>([]);
  const [message, setMessage] = useState("");

  async function openDialog() {
    setOpen(true);
    setLoading(true);
    setMessage("");
    try {
      const result = await proxyClient<Record<string, unknown>>(`/quotations/${id}`);
      const items = (result.items as Array<Record<string, unknown>>) || [];
      const loadedLots = await Promise.all(items.map(async (item) => {
        const query = new URLSearchParams({
          branch_id: String(result.branch_id),
          product_id: String(item.product_id),
          stock_bucket: String(item.stock_bucket)
        });
        const response = await proxyClient<{ items: Array<Record<string, unknown>> }>(`/sales/lot-options?${query.toString()}`);
        return { itemID: String(item.id), items: response.items };
      }));
      setDetail(result);
      setLots(Object.fromEntries(loadedLots.map((entry) => [entry.itemID, entry.items])));
      setAllocations(items.map((item) => ({
        key: crypto.randomUUID(),
        quotation_item_id: String(item.id),
        product_id: String(item.product_id),
        stock_bucket: String(item.stock_bucket),
        inventory_lot_id: "",
        quantity: String(item.quantity)
      })));
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "โหลดข้อมูล Lot ไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  async function handleConvert() {
    setLoading(true);
    setMessage("");
    try {
      await proxyClient(`/quotations/${id}/convert`, {
        method: "POST",
        body: JSON.stringify({ allocations: allocations.map((item) => ({
          quotation_item_id: item.quotation_item_id,
          inventory_lot_id: item.inventory_lot_id,
          quantity: Number(item.quantity)
        })) })
      });
      setOpen(false);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ออกใบขายไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  const quoteItems = (detail?.items as Array<Record<string, unknown>>) || [];

  return (
    <>
      <Button onClick={() => void openDialog()} type="button" variant="secondary">ออกใบขาย</Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-4xl">
          <DialogHeader
            title={`เลือก Lot ก่อนออกใบขาย ${String(detail?.quote_number || "")}`}
            description="ใบเสนอราคาไม่ได้จองสต๊อก ผลรวมจำนวนที่เลือกจากแต่ละ Lot ต้องเท่ากับจำนวนในใบเสนอราคา"
          />
          {loading && !detail ? <p className="py-10 text-center text-sm text-muted-foreground">กำลังโหลด...</p> : null}
          <div className="space-y-5">
            {quoteItems.map((item) => {
              const rows = allocations.filter((allocation) => allocation.quotation_item_id === String(item.id));
              return (
                <section className="rounded-2xl border p-4" key={String(item.id)}>
                  <div className="flex items-center justify-between gap-4">
                    <div><strong>{String(item.display_name || item.product_name)}</strong><p className="text-xs text-muted-foreground">ต้องจัดสรร {Number(item.quantity).toLocaleString("th-TH")} ชิ้น</p></div>
                    <Button
                      onClick={() => setAllocations((current) => [...current, { key: crypto.randomUUID(), quotation_item_id: String(item.id), product_id: String(item.product_id), stock_bucket: String(item.stock_bucket), inventory_lot_id: "", quantity: "1" }])}
                      type="button"
                      variant="secondary"
                    >เพิ่ม Lot</Button>
                  </div>
                  <div className="mt-3 space-y-2">
                    {rows.map((allocation) => (
                      <div className="grid gap-2 sm:grid-cols-[1fr_120px_auto]" key={allocation.key}>
                        <Select value={allocation.inventory_lot_id} onChange={(event) => setAllocations((current) => current.map((row) => row.key === allocation.key ? { ...row, inventory_lot_id: event.target.value } : row))}>
                          <option value="">เลือก Lot</option>
                          {(lots[allocation.quotation_item_id] || []).map((lot) => <option key={String(lot.id)} value={String(lot.id)}>{`${String(lot.lot_number)} · คงเหลือ ${Number(lot.remaining_quantity || 0).toLocaleString("th-TH")} · ต้นทุน ${Number(lot.unit_cost || 0).toLocaleString("th-TH")}`}</option>)}
                        </Select>
                        <Input min="1" type="number" value={allocation.quantity} onChange={(event) => setAllocations((current) => current.map((row) => row.key === allocation.key ? { ...row, quantity: event.target.value } : row))} />
                        <Button disabled={rows.length === 1} onClick={() => setAllocations((current) => current.filter((row) => row.key !== allocation.key))} type="button" variant="destructive">ลบ</Button>
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>
          {message ? <Notice tone="error">{message}</Notice> : null}
          <div className="flex justify-end gap-2">
            <Button onClick={() => setOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
            <Button disabled={loading || !allocations.length || allocations.some((item) => !item.inventory_lot_id)} onClick={() => void handleConvert()} type="button">{loading ? "กำลังออกใบขาย..." : "ยืนยันออกใบขาย"}</Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

type DeleteImpact = {
  confirmation: string;
  document_number: string;
  warning: string;
  related: Record<string, number>;
};

const relatedLabels: Record<string, string> = {
  invoices: "ใบขายที่เชื่อมโยง",
  items: "รายการสินค้า",
  payments: "รายการชำระเงิน"
};

export function DeleteDocumentButton({
  id,
  kind
}: {
  id: string;
  kind: "invoice" | "quotation";
}) {
  const router = useRouter();
  const [impact, setImpact] = useState<DeleteImpact | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const basePath = kind === "invoice" ? "/invoices" : "/quotations";
  const label = kind === "invoice" ? "ใบขาย" : "ใบเสนอราคา";

  async function openDialog() {
    setLoading(true);
    setMessage("");
    try {
      const result = await proxyClient<DeleteImpact>(`${basePath}/${id}/deletion-impact`);
      setImpact(result);
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : `โหลดข้อมูล${label}ไม่สำเร็จ`);
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete() {
    if (!impact) {
      return;
    }
    setLoading(true);
    setMessage("");
    try {
      await proxyClient(`${basePath}/${id}`, {
        method: "DELETE",
        body: JSON.stringify({ confirmation })
      });
      setImpact(null);
      setConfirmation("");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : `ลบ${label}ไม่สำเร็จ`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Button disabled={loading} onClick={openDialog} type="button" variant="destructive">
        {loading && !impact ? "กำลังโหลด..." : "ลบ"}
      </Button>
      {message && !impact ? <Notice className="mt-1" tone="error">{message}</Notice> : null}
      <Dialog open={Boolean(impact)}>
        <DialogContent>
          <DialogHeader
            actions={
              <Button onClick={() => setImpact(null)} type="button" variant="secondary">
                ปิด
              </Button>
            }
            description={impact?.warning}
            title={`ลบ${label} ${impact?.document_number || ""}`}
          />
          <div className="space-y-4">
            <div className="grid gap-2 rounded-xl border bg-muted/50 p-4 sm:grid-cols-2">
              {Object.entries(impact?.related || {}).map(([key, value]) => (
                <p className="text-sm" key={key}>
                  <span className="text-muted-foreground">{relatedLabels[key] || key}:</span>{" "}
                  <strong>{value.toLocaleString("th-TH")}</strong>
                </p>
              ))}
            </div>
            <label className="block space-y-2">
              <span className="text-sm text-muted-foreground">
                พิมพ์ <strong className="text-black">{impact?.confirmation}</strong> เพื่อยืนยัน
              </span>
              <Input
                aria-label="ข้อความยืนยันลบ"
                onChange={(event) => setConfirmation(event.target.value)}
                value={confirmation}
              />
            </label>
            {message ? <Notice tone="error">{message}</Notice> : null}
            <Button
              disabled={loading || confirmation !== impact?.confirmation}
              onClick={handleDelete}
              type="button"
              variant="destructive"
            >
              {loading ? "กำลังลบ..." : `ลบ${label}และข้อมูลที่เกี่ยวข้อง`}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
