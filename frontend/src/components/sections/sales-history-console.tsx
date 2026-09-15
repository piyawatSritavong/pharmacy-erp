"use client";

import { FormEvent, startTransition, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { FileText, RotateCcw, Search } from "lucide-react";

import { DataTable, SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Button, Dialog, DialogContent, DialogHeader, Input, Pagination, Select, Textarea } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

const PAGE_SIZE_DEFAULT = 20;

/** The three states a bill can be in after a month-end close — same wording and
 *  colours as รายงานสรุปสิ้นเดือน, so a bill reads the same on either screen. */
const CLOSE_STATUS: Record<string, { label: string; tone: string }> = {
  // Declaration order is the order the filter lists them.
  adjusted: { label: "Adjusted", tone: "bg-amber-100 text-amber-800" },
  active: { label: "Active", tone: "bg-emerald-100 text-emerald-700" },
  hidden: { label: "Hidden/Deleted", tone: "bg-red-100 text-red-700" }
};

// Part B, Rule 4 — the POS half of the return workflow: look up the
// original sale, pick the defective line, issue a replacement on the spot.
// The back-office side (send to supplier, resolve Case A/B) lives on
// /claims, gated by returns.manage, not this page.
export function SalesHistoryConsole({
  initialItems,
  showFullTimestamp = false,
  isSuperAdmin = false
}: {
  initialItems: Option[];
  showFullTimestamp?: boolean;
  /** Superadmin reads this page as an audit trail: before/after bill numbers
   *  and the close status, and no returns to issue from here. */
  isSuperAdmin?: boolean;
}) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [activeInvoice, setActiveInvoice] = useState<Option | null>(null);
  const [items, setItems] = useState<Option[]>([]);
  const [selectedItemId, setSelectedItemId] = useState("");
  const [quantity, setQuantity] = useState(1);
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(false);
  // Filter bar + pager, both client-side: GET /invoices returns the branch's
  // history in one response and takes no page or search params.
  // Seeded from the query string: the dashboard links here with the tile the
  // operator clicked already narrowed down, and a link that landed on an
  // unfiltered list would be an invitation to re-derive it by hand.
  const params = useSearchParams();
  const [search, setSearch] = useState(params.get("search") || "");
  const [branchFilter, setBranchFilter] = useState(params.get("branch") || "");
  const [paymentFilter, setPaymentFilter] = useState(params.get("payment_status") || "");
  const [paymentMethodFilter, setPaymentMethodFilter] = useState(params.get("payment_method") || "");
  const [closeStatusFilter, setCloseStatusFilter] = useState(params.get("close_status") || "");
  // "adjusted" covers two different outcomes — a bill repriced whole, and one
  // the close also struck lines from. The dashboard links to each separately,
  // so the list has to be able to tell them apart.
  const [removedLinesFilter, setRemovedLinesFilter] = useState(params.get("removed_lines") || "");
  // ใบกำกับภาษีอย่างย่อ -> เต็มรูป, same day only.
  const [fullTaxInvoice, setFullTaxInvoice] = useState<Option | null>(null);
  const [fullTaxName, setFullTaxName] = useState("");
  const [fullTaxId, setFullTaxId] = useState("");
  const [fullTaxBusy, setFullTaxBusy] = useState(false);
  const [taxFilter, setTaxFilter] = useState(params.get("tax_invoice_type") || "");
  const [dateFrom, setDateFrom] = useState(params.get("date_from") || "");
  const [dateTo, setDateTo] = useState(params.get("date_to") || "");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_DEFAULT);

  // Branch is only worth filtering when the history actually spans branches —
  // a cashier sees one branch, a back-office user sees them all.
  const branchOptions = useMemo(() => {
    const names = new Set<string>();
    for (const invoice of initialItems) {
      const name = String(invoice.branch_name || "").trim();
      if (name) names.add(name);
    }
    return [...names].sort((a, b) => a.localeCompare(b, "th"));
  }, [initialItems]);
  const showBranch = branchOptions.length > 1;

  const filtered = useMemo(() => {
    const term = search.trim().toLocaleLowerCase("th");
    return initialItems
      .filter((invoice) => {
        if (term && ![invoice.invoice_number, invoice.customer_name].some((value) => String(value || "").toLocaleLowerCase("th").includes(term))) return false;
        if (branchFilter && String(invoice.branch_name) !== branchFilter) return false;
        if (paymentFilter && String(invoice.payment_status) !== paymentFilter) return false;
        if (paymentMethodFilter && String(invoice.payment_method) !== paymentMethodFilter) return false;
        if (removedLinesFilter && String(Boolean(invoice.has_removed_lines)) !== String(removedLinesFilter === "1")) return false;
        // Bills the API says nothing about (anyone but the superadmin) are
        // active by definition — nothing has closed over them.
        if (closeStatusFilter && String(invoice.reconciliation_status || "active") !== closeStatusFilter) return false;
        if (taxFilter && String(invoice.tax_invoice_type) !== taxFilter) return false;
        // issued_at is an ISO timestamp; comparing the date half keeps the
        // range inclusive of the whole "to" day.
        const day = String(invoice.issued_at || "").slice(0, 10);
        if (dateFrom && day < dateFrom) return false;
        if (dateTo && day > dateTo) return false;
        return true;
      })
      // Newest sale first — the bill you just rang up is the one you come here
      // looking for. Bill numbers break ties (also newest first), since a
      // branch's numbering runs forward in time; sorting *by* the number put a
      // June bill above a July one whenever numbering was reset at a close.
      .sort((a, b) => {
        const byDate = String(b.issued_at || "").localeCompare(String(a.issued_at || ""));
        if (byDate !== 0) return byDate;
        return String(b.invoice_number || "").localeCompare(String(a.invoice_number || ""), "th", { numeric: true });
      });
  }, [branchFilter, closeStatusFilter, dateFrom, dateTo, initialItems, paymentFilter, paymentMethodFilter, removedLinesFilter, search, taxFilter]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  const rows = filtered.slice((safePage - 1) * pageSize, safePage * pageSize);

  function resetPage<T>(setter: (value: T) => void) {
    return (value: T) => { setter(value); setPage(1); };
  }

  async function openReturn(invoice: Option) {
    setActiveInvoice(invoice);
    setSelectedItemId("");
    setQuantity(1);
    setReason("");
    setMessage("");
    setLoading(true);
    try {
      const detail = await proxyClient<{ items: Option[] }>(`/invoices/${String(invoice.id)}`);
      setItems(detail.items || []);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "โหลดรายการสินค้าไม่สำเร็จ");
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!selectedItemId) {
      setMessage("กรุณาเลือกสินค้าที่จะคืน");
      return;
    }
    try {
      const result = await proxyClient<{ message: string }>("/product-returns", {
        method: "POST",
        body: JSON.stringify({ invoice_item_id: selectedItemId, quantity, reason })
      });
      setMessage(result.message);
      setActiveInvoice(null);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "บันทึกการคืนสินค้าไม่สำเร็จ");
    }
  }

  const selectedItem = items.find((item) => String(item.id) === selectedItemId);

  // The customer may swap an abbreviated slip for a full tax invoice, but only
  // on the day of the sale — a full tax invoice dated into a day already
  // reported is a different problem. The server enforces this too.
  const soldToday = (invoice: Option) => {
    const sold = new Date(String(invoice.issued_at || "")).toLocaleDateString("en-CA", { timeZone: "Asia/Bangkok" });
    const today = new Date().toLocaleDateString("en-CA", { timeZone: "Asia/Bangkok" });
    return sold === today;
  };
  const canUpgrade = (invoice: Option) =>
    !isSuperAdmin && String(invoice.tax_invoice_type) !== "full" && !invoice.deleted_at && soldToday(invoice);

  async function submitFullTaxInvoice() {
    if (!fullTaxInvoice) return;
    setFullTaxBusy(true);
    try {
      const result = await proxyClient<Option>(`/invoices/${String(fullTaxInvoice.id)}/full-tax-invoice`, {
        method: "POST",
        body: JSON.stringify({ customer_name: fullTaxName, customer_tax_id: fullTaxId })
      });
      setMessage(`${String(result.message || "")} เลขที่ใหม่ ${String(result.invoice_number || "")}`);
      setFullTaxInvoice(null);
      setFullTaxName("");
      setFullTaxId("");
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ออกใบกำกับภาษีเต็มรูปไม่สำเร็จ");
    } finally {
      setFullTaxBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      {message && !activeInvoice ? <p className="rounded-2xl border bg-card px-4 py-3 text-sm shadow-card">{message}</p> : null}
      <SectionCard description="ค้นหาด้วยเลขที่ใบขายหรือชื่อลูกค้า กรองตามสถานะและช่วงวันที่" title="ใบขายย้อนหลัง">
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <Field className="w-60" label="ค้นหา">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาใบขาย"
                className="pl-9"
                onChange={(event) => resetPage(setSearch)(event.target.value)}
                placeholder="เลขที่ใบขาย หรือชื่อลูกค้า"
                value={search}
              />
            </div>
          </Field>
          {showBranch ? (
            <Field className="w-full sm:w-48" label="สาขา">
              <Select aria-label="กรองตามสาขา" onChange={(event) => resetPage(setBranchFilter)(event.target.value)} value={branchFilter}>
                <option value="">ทุกสาขา</option>
                {branchOptions.map((name) => <option key={name} value={name}>{name}</option>)}
              </Select>
            </Field>
          ) : null}
          <Field className="w-40" label="สถานะชำระเงิน">
            <Select aria-label="กรองตามสถานะชำระเงิน" onChange={(event) => resetPage(setPaymentFilter)(event.target.value)} value={paymentFilter}>
              <option value="">ทุกสถานะ</option>
              <option value="paid">ชำระแล้ว</option>
              <option value="unpaid">ค้างชำระ</option>
            </Select>
          </Field>
          <Field className="w-40" label="วิธีชำระเงิน">
            <Select aria-label="กรองตามวิธีชำระเงิน" onChange={(event) => resetPage(setPaymentMethodFilter)(event.target.value)} value={paymentMethodFilter}>
              <option value="">ทุกวิธี</option>
              <option value="cash">เงินสด</option>
              <option value="bank_transfer">เงินโอน</option>
              <option value="mixed">เงินสด + โอน ผสม</option>
            </Select>
          </Field>
          {isSuperAdmin ? (
            <Field className="w-44" label="สถานะ">
              <Select aria-label="กรองตามสถานะบิล" onChange={(event) => resetPage(setCloseStatusFilter)(event.target.value)} value={closeStatusFilter}>
                <option value="">ทุกสถานะ</option>
                {Object.entries(CLOSE_STATUS).map(([value, state]) => (
                  <option key={value} value={value}>{state.label}</option>
                ))}
              </Select>
            </Field>
          ) : null}
          <Field className="w-40" label="ใบกำกับภาษี">
            <Select aria-label="กรองตามชนิดใบกำกับภาษี" onChange={(event) => resetPage(setTaxFilter)(event.target.value)} value={taxFilter}>
              <option value="">ทุกชนิด</option>
              <option value="full">เต็มรูป</option>
              <option value="abbreviated">อย่างย่อ</option>
            </Select>
          </Field>
          <Field className="w-40" label="ตั้งแต่วันที่">
            <Input aria-label="ตั้งแต่วันที่" onChange={(event) => resetPage(setDateFrom)(event.target.value)} type="date" value={dateFrom} />
          </Field>
          <Field className="w-40" label="ถึงวันที่">
            <Input aria-label="ถึงวันที่" onChange={(event) => resetPage(setDateTo)(event.target.value)} type="date" value={dateTo} />
          </Field>
        </div>
        <DataTable
          columns={[
            // Superadmin sees the close's before/after pair; a hidden bill has
            // no current number left, so it reads "—" the way the report does.
            ...(isSuperAdmin
              ? [
                  {
                    key: "original_invoice_number",
                    label: "เลขบิลเดิม",
                    className: "whitespace-nowrap font-medium",
                    render: (row: Option) => String(row.original_invoice_number || row.invoice_number || "—")
                  },
                  {
                    key: "invoice_number",
                    label: "เลขบิลใหม่",
                    className: "whitespace-nowrap font-medium",
                    render: (row: Option) =>
                      String(row.reconciliation_status) === "hidden" ? "—" : String(row.invoice_number || "—")
                  }
                ]
              : [{ key: "invoice_number", label: "เลขที่ใบขาย", className: "whitespace-nowrap font-medium" }]),
            ...(showBranch ? [{ key: "branch_name", label: "สาขา", className: "whitespace-nowrap" }] : []),
            { key: "customer_name", label: "ลูกค้า" },
            {
              key: "payment_status",
              label: isSuperAdmin ? "ชำระเงิน" : "สถานะ",
              className: "whitespace-nowrap",
              ...(isSuperAdmin
                ? { render: (row: Option) => (String(row.payment_status) === "paid" ? "ชำระแล้ว" : "ค้างชำระ") }
                : {})
            },
            ...(isSuperAdmin
              ? [
                  {
                    key: "reconciliation_status",
                    label: "สถานะ",
                    className: "whitespace-nowrap",
                    render: (row: Option) => {
                      const state = CLOSE_STATUS[String(row.reconciliation_status || "active")] || CLOSE_STATUS.active;
                      return (
                        <span className={`whitespace-nowrap rounded-full px-2 py-1 text-xs font-semibold ${state.tone}`}>
                          {state.label}
                        </span>
                      );
                    }
                  }
                ]
              : []),
            { key: "tax_invoice_label", label: "ใบกำกับภาษี", className: "whitespace-nowrap" },
            { key: "total_amount", label: "ยอดรวม", type: "currency", className: "whitespace-nowrap" },
            { key: "issued_at", label: "วันที่ขาย", type: showFullTimestamp ? "datetime" : "date", className: "whitespace-nowrap" }
          ]}
          rowActions={(invoice) => (
            <div className="flex justify-end gap-2 whitespace-nowrap">
              {/* Returns are counter work; the superadmin reads this page as an
                  audit trail, so the action is not offered there. */}
              {isSuperAdmin ? null : (
                <Button onClick={() => void openReturn(invoice)} type="button" variant="secondary">
                  <RotateCcw className="h-4 w-4" />
                  คืน/เปลี่ยนสินค้า
                </Button>
              )}
              {canUpgrade(invoice) ? (
                <Button
                  onClick={() => {
                    setFullTaxInvoice(invoice);
                    setFullTaxName(String(invoice.customer_name || ""));
                    setFullTaxId("");
                  }}
                  type="button"
                  variant="secondary"
                >
                  <FileText className="h-4 w-4" />
                  ขอใบกำกับเต็มรูป
                </Button>
              ) : null}
              <Link
                className="inline-flex items-center rounded-full border px-3 py-1.5 text-xs font-semibold hover:bg-muted"
                href={`/print/invoices/${String(invoice.id)}`}
                target="_blank"
              >
                เปิดใบเสร็จ
              </Link>
            </div>
          )}
          emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
          rows={rows}
        />
        <Pagination
          className="mt-4"
          onPageChange={setPage}
          onPageSizeChange={(size) => { setPageSize(size); setPage(1); }}
          page={safePage}
          pageSize={pageSize}
          total={filtered.length}
          totalPages={totalPages}
        />
      </SectionCard>

      <Dialog onOpenChange={(open) => !open && setActiveInvoice(null)} open={Boolean(activeInvoice)}>
        <DialogContent>
          <DialogHeader
            description="เลือกสินค้าที่ลูกค้านำมาคืน ระบบจะออกสินค้าทดแทนจากสต๊อกปัจจุบันทันทีและบันทึกเข้าคิวเคลม"
            title={`คืน/เปลี่ยนสินค้า — ใบขาย ${String(activeInvoice?.invoice_number || "")}`}
          />
          {loading ? (
            <p className="py-6 text-center text-sm text-muted-foreground">กำลังโหลดรายการสินค้า...</p>
          ) : (
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                {items.length === 0 ? <p className="text-sm text-muted-foreground">ไม่พบรายการสินค้าในใบขายนี้</p> : null}
                {items.map((item) => (
                  <label className="flex cursor-pointer items-center gap-3 rounded-xl border px-3 py-2 text-sm has-[:checked]:border-primary has-[:checked]:bg-primary/5" key={String(item.id)}>
                    <input
                      checked={selectedItemId === String(item.id)}
                      name="invoice_item"
                      onChange={() => { setSelectedItemId(String(item.id)); setQuantity(1); }}
                      type="radio"
                      value={String(item.id)}
                    />
                    <span className="flex-1">
                      <span className="block">{String(item.display_name || item.actual_product_name)}</span>
                      <span className="text-xs text-muted-foreground">ราคาปัจจุบัน {Number(item.unit_price || 0).toLocaleString("th-TH", { minimumFractionDigits: 2 })}{Number(item.discount_amount || 0) > 0 ? ` · ส่วนลดรายการ ${Number(item.discount_amount).toLocaleString("th-TH", { minimumFractionDigits: 2 })}` : ""}</span>
                    </span>
                    <span className="text-muted-foreground">ขาย {String(item.quantity)}{item.stock_bucket ? ` (${String(item.stock_bucket) === "ghost" ? "สต๊อกผี" : "สต๊อกจริง"})` : ""}</span>
                  </label>
                ))}
              </div>
              <Field label="จำนวนที่คืน">
                <Input
                  max={selectedItem ? Number(selectedItem.quantity) : undefined}
                  min={1}
                  onChange={(event) => setQuantity(Number(event.target.value) || 1)}
                  type="number"
                  value={quantity}
                />
              </Field>
              <Field label="เหตุผลการคืน">
                <Textarea onChange={(event) => setReason(event.target.value)} placeholder="เช่น สินค้าชำรุด, เปิดกล่องแล้วใช้งานไม่ได้" value={reason} />
              </Field>
              {message ? <p className="text-sm text-destructive">{message}</p> : null}
              <div className="flex justify-end gap-2">
                <Button onClick={() => setActiveInvoice(null)} type="button" variant="secondary">ยกเลิก</Button>
                <Button disabled={!selectedItemId} type="submit">ออกสินค้าทดแทน</Button>
              </div>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { if (!open) setFullTaxInvoice(null); }} open={Boolean(fullTaxInvoice)}>
        <DialogContent className="max-w-lg">
          <DialogHeader
            description={`ใบ ${String(fullTaxInvoice?.invoice_number || "")} จะถูกยกเลิก และออกใบกำกับภาษีเต็มรูปใบใหม่แทน · ทำได้เฉพาะภายในวันที่ขาย`}
            title="ขอใบกำกับภาษีเต็มรูป"
          />
          <div className="space-y-3">
            <Field label="ชื่อผู้ซื้อ">
              <Input aria-label="ชื่อผู้ซื้อ" onChange={(event) => setFullTaxName(event.target.value)} placeholder="ชื่อบุคคลหรือนิติบุคคล" value={fullTaxName} />
            </Field>
            <Field hint="ต้องระบุ ใบกำกับภาษีเต็มรูปออกโดยไม่มีเลขนี้ไม่ได้" label="เลขประจำตัวผู้เสียภาษี">
              <Input aria-label="เลขประจำตัวผู้เสียภาษี" inputMode="numeric" onChange={(event) => setFullTaxId(event.target.value)} placeholder="13 หลัก" value={fullTaxId} />
            </Field>
          </div>
          <div className="mt-5 flex justify-end gap-2">
            <Button onClick={() => setFullTaxInvoice(null)} type="button" variant="secondary">ยกเลิก</Button>
            <Button disabled={fullTaxBusy || !fullTaxName.trim() || !fullTaxId.trim()} onClick={() => void submitFullTaxInvoice()} type="button">
              {fullTaxBusy ? "กำลังออกใบ..." : "ยกเลิกใบย่อ และออกใบเต็มรูป"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
