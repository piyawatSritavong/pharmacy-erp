"use client";

import { startTransition, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, CheckCircle2, PackageCheck, Plus, Send, Trash2, TriangleAlert } from "lucide-react";

import { SectionCard, statusLabel } from "@/components/sections/common";
import { ProductSearchPicker } from "@/components/sections/product-search-picker";
import { Field } from "@/components/ui/field";
import { Badge, Button, Dialog, DialogContent, DialogHeader, EmptyState, Input, Select } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type TransferLineDraft = {
  key: string;
  product_id: string;
  quantity: string;
  stock_bucket: "real" | "ghost";
};
type ReceiptDraft = Record<string, { quantity: string; note: string }>;

function newLine(): TransferLineDraft {
  return {
    key: crypto.randomUUID(),
    product_id: "",
    quantity: "1",
    stock_bucket: "real"
  };
}

function transferItems(transfer: Option): Option[] {
  return Array.isArray(transfer.items) ? transfer.items as Option[] : [];
}

function differenceLabel(sent: number, received: number) {
  const difference = received - sent;
  if (difference === 0) return "ตรงตามรายการ";
  return difference > 0 ? `เกิน ${difference.toLocaleString("th-TH")}` : `ขาด ${Math.abs(difference).toLocaleString("th-TH")}`;
}

export function TransferConsole({
  branches,
  products,
  transfers = [],
  defaultBranchId,
  mode = "admin",
  createOpen: controlledCreateOpen,
  onCreateOpenChange,
  canUseGhost = false
}: {
  branches: Option[];
  products: Option[];
  transfers?: Option[];
  defaultBranchId?: string;
  mode?: "admin" | "receipt" | "all";
  /** Pass both to drive the create dialog from outside (e.g. a button sitting
   *  in the page's tab row); the console then hides its own trigger. */
  createOpen?: boolean;
  onCreateOpenChange?: (open: boolean) => void;
  canUseGhost?: boolean;
}) {
  const router = useRouter();
  const [sourceBranchId, setSourceBranchId] = useState(defaultBranchId || "");
  const [destinationBranchId, setDestinationBranchId] = useState("");
  const [lines, setLines] = useState<TransferLineDraft[]>([newLine()]);
  const [requestNote, setRequestNote] = useState("");
  const [pickupName, setPickupName] = useState("");
  const [courierName, setCourierName] = useState("");
  const [receiptDrafts, setReceiptDrafts] = useState<Record<string, ReceiptDraft>>({});
  const [busyId, setBusyId] = useState("");
  const [message, setMessage] = useState("");
  const [internalCreateOpen, setInternalCreateOpen] = useState(false);
  const externallyControlled = onCreateOpenChange !== undefined;
  const createOpen = externallyControlled ? Boolean(controlledCreateOpen) : internalCreateOpen;
  const setCreateOpen = externallyControlled ? onCreateOpenChange : setInternalCreateOpen;

  const receivableTransfers = transfers.filter(
    (item) =>
      String(item.destination_branch_id || "") === String(defaultBranchId || "") &&
      String(item.status || "") === "in_transit"
  );
  const dispatchableTransfers = transfers.filter((item) => String(item.status || "") === "requested");

  // Same product + same stock type = one line: when an edit makes a line
  // match another, merge the quantities into the earlier line and tell the
  // user (business-flow.md, duplicate line-item rule).
  const [mergeNotice, setMergeNotice] = useState("");
  useEffect(() => {
    if (!mergeNotice) return;
    const timer = window.setTimeout(() => setMergeNotice(""), 5000);
    return () => window.clearTimeout(timer);
  }, [mergeNotice]);

  function updateLine(key: string, patch: Partial<TransferLineDraft>) {
    setLines((current) => {
      const next = current.map((line) => (line.key === key ? { ...line, ...patch } : line));
      const edited = next.find((line) => line.key === key);
      if (!edited?.product_id) return next;
      const target = next.find(
        (line) => line.key !== key && line.product_id === edited.product_id && line.stock_bucket === edited.stock_bucket
      );
      if (!target) return next;
      const productName = String(products.find((item) => String(item.id) === edited.product_id)?.name || "สินค้านี้");
      setMergeNotice(`รวมจำนวน “${productName}” เข้ากับรายการเดิมแล้ว (สินค้าและประเภทสต๊อกเดียวกัน)`);
      return next
        .filter((line) => line.key !== key)
        .map((line) =>
          line.key === target.key
            ? { ...line, quantity: String((Number(line.quantity) || 0) + (Number(edited.quantity) || 0)) }
            : line
        );
    });
  }

  function receiptValue(transferId: string, item: Option) {
    const itemId = String(item.id);
    return receiptDrafts[transferId]?.[itemId] || {
      quantity: String(item.quantity || 0),
      note: ""
    };
  }

  function updateReceipt(transferId: string, item: Option, patch: Partial<{ quantity: string; note: string }>) {
    const itemId = String(item.id);
    setReceiptDrafts((current) => ({
      ...current,
      [transferId]: {
        ...(current[transferId] || {}),
        [itemId]: { ...receiptValue(transferId, item), ...patch }
      }
    }));
  }

  async function createTransfer() {
    setBusyId("create");
    try {
      const response = await proxyClient<{ message: string }>("/transfers", {
        method: "POST",
        body: JSON.stringify({
          source_branch_id: sourceBranchId,
          destination_branch_id: destinationBranchId,
          request_note: requestNote,
          pickup_name: pickupName,
          courier_name: courierName,
          items: lines.map((line) => ({
            product_id: line.product_id,
            quantity: Number(line.quantity),
            stock_bucket: line.stock_bucket
          }))
        })
      });
      setMessage(response.message);
      setLines([newLine()]);
      setRequestNote("");
      setCreateOpen(false);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "สร้างรายการโอนไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
  }

  async function dispatchTransfer(transferId: string) {
    setBusyId(transferId);
    try {
      await proxyClient(`/transfers/${transferId}/dispatch`, {
        method: "POST",
        body: JSON.stringify({ pickup_name: pickupName, courier_name: courierName })
      });
      setMessage("ยืนยันส่งสินค้าออกจากต้นทางแล้ว");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ส่งสินค้าไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
  }

  async function receiveTransfer(transfer: Option) {
    const transferId = String(transfer.id);
    const items = transferItems(transfer);
    setBusyId(transferId);
    try {
      await proxyClient(`/transfers/${transferId}/receive`, {
        method: "POST",
        body: JSON.stringify({
          items: items.map((item) => {
            const draft = receiptValue(transferId, item);
            return {
              item_id: String(item.id),
              received_quantity: Number(draft.quantity),
              discrepancy_note: draft.note
            };
          })
        })
      });
      setMessage(`รับสินค้าใบโอน ${String(transfer.transfer_code)} แล้ว`);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "รับโอนสินค้าไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
  }

  if (mode === "receipt") {
    return (
      <div className="space-y-5">
        {message ? <p className="rounded-2xl border bg-white px-4 py-3 text-sm shadow-card">{message}</p> : null}
        {receivableTransfers.map((transfer) => {
          const transferId = String(transfer.id);
          const items = transferItems(transfer);
          const hasDifference = items.some((item) => {
            const draft = receiptValue(transferId, item);
            return Number(draft.quantity) !== Number(item.quantity);
          });
          const missingNote = items.some((item) => {
            const draft = receiptValue(transferId, item);
            return Number(draft.quantity) !== Number(item.quantity) && !draft.note.trim();
          });

          return (
            <SectionCard
              key={transferId}
              title={`ใบโอน ${String(transfer.transfer_code)}`}
              description={`${String(transfer.source_branch_name)} ส่งมายัง ${String(transfer.destination_branch_name)}`}
            >
              <div className="mb-5 flex flex-wrap items-center gap-3 rounded-2xl bg-surface-warm p-4 text-sm">
                <strong>{String(transfer.source_branch_name)}</strong>
                <ArrowRight className="h-4 w-4 text-primary" />
                <strong>{String(transfer.destination_branch_name)}</strong>
                <Badge className="ml-auto">{items.length.toLocaleString("th-TH")} รายการ</Badge>
              </div>

              <div className="overflow-x-auto rounded-2xl border">
                <table className="w-full min-w-[760px] text-left text-sm">
                  <thead className="bg-muted text-xs text-muted-foreground">
                    <tr>
                      <th className="px-4 py-3">สินค้า</th>
                      <th className="px-4 py-3 text-center">จำนวนที่ส่ง</th>
                      <th className="px-4 py-3">จำนวนที่ได้รับจริง</th>
                      <th className="px-4 py-3">ผลเปรียบเทียบ</th>
                      <th className="px-4 py-3">หมายเหตุส่วนต่าง</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {items.map((item) => {
                      const draft = receiptValue(transferId, item);
                      const sent = Number(item.quantity || 0);
                      const received = Number(draft.quantity || 0);
                      const matches = sent === received;
                      return (
                        <tr key={String(item.id)}>
                          <td className="px-4 py-4">
                            <strong className="block">{String(item.product_name)}</strong>
                            <span className="text-xs text-muted-foreground">{String(item.sku)}</span>
                          </td>
                          <td className="px-4 py-4 text-center text-lg font-black">{sent.toLocaleString("th-TH")}</td>
                          <td className="px-4 py-4">
                            <Input
                              aria-label={`จำนวนที่ได้รับจริง ${String(item.product_name)}`}
                              min="0"
                              onChange={(event) => updateReceipt(transferId, item, { quantity: event.target.value })}
                              type="number"
                              value={draft.quantity}
                            />
                          </td>
                          <td className="px-4 py-4">
                            <Badge className={matches ? "bg-success-50 text-success-800" : "bg-warning-50 text-warning-800"}>
                              {matches ? <CheckCircle2 className="mr-1 h-3 w-3" /> : <TriangleAlert className="mr-1 h-3 w-3" />}
                              {differenceLabel(sent, received)}
                            </Badge>
                          </td>
                          <td className="px-4 py-4">
                            <Input
                              aria-label={`หมายเหตุส่วนต่าง ${String(item.product_name)}`}
                              onChange={(event) => updateReceipt(transferId, item, { note: event.target.value })}
                              placeholder={matches ? "ไม่บังคับ" : "ระบุสาเหตุที่ไม่ตรง"}
                              value={draft.note}
                            />
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>

              <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm text-muted-foreground">
                  {hasDifference ? "ระบบจะบันทึกจำนวนจริงและแจ้งส่วนต่างให้ผู้ดูแลตรวจสอบ" : "จำนวนสินค้าครบตรงตามใบโอน"}
                </p>
                <Button
                  disabled={busyId === transferId || missingNote || items.length === 0}
                  onClick={() => void receiveTransfer(transfer)}
                  type="button"
                >
                  <PackageCheck className="h-4 w-4" />
                  {busyId === transferId ? "กำลังบันทึก..." : "รับสินค้าแล้ว"}
                </Button>
              </div>
            </SectionCard>
          );
        })}
        {receivableTransfers.length === 0 ? (
          <EmptyState className="rounded-3xl border border-dashed bg-white p-12" description="ไม่มีรายการโอนที่รอรับในขณะนี้" />
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {externallyControlled ? null : (
        <div className="flex justify-end">
          <Button onClick={() => setCreateOpen(true)} type="button"><Plus className="h-4 w-4" />สร้างรายการโอนสินค้า</Button>
        </div>
      )}
      <Dialog onOpenChange={setCreateOpen} open={createOpen}>
        <DialogContent className="max-w-5xl">
          <DialogHeader title="สร้างรายการโอนสินค้า" description="กำหนดต้นทาง ปลายทาง และสินค้าที่ต้องการโยกย้าย" />
          <div>
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="สาขาต้นทาง">
            <Select aria-label="สาขาต้นทาง" value={sourceBranchId} onChange={(event) => setSourceBranchId(event.target.value)}>
              <option value="">เลือกสาขาต้นทาง</option>
              {branches.map((branch) => <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>)}
            </Select>
          </Field>
          <Field label="สาขาปลายทาง">
            <Select aria-label="สาขาปลายทาง" value={destinationBranchId} onChange={(event) => setDestinationBranchId(event.target.value)}>
              <option value="">เลือกสาขาปลายทาง</option>
              {branches.map((branch) => <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>)}
            </Select>
          </Field>
        </div>

        <div className="mt-5 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="font-bold">รายการสินค้า</h3>
            <Button onClick={() => setLines((current) => [...current, newLine()])} type="button" variant="secondary">
              <Plus className="h-4 w-4" />เพิ่มสินค้าอีก
            </Button>
          </div>
          {mergeNotice ? (
            <p className="rounded-lg bg-info-50 px-4 py-2 text-sm text-info-800" role="status">
              {mergeNotice}
            </p>
          ) : null}
          {lines.map((line, index) => {
            return (
              <div
                className={cn(
                  "grid gap-3 rounded-2xl bg-muted p-4",
                  canUseGhost ? "md:grid-cols-[minmax(0,1fr)_150px_160px_auto]" : "md:grid-cols-[minmax(0,1fr)_150px_auto]"
                )}
                key={line.key}
              >
                <Field label={`สินค้า ${index + 1}`}>
                  <ProductSearchPicker ariaLabel={`สินค้าที่โอน ${index + 1}`} initialOptions={products} onChange={(productId) => updateLine(line.key, { product_id: productId })} value={line.product_id} />
                </Field>
                <Field label="จำนวนที่ส่ง">
                  <Input aria-label={`จำนวนที่โอน ${index + 1}`} min="1" onChange={(event) => updateLine(line.key, { quantity: event.target.value })} type="number" value={line.quantity} />
                </Field>
                {/* Only the superadmin moves Ghost Stock. For everyone else a
                    transfer is always real stock, so there is nothing to pick. */}
                {canUseGhost ? (
                  <Field label="ประเภทสต๊อก">
                    <Select aria-label={`ประเภทสต๊อกที่โอน ${index + 1}`} value={line.stock_bucket} onChange={(event) => updateLine(line.key, { stock_bucket: event.target.value as "real" | "ghost" })}>
                      <option value="real">สต๊อกจริง</option>
                      <option value="ghost">สต๊อกผี</option>
                    </Select>
                  </Field>
                ) : null}
                <Button aria-label={`ลบสินค้า ${index + 1}`} disabled={lines.length === 1} onClick={() => setLines((current) => current.filter((item) => item.key !== line.key))} type="button" variant="secondary">
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            );
          })}
        </div>

        <div className="mt-5 grid gap-4 md:grid-cols-3">
          <Field label="หมายเหตุการโอน"><Input aria-label="หมายเหตุการโอน" onChange={(event) => setRequestNote(event.target.value)} placeholder="วัตถุประสงค์หรือรายละเอียด" value={requestNote} /></Field>
          <Field label="ชื่อผู้รับสินค้า"><Input aria-label="ชื่อผู้รับสินค้า" onChange={(event) => setPickupName(event.target.value)} placeholder="ไม่บังคับ" value={pickupName} /></Field>
          <Field label="ผู้ขนส่ง"><Input aria-label="ชื่อผู้ขนส่ง" onChange={(event) => setCourierName(event.target.value)} placeholder="ไม่บังคับ" value={courierName} /></Field>
        </div>
        <Button
          className="mt-5"
          disabled={busyId === "create" || !sourceBranchId || !destinationBranchId || lines.some((line) => !line.product_id || Number(line.quantity) <= 0)}
          onClick={() => void createTransfer()}
          type="button"
        >
          <Plus className="h-4 w-4" />{busyId === "create" ? "กำลังสร้าง..." : "สร้างรายการโอน"}
        </Button>
          </div>
        </DialogContent>
      </Dialog>

      <SectionCard title={`รายการรอส่ง (${dispatchableTransfers.length.toLocaleString("th-TH")})`} description="ตรวจสอบรายการก่อนตัดสต๊อกจากต้นทางและส่งให้สาขาปลายทาง">
        <div className="space-y-4">
          {dispatchableTransfers.map((transfer) => (
            <article className="rounded-2xl border bg-white p-4" key={String(transfer.id)}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="font-black">{String(transfer.transfer_code)}</h3>
                  <p className="mt-1 text-sm text-muted-foreground">{String(transfer.source_branch_name)} → {String(transfer.destination_branch_name)}</p>
                </div>
                <Badge>{statusLabel(transfer.status)}</Badge>
              </div>
              <div className="mt-4 divide-y rounded-xl bg-muted px-4">
                {transferItems(transfer).map((item) => (
                  <div className="flex items-center justify-between gap-3 py-3 text-sm" key={String(item.id)}>
                    <span><strong>{String(item.product_name)}</strong> <span className="text-muted-foreground">· {String(item.sku)}</span></span>
                    <span className="font-bold">{Number(item.quantity).toLocaleString("th-TH")} ชิ้น · {String(item.stock_bucket) === "ghost" ? "สต๊อกผี" : canUseGhost ? "สต๊อกจริง" : "สต๊อก"}</span>
                  </div>
                ))}
              </div>
              <Button className="mt-4" disabled={busyId === String(transfer.id)} onClick={() => void dispatchTransfer(String(transfer.id))} type="button" variant="secondary">
                <Send className="h-4 w-4" />{busyId === String(transfer.id) ? "กำลังส่ง..." : "ยืนยันส่งสินค้า"}
              </Button>
            </article>
          ))}
          {dispatchableTransfers.length === 0 ? <EmptyState className="rounded-2xl border border-dashed p-8" description="ไม่มีรายการรอส่ง" /> : null}
        </div>
        {message ? <p className="mt-4 text-sm text-muted-foreground">{message}</p> : null}
      </SectionCard>
    </div>
  );
}
