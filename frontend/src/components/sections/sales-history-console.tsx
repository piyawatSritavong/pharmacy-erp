"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { FileText, RotateCcw, Search, SlidersHorizontal } from "lucide-react";

import { CLOSE_STATUS_LABEL, DataTable, SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { badgeVariants, Button, Dialog, DialogContent, DialogFooter, DialogHeader, EmptyState, Input, LoadingState, Notice, Pagination, Select, Textarea } from "@/components/ui/primitives";
import type { BadgeTone } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

const PAGE_SIZE_DEFAULT = 20;

/** The three states a bill can be in after a month-end close — same wording and
 *  colours as รายงานสรุปสิ้นเดือน, so a bill reads the same on either screen. */
const CLOSE_STATUS: Record<string, { label: string; tone: BadgeTone }> = {
  // Declaration order is the order the filter lists them.
  adjusted: { label: CLOSE_STATUS_LABEL.adjusted, tone: "warning" },
  active: { label: CLOSE_STATUS_LABEL.active, tone: "success" },
  hidden: { label: CLOSE_STATUS_LABEL.hidden, tone: "error" }
};

// Part B, Rule 4 — the POS half of the return workflow: look up the
// original sale, pick the defective line, issue a replacement on the spot.
// The back-office side (send to supplier, resolve Case A/B) lives on
// /claims, gated by returns.manage, not this page.
export type InvoicePage = { items: Option[]; total: number; page: number; page_size: number; branches?: string[] };

function withTaxLabel(items: Option[]) {
  return items.map((item) => ({ ...item, tax_invoice_label: item.tax_invoice_type === "full" ? "เต็มรูป" : "อย่างย่อ" }));
}

export function SalesHistoryConsole({
  initial,
  showFullTimestamp = false,
  isSuperAdmin = false
}: {
  /** First page, read on the server with the same filters as the URL. */
  initial: InvoicePage;
  showFullTimestamp?: boolean;
  /** Superadmin reads this page as an audit trail: before/after bill numbers
   *  and the close status, and no returns to issue from here. */
  isSuperAdmin?: boolean;
}) {
  const [message, setMessage] = useState("");
  // The page-level line is for results; failures inside the return dialog
  // stay in the dialog.
  const [messageTone, setMessageTone] = useState<"success" | "error">("success");
  const [returning, setReturning] = useState(false);
  const [activeInvoice, setActiveInvoice] = useState<Option | null>(null);
  const [items, setItems] = useState<Option[]>([]);
  const [selectedItemId, setSelectedItemId] = useState("");
  const [quantity, setQuantity] = useState(1);
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(false);
  // Filters and paging run on the server: the whole history no longer comes
  // down to the browser. Seeded from the query string, because the dashboard
  // links here with the tile the operator clicked already narrowed down.
  const params = useSearchParams();
  const [search, setSearch] = useState(params.get("search") || "");
  const [debouncedSearch, setDebouncedSearch] = useState(search);
  const [branchFilter, setBranchFilter] = useState(params.get("branch") || "");
  const [paymentFilter, setPaymentFilter] = useState(params.get("payment_status") || "");
  const [paymentMethodFilter, setPaymentMethodFilter] = useState(params.get("payment_method") || "");
  const saleTypeFilter = params.get("sale_type") || "";
  const [closeStatusFilter, setCloseStatusFilter] = useState(params.get("close_status") || "");
  // "adjusted" covers two different outcomes — a bill repriced whole, and one
  // the close also struck lines from. The dashboard links to each separately.
  const removedLinesFilter = params.get("removed_lines") || "";
  // ใบกำกับภาษีอย่างย่อ -> เต็มรูป, same day only.
  const [fullTaxInvoice, setFullTaxInvoice] = useState<Option | null>(null);
  const [fullTaxName, setFullTaxName] = useState("");
  const [fullTaxId, setFullTaxId] = useState("");
  const [fullTaxBusy, setFullTaxBusy] = useState(false);
  const [taxFilter, setTaxFilter] = useState(params.get("tax_invoice_type") || "");
  const [dateFrom, setDateFrom] = useState(params.get("date_from") || "");
  const [dateTo, setDateTo] = useState(params.get("date_to") || "");
  const [page, setPage] = useState(initial.page || 1);
  const [pageSize, setPageSize] = useState(initial.page_size || PAGE_SIZE_DEFAULT);
  const [result, setResult] = useState<InvoicePage>(initial);
  const [listLoading, setListLoading] = useState(false);
  const [listError, setListError] = useState("");
  // On a phone the filters fold away behind one button so the bills come first.
  const [filtersOpen, setFiltersOpen] = useState(false);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search), 300);
    return () => window.clearTimeout(timer);
  }, [search]);

  const query = new URLSearchParams();
  const put = (key: string, value: string) => { if (value) query.set(key, value); };
  put("search", debouncedSearch.trim());
  put("branch", branchFilter);
  put("payment_status", paymentFilter);
  put("payment_method", paymentMethodFilter);
  put("sale_type", saleTypeFilter);
  put("close_status", closeStatusFilter);
  put("removed_lines", removedLinesFilter);
  put("tax_invoice_type", taxFilter);
  put("date_from", dateFrom);
  put("date_to", dateTo);
  const filterKey = query.toString();
  query.set("page", String(page));
  query.set("page_size", String(pageSize));
  const requestKey = query.toString();

  const firstLoad = useRef(true);
  const load = useCallback(async (key: string) => {
    setListLoading(true);
    setListError("");
    try {
      setResult(await proxyClient<InvoicePage>(`/invoices?${key}`));
    } catch (error) {
      setListError(error instanceof Error ? error.message : "โหลดรายการขายไม่สำเร็จ");
    } finally {
      setListLoading(false);
    }
  }, []);

  useEffect(() => {
    // The server already rendered the first page for the URL's filters.
    if (firstLoad.current) {
      firstLoad.current = false;
      return;
    }
    void load(requestKey);
    // Keep the address shareable without a server round trip.
    window.history.replaceState(null, "", filterKey ? `?${filterKey}` : window.location.pathname);
  }, [filterKey, load, requestKey]);

  const refresh = () => void load(requestKey);
  const rows = withTaxLabel(result.items);
  const branchOptions = result.branches || [];
  const showBranch = branchOptions.length > 1;
  const totalPages = Math.max(1, Math.ceil(result.total / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  const activeFilters = [branchFilter, paymentFilter, paymentMethodFilter, closeStatusFilter, taxFilter, dateFrom, dateTo].filter(Boolean).length;

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
    setReturning(true);
    try {
      const result = await proxyClient<{ message: string }>("/product-returns", {
        method: "POST",
        body: JSON.stringify({ invoice_item_id: selectedItemId, quantity, reason })
      });
      setMessage(result.message);
      setMessageTone("success");
      setActiveInvoice(null);
      refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "บันทึกการคืนสินค้าไม่สำเร็จ");
      setMessageTone("error");
    } finally {
      setReturning(false);
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
      setMessageTone("success");
      setFullTaxInvoice(null);
      setFullTaxName("");
      setFullTaxId("");
      refresh();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ออกใบกำกับภาษีเต็มรูปไม่สำเร็จ");
      setMessageTone("error");
    } finally {
      setFullTaxBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      {message && !activeInvoice && !fullTaxInvoice ? <Notice tone={messageTone}>{message}</Notice> : null}
      <SectionCard description="ค้นหาด้วยเลขที่ใบขายหรือชื่อลูกค้า กรองตามสถานะและช่วงวันที่" title="ใบขายย้อนหลัง">
        <div className="mb-3 flex items-end gap-2 sm:hidden">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              aria-label="ค้นหาใบขาย"
              className="pl-9"
              onChange={(event) => resetPage(setSearch)(event.target.value)}
              placeholder="เลขที่ใบขาย หรือชื่อลูกค้า"
              value={search}
            />
          </div>
          <Button aria-expanded={filtersOpen} onClick={() => setFiltersOpen((open) => !open)} type="button" variant="secondary">
            <SlidersHorizontal className="h-4 w-4" />
            ตัวกรอง{activeFilters ? ` (${activeFilters})` : ""}
          </Button>
        </div>
        <div className={cn("mb-4 flex-wrap items-end gap-3 sm:flex", filtersOpen ? "flex" : "hidden")}>
          <Field className="hidden w-60 sm:block" label="ค้นหา">
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
                ? { render: (row: Option) => `${String(row.sale_type) === "credit" ? "ขายเชื่อ · " : ""}${String(row.payment_status) === "paid" ? "ชำระแล้ว" : String(row.payment_status) === "partial" ? "ชำระบางส่วน" : "ค้างชำระ"}` }
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
                        <span className={cn(badgeVariants({ tone: state.tone }), "whitespace-nowrap rounded-full px-2 py-1 font-semibold")}>
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
                className="inline-flex min-h-10 items-center rounded-full border px-3 py-1.5 text-xs font-semibold hover:bg-muted"
                href={`/print/invoices/${String(invoice.id)}`}
                target="_blank"
              >
                เปิดใบเสร็จ
              </Link>
            </div>
          )}
          emptyDescription={listLoading ? "กำลังโหลด..." : "ลองปรับคำค้นหาหรือตัวกรอง"}
          mobileCards
          rows={rows}
        />
        {listError ? <Notice className="mt-3" tone="error">{listError}</Notice> : null}
        <Pagination
          className="mt-4"
          onPageChange={setPage}
          onPageSizeChange={(size) => { setPageSize(size); setPage(1); }}
          page={safePage}
          pageSize={pageSize}
          total={result.total}
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
            <LoadingState compact label="กำลังโหลดรายการสินค้า..." />
          ) : (
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                {items.length === 0 ? <EmptyState className="p-4" description="ไม่พบรายการสินค้าในใบขายนี้" /> : null}
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
              {message ? <Notice tone="error">{message}</Notice> : null}
              <DialogFooter>
                <Button disabled={returning} onClick={() => setActiveInvoice(null)} type="button" variant="secondary">ยกเลิก</Button>
                <Button disabled={!selectedItemId} loading={returning} loadingText="กำลังบันทึก..." type="submit">ออกสินค้าทดแทน</Button>
              </DialogFooter>
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
          {message && messageTone === "error" ? <Notice className="mt-4" tone="error">{message}</Notice> : null}
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
