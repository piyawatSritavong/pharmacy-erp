"use client";

import { startTransition, useEffect, useState } from "react";
import { PackageOpen, RotateCcw } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { DataTable, SectionCard, statusLabel } from "@/components/sections/common";
import { Button, Dialog, DialogContent, DialogHeader, Input, Select, Textarea } from "@/components/ui/primitives";
import { Field } from "@/components/ui/field";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

function text(value: unknown) {
  return value == null ? "" : String(value);
}

/**
 * POS เคลม/คืนสินค้า — a cashier raises a return for something they sold: pick
 * the bill, the line, how many, and why. Admin turns it into a supplier claim
 * later; here it is just the front-counter act of taking goods back.
 */
export function PosClaimsConsole({ initialItems, invoices }: { initialItems: Option[]; invoices: Option[] }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [invoiceId, setInvoiceId] = useState("");
  const [items, setItems] = useState<Option[]>([]);
  const [itemId, setItemId] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!invoiceId) {
      setItems([]);
      setItemId("");
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const detail = await proxyClient<{ items: Option[] }>(`/invoices/${invoiceId}`);
        if (!cancelled) setItems(detail.items || []);
      } catch (error) {
        if (!cancelled) toast.error(error instanceof Error ? error.message : "โหลดรายการในบิลไม่สำเร็จ");
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
      setInvoiceId("");
      setItemId("");
      setQuantity("1");
      setReason("");
      startTransition(() => router.refresh());
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
        <DataTable
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
            <Field label="เลือกบิลที่ขายไปแล้ว">
              <Select aria-label="เลือกบิล" onChange={(event) => setInvoiceId(event.target.value)} value={invoiceId}>
                <option value="">เลือกบิล</option>
                {invoices.map((invoice) => (
                  <option key={text(invoice.id)} value={text(invoice.id)}>
                    {text(invoice.invoice_number)} · {text(invoice.customer_name) || "ลูกค้าหน้าร้าน"}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="สินค้าที่คืน">
              <Select aria-label="สินค้าที่คืน" disabled={!invoiceId} onChange={(event) => setItemId(event.target.value)} value={itemId}>
                <option value="">{invoiceId ? "เลือกสินค้า" : "เลือกบิลก่อน"}</option>
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
