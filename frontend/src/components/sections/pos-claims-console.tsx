"use client";

import { useEffect, useState } from "react";
import { PackageOpen, RotateCcw } from "lucide-react";
import { toast } from "sonner";

import { DataTable, SectionCard, statusLabel } from "@/components/sections/common";
import { Button, Dialog, DialogContent, DialogHeader, Input, Select, Textarea } from "@/components/ui/primitives";
import { Field } from "@/components/ui/field";
import { proxyClient } from "@/services/api";
import { useRefresh } from "@/components/layout/refresh-indicator";

type Option = Record<string, unknown>;

function text(value: unknown) {
  return value == null ? "" : String(value);
}

/**
 * POS เคลม/คืนสินค้า — a cashier raises a return for something they sold: pick
 * the bill, the line, how many, and why. Admin turns it into a supplier claim
 * later; here it is just the front-counter act of taking goods back.
 */
export function PosClaimsConsole({ initialItems }: { initialItems: Option[] }) {
  const refresh = useRefresh();
  const [open, setOpen] = useState(false);
  const [invoiceId, setInvoiceId] = useState("");
  const [items, setItems] = useState<Option[]>([]);
  // The bill's lines load into the select; a failure is shown under it, not
  // only in a toast that fades and leaves an empty select behind.
  const [itemsState, setItemsState] = useState<"idle" | "loading" | "error">("idle");
  const [itemsError, setItemsError] = useState("");
  const [itemId, setItemId] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(false);
  // Bills are found by searching, not picked from a list of every bill the
  // branch ever issued: that list was the whole history, on a phone.
  const [billSearch, setBillSearch] = useState("");
  const [bills, setBills] = useState<Option[]>([]);
  const [billsLoading, setBillsLoading] = useState(false);
  const [chosenBill, setChosenBill] = useState<Option | null>(null);

  useEffect(() => {
    if (!open || chosenBill) return;
    let cancelled = false;
    setBillsLoading(true);
    const timer = window.setTimeout(() => {
      const query = new URLSearchParams({ page_size: "8" });
      if (billSearch.trim()) query.set("search", billSearch.trim());
      proxyClient<{ items: Option[] }>(`/invoices?${query.toString()}`)
        .then((result) => { if (!cancelled) setBills(result.items || []); })
        .catch(() => { if (!cancelled) setBills([]); })
        .finally(() => { if (!cancelled) setBillsLoading(false); });
    }, 250);
    return () => { cancelled = true; window.clearTimeout(timer); };
  }, [billSearch, chosenBill, open]);

  function chooseBill(bill: Option | null) {
    setChosenBill(bill);
    setInvoiceId(bill ? text(bill.id) : "");
    setItemId("");
  }

  useEffect(() => {
    if (!invoiceId) {
      setItems([]);
      setItemId("");
      return;
    }
    let cancelled = false;
    setItemsState("loading");
    void (async () => {
      try {
        const detail = await proxyClient<{ items: Option[] }>(`/invoices/${invoiceId}`);
        if (!cancelled) {
          setItems(detail.items || []);
          setItemsState("idle");
        }
      } catch (error) {
        if (cancelled) return;
        const message = error instanceof Error ? error.message : "โหลดรายการในบิลไม่สำเร็จ";
        setItems([]);
        setItemsError(message);
        setItemsState("error");
        toast.error(message);
      }
    })();
    return () => { cancelled = true; };
  }, [invoiceId]);

  async function submit() {
    if (!itemId || Number(quantity) <= 0) {
      toast.error("เลือกสินค้าและจำนวนที่จะคืน");
      return;
    }
    setLoading(true);
    try {
      const result = await proxyClient<{ message: string }>("/product-returns", {
        method: "POST",
        body: JSON.stringify({ invoice_item_id: itemId, quantity: Number(quantity), reason })
      });
      toast.success(result.message || "บันทึกการคืนสินค้าแล้ว");
      setOpen(false);
      chooseBill(null);
      setBillSearch("");
      setItemId("");
      setQuantity("1");
      setReason("");
      refresh();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "บันทึกการคืนสินค้าไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      <SectionCard
        title="รายการเคลม/คืนสินค้าของสาขา"
        description="สินค้าที่คืนจากลูกค้าและสถานะการเคลม"
        actions={<Button onClick={() => setOpen(true)} type="button"><RotateCcw className="h-4 w-4" />แจ้งเคลม/คืนสินค้า</Button>}
      >
        <DataTable mobileCards
          columns={[
            { key: "product_name", label: "สินค้า" },
            { key: "quantity", label: "จำนวน" },
            { key: "reason", label: "เหตุผล" },
            { key: "status", label: "สถานะ" },
            { key: "created_at", label: "วันที่แจ้ง", type: "datetime" }
          ]}
          emptyDescription="ยังไม่มีการคืนสินค้าในสาขานี้"
          rows={initialItems.map((item) => ({ ...item, status: statusLabel(item.status) }))}
        />
      </SectionCard>

      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent>
          <DialogHeader title="แจ้งเคลม/คืนสินค้า" description="เลือกบิลและรายการที่ลูกค้านำมาคืน" />
          <div className="grid gap-4">
            <Field label="บิลที่ขายไปแล้ว">
              {chosenBill ? (
                <div className="flex items-center justify-between gap-2 rounded-xl border border-primary bg-primary/5 px-3 py-2">
                  <span className="min-w-0">
                    <span className="block font-semibold">{text(chosenBill.invoice_number)}</span>
                    <span className="block truncate text-xs text-muted-foreground">{text(chosenBill.customer_name) || "ลูกค้าหน้าร้าน"}</span>
                  </span>
                  <Button onClick={() => chooseBill(null)} type="button" variant="secondary">เปลี่ยนบิล</Button>
                </div>
              ) : (
                <div className="space-y-2">
                  <Input aria-label="ค้นหาบิล" onChange={(event) => setBillSearch(event.target.value)} placeholder="เลขที่บิล หรือชื่อลูกค้า" value={billSearch} />
                  <div className="max-h-56 space-y-1 overflow-y-auto" role="listbox" aria-label="ผลการค้นหาบิล">
                    {billsLoading && !bills.length ? <p className="px-1 py-2 text-sm text-muted-foreground">กำลังค้นหา...</p> : null}
                    {!billsLoading && !bills.length ? <p className="px-1 py-2 text-sm text-muted-foreground">ไม่พบบิล</p> : null}
                    {bills.map((bill) => (
                      <button
                        aria-selected={false}
                        className="flex min-h-11 w-full items-center justify-between gap-2 rounded-xl border px-3 py-2 text-left text-sm hover:bg-muted"
                        key={text(bill.id)}
                        onClick={() => chooseBill(bill)}
                        role="option"
                        type="button"
                      >
                        <span className="min-w-0">
                          <span className="block font-semibold">{text(bill.invoice_number)}</span>
                          <span className="block truncate text-xs text-muted-foreground">{text(bill.customer_name) || "ลูกค้าหน้าร้าน"}</span>
                        </span>
                        <span className="shrink-0 tabular-nums">{Number(bill.total_amount || 0).toLocaleString("th-TH", { minimumFractionDigits: 2 })}</span>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </Field>
            <Field error={itemsState === "error" ? itemsError : undefined} label="สินค้าที่คืน">
              <Select aria-label="สินค้าที่คืน" disabled={!invoiceId || itemsState === "loading"} onChange={(event) => setItemId(event.target.value)} value={itemId}>
                <option value="">{!invoiceId ? "เลือกบิลก่อน" : itemsState === "loading" ? "กำลังโหลดรายการในบิล..." : itemsState === "error" ? "โหลดรายการไม่สำเร็จ" : "เลือกสินค้า"}</option>
                {items.map((item) => (
                  <option key={text(item.id)} value={text(item.id)}>
                    {text(item.actual_name || item.display_name)} · ขายไป {text(item.quantity)}
                  </option>
                ))}
              </Select>
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="จำนวนที่คืน">
                <Input aria-label="จำนวนที่คืน" min="1" onChange={(event) => setQuantity(event.target.value)} type="number" value={quantity} />
              </Field>
              <Field label="เหตุผล">
                <Input aria-label="เหตุผลการคืน" onChange={(event) => setReason(event.target.value)} placeholder="เช่น ชำรุด, หมดอายุ" value={reason} />
              </Field>
            </div>
          </div>
          <div className="mt-5 flex justify-end gap-2">
            <Button onClick={() => setOpen(false)} type="button" variant="ghost">ยกเลิก</Button>
            <Button disabled={!itemId || Number(quantity) <= 0 || loading} onClick={() => void submit()} type="button">
              <PackageOpen className="h-4 w-4" />{loading ? "กำลังบันทึก..." : "บันทึกการคืน"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
