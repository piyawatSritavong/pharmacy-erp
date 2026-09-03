"use client";

import { startTransition, useEffect, useMemo, useState } from "react";
import {
  Eye,
  FilePlus2,
  PackagePlus,
  Plus,
  Printer,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { useRouter } from "next/navigation";

import { PurchaseProductPicker } from "@/components/sections/purchase-product-picker";
import { Field } from "@/components/ui/field";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  Input,
  Notice,
  Pagination,
  Select,
  Textarea,
} from "@/components/ui/primitives";
import type { PaginationState } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Item = Record<string, unknown>;
// Lines reference existing catalog products only — brand-new products are
// created solely in รายการสินค้า (business-flow.md); a PO adds stock, never
// catalog entries.
type Line = {
  key: string;
  product_id: string;
  name: string;
  sku: string;
  unit_name: string;
  stock_bucket: "real" | "ghost";
  quantity: number;
  unit_cost: number;
  line_discount: number;
  lot_number: string;
  expires_on: string;
  tracks_expiry: boolean;
  expiry_warning_days: number;
};
type CorrectionDraft = {
  id: string;
  product_id: string;
  product_name: string;
  stock_bucket: "real" | "ghost";
  quantity: number;
  unit_cost: number;
  line_discount: number;
  lot_number: string;
  expires_on: string;
  tracks_expiry: boolean;
  reason: string;
};

const newLine = (): Line => ({
  key: crypto.randomUUID(),
  product_id: "",
  name: "",
  sku: "",
  unit_name: "ชิ้น",
  stock_bucket: "real",
  quantity: 1,
  unit_cost: 0,
  line_discount: 0,
  lot_number: "",
  expires_on: "",
  tracks_expiry: false,
  expiry_warning_days: 30,
});

function bangkokLocalInput() {
  return new Date(Date.now() + 7 * 60 * 60 * 1000).toISOString().slice(0, 16);
}

export function PurchaseOrderConsole({
  orders,
  suppliers,
  branches,
  pagination,
  supplierCursor: initialSupplierCursor = "",
  supplierHasMore: initialSupplierHasMore = false,
  canUseGhost = false,
}: {
  orders: Item[];
  suppliers: Item[];
  branches: Item[];
  pagination?: PaginationState;
  supplierCursor?: string;
  supplierHasMore?: boolean;
  canUseGhost?: boolean;
}) {
  const router = useRouter();

  function goToPage(page: number, pageSize = pagination?.page_size) {
    const params = new URLSearchParams();
    if (page > 1) params.set("page", String(page));
    if (pageSize) params.set("page_size", String(pageSize));
    router.push(`/purchase-orders?${params.toString()}`);
  }
  const [query, setQuery] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<Item | null>(null);
  const [correctionOpen, setCorrectionOpen] = useState(false);
  const [correction, setCorrection] = useState<CorrectionDraft | null>(null);
  const [headerEditOpen, setHeaderEditOpen] = useState(false);
  const [headerEdit, setHeaderEdit] = useState({
    vat_mode: "exclusive",
    vat_rate: 7,
    header_discount: 0,
    shipping_amount: 0,
    notes: "",
    reason: "",
  });
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [message, setMessage] = useState("");
  const [branchId, setBranchId] = useState(String(branches[0]?.id || ""));
  const selectedBranch = branches.find((branch) => String(branch.id) === branchId);
  const canUseGhostAtBranch = canUseGhost && String(selectedBranch?.branch_type) === "main_warehouse";
  const [supplierId, setSupplierId] = useState(String(suppliers[0]?.id || ""));
  const [supplierOptions, setSupplierOptions] = useState(suppliers);
  const [supplierCursor, setSupplierCursor] = useState(initialSupplierCursor);
  const [supplierHasMore, setSupplierHasMore] = useState(
    initialSupplierHasMore,
  );
  const [supplierLoading, setSupplierLoading] = useState(false);
  const [purchasedAt, setPurchasedAt] = useState(bangkokLocalInput());
  const [dueDate, setDueDate] = useState("");
  const [jobName, setJobName] = useState("");
  const [deliveryTerms, setDeliveryTerms] = useState("");
  const [supplierDocument, setSupplierDocument] = useState("");
  const [vatMode, setVATMode] = useState("exclusive");
  const [vatRate, setVATRate] = useState(7);
  const [headerDiscount, setHeaderDiscount] = useState(0);
  const [shipping, setShipping] = useState(0);
  const [notes, setNotes] = useState("");
  const [pickerBucket, setPickerBucket] = useState<"real" | "ghost">("real");
  const [lines, setLines] = useState<Line[]>([]);
  const [saving, setSaving] = useState(false);
  // Brief inline notice shown when a picked product merged into an existing
  // line instead of creating a duplicate row.
  const [mergeNotice, setMergeNotice] = useState("");
  useEffect(() => {
    if (!mergeNotice) return;
    const timer = window.setTimeout(() => setMergeNotice(""), 5000);
    return () => window.clearTimeout(timer);
  }, [mergeNotice]);

  // Changing a line's stock type can also collide with an existing line for
  // the same product — merge there too instead of leaving two identical rows.
  function changeLineBucket(key: string, bucket: "real" | "ghost") {
    setLines((current) => {
      const moving = current.find((line) => line.key === key);
      if (!moving) return current;
      const target = current.find(
        (line) => line.key !== key && line.product_id === moving.product_id && line.stock_bucket === bucket,
      );
      if (target && moving.product_id) {
        setMergeNotice(`รวมจำนวน “${moving.name}” เข้ากับรายการเดิมแล้ว (สินค้าและประเภทสต๊อกเดียวกัน)`);
        return current
          .filter((line) => line.key !== key)
          .map((line) =>
            line.key === target.key ? { ...line, quantity: line.quantity + moving.quantity } : line,
          );
      }
      return current.map((line) => (line.key === key ? { ...line, stock_bucket: bucket } : line));
    });
  }

  const visibleOrders = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return keyword
      ? orders.filter((item) =>
          [
            item.po_number,
            item.supplier_name,
            item.branch_name,
            item.supplier_document_number,
          ].some((value) =>
            String(value || "")
              .toLowerCase()
              .includes(keyword),
          ),
        )
      : orders;
  }, [orders, query]);
  const totals = useMemo(() => {
    const subtotal = lines.reduce(
      (sum, line) => sum + line.unit_cost * line.quantity,
      0,
    );
    const lineDiscount = lines.reduce(
      (sum, line) => sum + line.line_discount,
      0,
    );
    const taxable = Math.max(
      0,
      subtotal - lineDiscount - headerDiscount + shipping,
    );
    const tax =
      vatMode === "exclusive"
        ? (taxable * vatRate) / 100
        : vatMode === "inclusive"
          ? (taxable * vatRate) / (100 + vatRate)
          : 0;
    return {
      subtotal,
      lineDiscount,
      tax,
      total: vatMode === "exclusive" ? taxable + tax : taxable,
    };
  }, [lines, headerDiscount, shipping, vatMode, vatRate]);

  function resetForm() {
    setBranchId(String(branches[0]?.id || ""));
    setSupplierId(String(suppliers[0]?.id || ""));
    setPurchasedAt(bangkokLocalInput());
    setDueDate("");
    setJobName("");
    setDeliveryTerms("");
    setSupplierDocument("");
    setVATMode("exclusive");
    setVATRate(7);
    setHeaderDiscount(0);
    setShipping(0);
    setNotes("");
    setLines([]);
    setPickerBucket("real");
    setMessage("");
  }

  function selectReceivingBranch(nextBranchId: string) {
    const nextBranch = branches.find((branch) => String(branch.id) === nextBranchId);
    if (String(nextBranch?.branch_type) !== "main_warehouse") {
      setPickerBucket("real");
      if (lines.some((line) => line.stock_bucket === "ghost")) {
        setLines([]);
        setMessage("เปลี่ยนเป็นสาขาขายแล้ว ระบบล้างรายการ Ghost เดิม กรุณาเลือกสินค้า Real ใหม่");
      }
    }
    setBranchId(nextBranchId);
  }
  function updateLine(key: string, patch: Partial<Line>) {
    setLines((current) =>
      current.map((line) => (line.key === key ? { ...line, ...patch } : line)),
    );
  }
  function chooseProduct(product: Item) {
    // Same product + same stock type = one line: merge quantities instead of
    // adding a duplicate row (business-flow.md, duplicate line-item rule).
    setLines((current) => {
      const existing = current.find(
        (line) =>
          line.product_id === String(product.id) &&
          line.stock_bucket === pickerBucket,
      );
      if (existing) {
        setMergeNotice(
          `รวมจำนวน “${String(product.name)}” เข้ากับรายการเดิมแล้ว (สินค้าและประเภทสต๊อกเดียวกัน)`,
        );
        return current.map((line) =>
          line.key === existing.key
            ? { ...line, quantity: line.quantity + 1 }
            : line,
        );
      }
      return [
        ...current,
        {
          ...newLine(),
          product_id: String(product.id),
          name: String(product.name),
          sku: String(product.sku),
          unit_name: String(product.unit_name || "ชิ้น"),
          stock_bucket: pickerBucket,
          unit_cost: Number(product.cost_price || 0),
          tracks_expiry: Boolean(product.tracks_expiry),
          expiry_warning_days: Number(product.expiry_warning_days || 30),
        },
      ];
    });
  }

  async function save() {
    if (!branchId || !supplierId || !lines.length) {
      setMessage(
        "กรุณาเลือกสาขา บริษัทคู่ค้า และเพิ่มสินค้าอย่างน้อย 1 รายการ",
      );
      return;
    }
    setSaving(true);
    setMessage("");
    const supplier = supplierOptions.find(
      (item) => String(item.id) === supplierId,
    );
    const supplierAddress = supplier
      ? [
          supplier.address_line,
          supplier.subdistrict,
          supplier.district,
          supplier.province,
          supplier.postal_code,
        ]
          .filter(Boolean)
          .join(" ")
      : "";
    try {
      const response = await proxyClient<{ id: string; message: string }>(
        "/purchase-orders",
        {
          method: "POST",
          body: JSON.stringify({
            branch_id: branchId,
            supplier_id: supplierId,
            purchased_at: new Date(`${purchasedAt}:00+07:00`).toISOString(),
            due_date: dueDate,
            job_name: jobName,
            delivery_terms: deliveryTerms,
            supplier_document_number: supplierDocument,
            supplier_address_snapshot: supplierAddress,
            vat_mode: vatMode,
            vat_rate: vatRate,
            header_discount: headerDiscount,
            shipping_amount: shipping,
            notes,
            items: lines.map((line) => ({
              product_id: line.product_id,
              stock_bucket: line.stock_bucket,
              quantity: line.quantity,
              unit_cost: line.unit_cost,
              line_discount: line.line_discount,
              lot_number: line.lot_number,
              expires_on: line.expires_on,
            })),
          }),
        },
      );
      setCreateOpen(false);
      resetForm();
      startTransition(() => router.refresh());
      await openDetail(response.id);
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "บันทึกใบสั่งซื้อไม่สำเร็จ",
      );
    } finally {
      setSaving(false);
    }
  }

  async function openDetail(id: string) {
    setLoadingDetail(true);
    setDetailOpen(true);
    try {
      setDetail(await proxyClient<Item>(`/purchase-orders/${id}`));
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "โหลดใบสั่งซื้อไม่สำเร็จ",
      );
    } finally {
      setLoadingDetail(false);
    }
  }
  async function loadMoreSuppliers() {
    if (!supplierHasMore || supplierLoading) return;
    setSupplierLoading(true);
    try {
      const response = await proxyClient<{
        items: Item[];
        next_cursor?: string;
        has_more: boolean;
      }>(
        `/suppliers?active=true&limit=20&cursor=${encodeURIComponent(supplierCursor)}`,
      );
      setSupplierOptions((current) => [
        ...current,
        ...response.items.filter(
          (next) =>
            !current.some((item) => String(item.id) === String(next.id)),
        ),
      ]);
      setSupplierCursor(response.next_cursor || "");
      setSupplierHasMore(response.has_more);
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "โหลดบริษัทคู่ค้าเพิ่มไม่สำเร็จ",
      );
    } finally {
      setSupplierLoading(false);
    }
  }
  async function cancelOrder() {
    if (!detail) return;
    const reason = window.prompt("เหตุผลการยกเลิกใบสั่งซื้อ");
    if (!reason) return;
    try {
      await proxyClient(`/purchase-orders/${String(detail.id)}/cancel`, {
        method: "POST",
        body: JSON.stringify({ reason }),
      });
      setDetailOpen(false);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ยกเลิกไม่สำเร็จ");
    }
  }
  function openCorrection(item: Item) {
    setCorrection({
      id: String(item.id),
      product_id: String(item.product_id),
      product_name: String(item.product_name),
      stock_bucket: item.stock_bucket === "ghost" ? "ghost" : "real",
      quantity: Number(item.received_quantity),
      unit_cost: Number(item.unit_cost),
      line_discount: Number(item.line_discount),
      lot_number: String(item.lot_number || ""),
      expires_on: item.expires_on ? String(item.expires_on).slice(0, 10) : "",
      tracks_expiry: Boolean(item.expires_on),
      reason: "",
    });
    setCorrectionOpen(true);
  }
  async function saveCorrection() {
    if (!detail || !correction || !correction.reason.trim()) {
      setMessage("กรุณาระบุเหตุผลการแก้ไข");
      return;
    }
    try {
      await proxyClient(`/purchase-orders/${String(detail.id)}`, {
        method: "PUT",
        body: JSON.stringify({
          vat_mode: detail.vat_mode,
          vat_rate: detail.vat_rate,
          header_discount: detail.header_discount,
          shipping_amount: detail.shipping_amount,
          notes: detail.notes,
          correction_reason: correction.reason,
          items: [
            {
              id: correction.id,
              product_id: correction.product_id,
              stock_bucket: correction.stock_bucket,
              quantity: correction.quantity,
              unit_cost: correction.unit_cost,
              line_discount: correction.line_discount,
              lot_number: correction.lot_number,
              expires_on: correction.expires_on,
            },
          ],
        }),
      });
      setCorrectionOpen(false);
      await openDetail(String(detail.id));
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "แก้ไขรายการไม่สำเร็จ",
      );
    }
  }
  function openHeaderEdit() {
    if (!detail) return;
    setHeaderEdit({
      vat_mode: String(detail.vat_mode || "exclusive"),
      vat_rate: Number(detail.vat_rate || 0),
      header_discount: Number(detail.header_discount || 0),
      shipping_amount: Number(detail.shipping_amount || 0),
      notes: String(detail.notes || ""),
      reason: "",
    });
    setHeaderEditOpen(true);
  }
  async function saveHeaderEdit() {
    if (!detail || !headerEdit.reason.trim()) {
      setMessage("กรุณาระบุเหตุผลการแก้ไข");
      return;
    }
    try {
      await proxyClient(`/purchase-orders/${String(detail.id)}`, {
        method: "PUT",
        body: JSON.stringify({
          ...headerEdit,
          correction_reason: headerEdit.reason,
          items: [],
        }),
      });
      setHeaderEditOpen(false);
      await openDetail(String(detail.id));
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "แก้ไขยอดเอกสารไม่สำเร็จ",
      );
    }
  }

  return (
    <>
      {/* print:hidden — same reasoning as the page header: the exported PDF
          from the detail dialog below should be the formal PO document
          alone, not this list/search view underneath it. */}
      <section className="overflow-hidden rounded-3xl border bg-white shadow-card print:hidden">
        <div className="flex flex-col gap-3 border-b bg-surface-warm p-5 md:flex-row md:items-center md:justify-between">
          <div className="relative min-w-0 flex-1 md:max-w-xl">
            <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              aria-label="ค้นหาใบสั่งซื้อ"
              className="pl-9"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="เลข PO บริษัทคู่ค้า สาขา หรือเลขเอกสารคู่ค้า"
              value={query}
            />
          </div>
          <Button
            onClick={() => {
              resetForm();
              setCreateOpen(true);
            }}
            type="button"
          >
            <FilePlus2 className="h-4 w-4" />
            สร้างใบสั่งซื้อเข้า
          </Button>
        </div>
        {message ? (
          <Notice className="rounded-none border-b" tone="error">
            {message}
          </Notice>
        ) : null}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] text-sm">
            <thead className="bg-muted text-left">
              <tr>
                <th className="p-3">เลข PO</th>
                <th className="p-3">วันเวลาซื้อ</th>
                <th className="p-3">บริษัทคู่ค้า</th>
                <th className="p-3">สาขารับ</th>
                <th className="p-3">รายการ/จำนวน</th>
                <th className="p-3">ยอดสุทธิ</th>
                <th className="p-3">สถานะ</th>
                <th className="p-3"></th>
              </tr>
            </thead>
            <tbody>
              {visibleOrders.map((item) => (
                <tr className="border-t" key={String(item.id)}>
                  <td className="p-3 font-bold">{String(item.po_number)}</td>
                  <td className="p-3">
                    {new Date(String(item.purchased_at)).toLocaleString("th-TH", { timeZone: "Asia/Bangkok" })}
                  </td>
                  <td className="p-3">
                    {String(item.supplier_name)}
                    <span className="block text-xs text-muted-foreground">
                      {String(item.supplier_document_number || "")}
                    </span>
                  </td>
                  <td className="p-3">{String(item.branch_name)}</td>
                  <td className="p-3">
                    {Number(item.item_count)} รายการ ·{" "}
                    {Number(item.total_quantity)} ชิ้น
                  </td>
                  <td className="p-3 font-bold">
                    {currency(Number(item.total_amount))}
                  </td>
                  <td className="p-3">
                    {item.status === "posted" ? "รับเข้าสต๊อกแล้ว" : "ยกเลิก"}
                  </td>
                  <td className="p-3 text-right">
                    <Button
                      className="h-9 px-3"
                      onClick={() => openDetail(String(item.id))}
                      type="button"
                      variant="secondary"
                    >
                      <Eye className="h-4 w-4" />
                      ดู
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {visibleOrders.length === 0 ? <EmptyState className="p-12" description="ยังไม่มีใบสั่งซื้อเข้า" /> : null}
        {pagination ? (
          <Pagination
            className="border-t p-4 print:hidden"
            onPageChange={(page) => goToPage(page)}
            onPageSizeChange={(pageSize) => goToPage(1, pageSize)}
            page={pagination.page}
            pageSize={pagination.page_size}
            total={pagination.total}
            totalPages={pagination.total_pages}
          />
        ) : null}
      </section>

      <Dialog onOpenChange={setCreateOpen} open={createOpen}>
        <DialogContent className="max-w-6xl">
          <DialogHeader
            description="เมื่อบันทึก ระบบจะสร้าง lot และเพิ่มสินค้าเข้าสต๊อกทันที"
            title="สร้างใบสั่งซื้อเข้า"
          />
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <Field label="สาขารับสินค้า *">
              <Select
                onChange={(e) => selectReceivingBranch(e.target.value)}
                value={branchId}
              >
                {branches.map((branch) => (
                  <option key={String(branch.id)} value={String(branch.id)}>
                    {String(branch.name)}
                  </option>
                ))}
              </Select>
            </Field>
            <div>
              <Field label="บริษัทคู่ค้า *">
                <Select
                  onChange={(e) => setSupplierId(e.target.value)}
                  value={supplierId}
                >
                  <option value="">เลือกบริษัท</option>
                  {supplierOptions
                    .filter((item) => item.active)
                    .map((item) => (
                      <option key={String(item.id)} value={String(item.id)}>
                        {String(item.legal_name)}
                      </option>
                    ))}
                </Select>
              </Field>
              {supplierHasMore ? (
                <button
                  className="mt-1 text-xs font-bold text-primary hover:underline"
                  disabled={supplierLoading}
                  onClick={loadMoreSuppliers}
                  type="button"
                >
                  {supplierLoading
                    ? "กำลังโหลด..."
                    : "โหลดบริษัทคู่ค้าเพิ่ม 20 รายการ"}
                </button>
              ) : null}
            </div>
            <Field label="วันเวลาซื้อ">
              <Input
                onChange={(e) => setPurchasedAt(e.target.value)}
                type="datetime-local"
                value={purchasedAt}
              />
            </Field>
            <Field label="วันครบกำหนด" hint="สำหรับพิมพ์ใบสั่งซื้อ">
              <Input
                onChange={(e) => setDueDate(e.target.value)}
                type="date"
                value={dueDate}
              />
            </Field>
            <Field label="เลขเอกสารคู่ค้า">
              <Input
                onChange={(e) => setSupplierDocument(e.target.value)}
                value={supplierDocument}
              />
            </Field>
            <Field label="ชื่องาน" hint="สำหรับพิมพ์ใบสั่งซื้อ">
              <Input
                onChange={(e) => setJobName(e.target.value)}
                placeholder="เช่น สั่งซื้อแก้วกาแฟและหลอดกาแฟ"
                value={jobName}
              />
            </Field>
            <Field className="md:col-span-2 xl:col-span-4" label="เงื่อนไขการจัดส่ง" hint="สำหรับพิมพ์ใบสั่งซื้อ">
              <Textarea
                onChange={(e) => setDeliveryTerms(e.target.value)}
                placeholder="กำหนดระยะเวลา วิธีและสถานที่ในการจัดส่ง เงื่อนไขการส่งมอบสินค้า"
                value={deliveryTerms}
              />
            </Field>
          </div>
          {/* Search on top, the lines you have added underneath — stacked so the
              list runs the full width of the dialog instead of being squeezed
              into a column beside the picker. */}
          <div className="mt-5 space-y-5">
            <div>
              {canUseGhostAtBranch ? (
                <div className="mb-3 flex gap-2">
                  <Select
                    aria-label="ประเภทสต๊อกสำหรับสินค้าที่เพิ่ม"
                    onChange={(e) =>
                      setPickerBucket(e.target.value as "real" | "ghost")
                    }
                    value={pickerBucket}
                  >
                    <option value="real">สต๊อกจริง</option>
                    <option value="ghost">สต๊อกผี</option>
                  </Select>
                </div>
              ) : null}
              <PurchaseProductPicker
                branchId={branchId}
                onChoose={chooseProduct}
                stockBucket={pickerBucket}
              />
            </div>
            <div className="space-y-3">
              <h3 className="font-bold">รายการสินค้า ({lines.length})</h3>
              {mergeNotice ? (
                <p className="rounded-xl bg-info-50 px-4 py-2.5 text-sm text-info-800" role="status">
                  {mergeNotice}
                </p>
              ) : null}
              {lines.map((line, index) => (
                <div
                  className="rounded-2xl border bg-surface-warm p-4"
                  key={line.key}
                >
                  <div className="mb-3 flex items-center justify-between">
                    <strong>
                      {index + 1}. {line.name}
                    </strong>
                    <Button
                      className="h-8 px-2"
                      onClick={() =>
                        setLines((current) =>
                          current.filter((item) => item.key !== line.key),
                        )
                      }
                      type="button"
                      variant="ghost"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                  <div className={`grid gap-3 ${canUseGhostAtBranch ? "md:grid-cols-4" : "md:grid-cols-3"}`}>
                    {canUseGhostAtBranch ? (
                      <Field label="ประเภทสต๊อก">
                        <Select
                          onChange={(e) =>
                            changeLineBucket(line.key, e.target.value as "real" | "ghost")
                          }
                          value={line.stock_bucket}
                        >
                          <option value="real">จริง</option>
                          <option value="ghost">สต๊อกผี</option>
                        </Select>
                      </Field>
                    ) : null}
                    <Field label="จำนวน">
                      <Input
                        min={1}
                        onChange={(e) =>
                          updateLine(line.key, {
                            quantity: Number(e.target.value),
                          })
                        }
                        type="number"
                        value={line.quantity}
                      />
                    </Field>
                    <Field label="ราคาซื้อ/หน่วย">
                      <Input
                        min={0}
                        onChange={(e) =>
                          updateLine(line.key, {
                            unit_cost: Number(e.target.value),
                          })
                        }
                        step="0.01"
                        type="number"
                        value={line.unit_cost}
                      />
                    </Field>
                    <Field label="ส่วนลดรายการ">
                      <Input
                        min={0}
                        onChange={(e) =>
                          updateLine(line.key, {
                            line_discount: Number(e.target.value),
                          })
                        }
                        step="0.01"
                        type="number"
                        value={line.line_discount}
                      />
                    </Field>
                    <Field label="Lot/Batch">
                      <Input
                        onChange={(e) =>
                          updateLine(line.key, { lot_number: e.target.value })
                        }
                        placeholder="เว้นว่างเพื่อสร้างอัตโนมัติ"
                        value={line.lot_number}
                      />
                    </Field>
                    <Field
                      label={`วันหมดอายุ${line.tracks_expiry ? " *" : ""}`}
                    >
                      <Input
                        required={line.tracks_expiry}
                        onChange={(e) =>
                          updateLine(line.key, { expires_on: e.target.value })
                        }
                        type="date"
                        value={line.expires_on}
                      />
                    </Field>
                    <p className="self-end pb-2 text-right font-bold md:col-span-2">
                      ยอดรายการ{" "}
                      {currency(
                        line.unit_cost * line.quantity - line.line_discount,
                      )}
                    </p>
                  </div>
                </div>
              ))}
              {lines.length === 0 ? (
                <div className="rounded-2xl border border-dashed p-10 text-center text-muted-foreground">
                  <PackagePlus className="mx-auto mb-2 h-6 w-6" />
                  เลือกสินค้าจากรายการด้านซ้าย — สินค้าใหม่สร้างได้ที่เมนู รายการสินค้า
                </div>
              ) : null}
            </div>
          </div>
          <div className="mt-5 grid gap-4 border-t pt-5 md:grid-cols-[minmax(0,1fr)_380px]">
            <div>
              <Field label="หมายเหตุ">
                <Textarea
                  onChange={(e) => setNotes(e.target.value)}
                  value={notes}
                />
              </Field>
            </div>
            <div className="space-y-3 rounded-2xl bg-foreground p-4 text-white">
              <div className="grid grid-cols-2 gap-2">
                <Field label="VAT" labelClassName="text-white">
                  <Select
                    onChange={(e) => setVATMode(e.target.value)}
                    value={vatMode}
                  >
                    <option value="none">ไม่คิด VAT</option>
                    <option value="exclusive">ราคาไม่รวม VAT</option>
                    <option value="inclusive">ราคารวม VAT</option>
                  </Select>
                </Field>
                <Field label="อัตรา %" labelClassName="text-white">
                  <Input
                    disabled={vatMode === "none"}
                    min={0}
                    onChange={(e) => setVATRate(Number(e.target.value))}
                    type="number"
                    value={vatRate}
                  />
                </Field>
                <Field label="ส่วนลดท้ายบิล" labelClassName="text-white">
                  <Input
                    min={0}
                    onChange={(e) => setHeaderDiscount(Number(e.target.value))}
                    type="number"
                    value={headerDiscount}
                  />
                </Field>
                <Field label="ค่าขนส่ง" labelClassName="text-white">
                  <Input
                    min={0}
                    onChange={(e) => setShipping(Number(e.target.value))}
                    type="number"
                    value={shipping}
                  />
                </Field>
              </div>
              <div className="space-y-1 border-t border-white/20 pt-3 text-sm">
                <p className="flex justify-between">
                  <span>สินค้า</span>
                  <span>{currency(totals.subtotal)}</span>
                </p>
                <p className="flex justify-between">
                  <span>ส่วนลดรายการ</span>
                  <span>-{currency(totals.lineDiscount)}</span>
                </p>
                <p className="flex justify-between">
                  <span>VAT</span>
                  <span>{currency(totals.tax)}</span>
                </p>
                <p className="flex justify-between text-lg font-bold">
                  <span>ยอดสุทธิ</span>
                  <span>{currency(totals.total)}</span>
                </p>
              </div>
            </div>
          </div>
          {message ? (
            <p className="mt-4 text-sm text-primary">{message}</p>
          ) : null}
          <div className="mt-5 flex justify-end gap-2">
            <Button
              onClick={() => setCreateOpen(false)}
              type="button"
              variant="secondary"
            >
              ยกเลิก
            </Button>
            <Button disabled={saving} onClick={save} type="button">
              {saving ? "กำลังบันทึก..." : "บันทึกและรับเข้าสต๊อก"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setDetailOpen} open={detailOpen}>
        <DialogContent className="max-w-5xl print:static print:max-h-none print:max-w-none print:translate-x-0 print:translate-y-0 print:border-0 print:shadow-none">
          <div className="print:hidden">
            <DialogHeader
              description={
                detail
                  ? `${String(detail.supplier_name)} · ${String(detail.branch_name)}`
                  : ""
              }
              title={detail ? String(detail.po_number) : "ใบสั่งซื้อเข้า"}
            />
          </div>
          {loadingDetail ? (
            <p className="p-10 text-center">กำลังโหลด...</p>
          ) : detail ? (
            <div className="space-y-5">
              {/* D4: formal document matching PO-Form.jpg — screen-hidden,
                  this is what actually renders when printed / exported as
                  PDF. The operational lot/correction view below stays
                  on-screen only (print:hidden) since it's a working tool,
                  not part of the business document. */}
              <PurchaseOrderPrintDocument detail={detail} />

              <div className="print:hidden">
              <div className="grid gap-3 rounded-2xl bg-surface-warm p-4 text-sm md:grid-cols-3">
                <p>
                  <strong>วันซื้อ</strong>
                  <span className="block">
                    {new Date(String(detail.purchased_at)).toLocaleString("th-TH", { timeZone: "Asia/Bangkok" })}
                  </span>
                </p>
                <p>
                  <strong>เอกสารคู่ค้า</strong>
                  <span className="block">
                    {String(detail.supplier_document_number || "-")}
                  </span>
                </p>
                <p>
                  <strong>สถานะ</strong>
                  <span className="block">
                    {detail.status === "posted" ? "รับเข้าสต๊อกแล้ว" : "ยกเลิก"}{" "}
                    · revision {Number(detail.revision)}
                  </span>
                </p>
                <p className="md:col-span-3">
                  <strong>ที่อยู่คู่ค้า</strong>
                  <span className="block">
                    {String(detail.supplier_address || "-")}
                  </span>
                </p>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full min-w-[900px] text-sm">
                  <thead className="bg-muted text-left">
                    <tr>
                      <th className="p-3">สินค้า</th>
                      {canUseGhost ? <th className="p-3">Bucket</th> : null}
                      <th className="p-3">รับเข้า</th>
                      <th className="p-3">คงเหลือ lot</th>
                      <th className="p-3">ต้นทุน</th>
                      <th className="p-3">Lot</th>
                      <th className="p-3">หมดอายุ</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {((detail.items as Item[]) || []).map((item) => (
                      <tr className="border-t" key={String(item.id)}>
                        <td className="p-3">
                          <strong>{String(item.product_name)}</strong>
                          <span className="block text-xs text-muted-foreground">
                            {String(item.sku)}
                          </span>
                        </td>
                        {canUseGhost ? (
                          <td className="p-3">
                            {item.stock_bucket === "ghost" ? "ผี" : "จริง"}
                          </td>
                        ) : null}
                        <td className="p-3">
                          {Number(item.received_quantity)}
                        </td>
                        <td className="p-3 font-bold">
                          {Number(item.remaining_quantity)}
                        </td>
                        <td className="p-3">
                          {currency(Number(item.unit_cost))}
                        </td>
                        <td className="p-3">{String(item.lot_number)}</td>
                        <td className="p-3">
                          {item.expires_on
                            ? new Date(
                                String(item.expires_on),
                              ).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })
                            : "-"}
                        </td>
                        <td className="p-3">
                          {detail.status === "posted" ? (
                            <Button
                              className="h-8 px-2 print:hidden"
                              onClick={() => openCorrection(item)}
                              type="button"
                              variant="ghost"
                            >
                              <RotateCcw className="h-4 w-4" />
                              แก้รายการ
                            </Button>
                          ) : null}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="ml-auto max-w-sm space-y-1 text-sm">
                <p className="flex justify-between">
                  <span>Subtotal</span>
                  <span>{currency(Number(detail.subtotal))}</span>
                </p>
                <p className="flex justify-between">
                  <span>ส่วนลด</span>
                  <span>
                    -
                    {currency(
                      Number(detail.line_discount_total) +
                        Number(detail.header_discount),
                    )}
                  </span>
                </p>
                <p className="flex justify-between">
                  <span>ค่าขนส่ง</span>
                  <span>{currency(Number(detail.shipping_amount))}</span>
                </p>
                <p className="flex justify-between">
                  <span>VAT</span>
                  <span>{currency(Number(detail.tax_amount))}</span>
                </p>
                <p className="flex justify-between border-t pt-2 text-lg font-bold">
                  <span>ยอดสุทธิ</span>
                  <span>{currency(Number(detail.total_amount))}</span>
                </p>
              </div>
              <div className={canUseGhost ? "print:hidden" : "hidden"}>
                <h3 className="mb-2 font-bold">ประวัติเอกสาร</h3>
                <div className="space-y-2">
                  {((detail.events as Item[]) || []).map((event) => (
                    <p
                      className="rounded-xl bg-muted px-3 py-2 text-sm"
                      key={String(event.id)}
                    >
                      <strong>{String(event.event_type)}</strong> · revision{" "}
                      {Number(event.revision)} · {String(event.actor_name)}
                      <span className="block text-xs text-muted-foreground">
                        {new Date(String(event.event_at)).toLocaleString("th-TH", { timeZone: "Asia/Bangkok" })}{" "}
                        {event.note ? `· ${String(event.note)}` : ""}
                      </span>
                    </p>
                  ))}
                </div>
              </div>
              <div className="flex justify-end gap-2 print:hidden">
                {detail.status === "posted" ? (
                  <Button
                    onClick={openHeaderEdit}
                    type="button"
                    variant="secondary"
                  >
                    <RotateCcw className="h-4 w-4" />
                    แก้ยอด/หมายเหตุ
                  </Button>
                ) : null}
                <Button
                  onClick={() => window.print()}
                  type="button"
                  variant="secondary"
                >
                  <Printer className="h-4 w-4" />
                  พิมพ์
                </Button>
                {detail.status === "posted" ? (
                  <Button
                    onClick={cancelOrder}
                    type="button"
                    variant="destructive"
                  >
                    <Trash2 className="h-4 w-4" />
                    ยกเลิก PO
                  </Button>
                ) : null}
              </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setCorrectionOpen} open={correctionOpen}>
        <DialogContent className="max-w-4xl">
          <DialogHeader
            description="เปลี่ยนสินค้า ประเภทสต๊อก Lot หรือวันหมดอายุได้เฉพาะเมื่อ Lot นี้ยังไม่ถูกใช้ ระบบจะสร้าง movement ส่วนต่างและ audit ให้เสมอ"
            title="แก้ไขรายการรับเข้า"
          />
          {correction && detail ? (
            <div className="grid gap-5 md:grid-cols-[300px_minmax(0,1fr)]">
              <div>
                <p className="mb-2 text-sm font-bold">
                  สินค้าที่เลือก: {correction.product_name}
                </p>
                <PurchaseProductPicker
                  branchId={String(detail.branch_id)}
                  onChoose={(product) =>
                    setCorrection((current) =>
                      current
                        ? {
                            ...current,
                            product_id: String(product.id),
                            product_name: String(product.name),
                            tracks_expiry: Boolean(product.tracks_expiry),
                          }
                        : current,
                    )
                  }
                  stockBucket={correction.stock_bucket}
                />
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                {canUseGhost ? (
                  <Field label="ประเภทสต๊อก">
                    <Select
                      onChange={(event) =>
                        setCorrection({
                          ...correction,
                          stock_bucket: event.target.value as "real" | "ghost",
                        })
                      }
                      value={correction.stock_bucket}
                    >
                      <option value="real">สต๊อกจริง</option>
                      {String(branches.find((branch) => String(branch.id) === String(detail.branch_id))?.branch_type) === "main_warehouse" ? <option value="ghost">สต๊อกผี</option> : null}
                    </Select>
                  </Field>
                ) : null}
                <Field label="จำนวนรับเข้า">
                  <Input
                    min={1}
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        quantity: Number(event.target.value),
                      })
                    }
                    type="number"
                    value={correction.quantity}
                  />
                </Field>
                <Field label="ราคาซื้อ/หน่วย">
                  <Input
                    min={0}
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        unit_cost: Number(event.target.value),
                      })
                    }
                    step="0.01"
                    type="number"
                    value={correction.unit_cost}
                  />
                </Field>
                <Field label="ส่วนลดรายการ">
                  <Input
                    min={0}
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        line_discount: Number(event.target.value),
                      })
                    }
                    step="0.01"
                    type="number"
                    value={correction.line_discount}
                  />
                </Field>
                <Field label="Lot/Batch">
                  <Input
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        lot_number: event.target.value,
                      })
                    }
                    value={correction.lot_number}
                  />
                </Field>
                <Field
                  label={`วันหมดอายุ${correction.tracks_expiry ? " *" : ""}`}
                >
                  <Input
                    required={correction.tracks_expiry}
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        expires_on: event.target.value,
                      })
                    }
                    type="date"
                    value={correction.expires_on}
                  />
                </Field>
                <Field className="md:col-span-2" label="เหตุผลการแก้ไข *">
                  <Textarea
                    onChange={(event) =>
                      setCorrection({
                        ...correction,
                        reason: event.target.value,
                      })
                    }
                    value={correction.reason}
                  />
                </Field>
                <div className="flex justify-end gap-2 md:col-span-2">
                  <Button
                    onClick={() => setCorrectionOpen(false)}
                    type="button"
                    variant="secondary"
                  >
                    ยกเลิก
                  </Button>
                  <Button onClick={saveCorrection} type="button">
                    บันทึก correction
                  </Button>
                </div>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setHeaderEditOpen} open={headerEditOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader
            description="ปรับเงื่อนไข VAT ส่วนลด ค่าขนส่ง และหมายเหตุ โดยไม่เปลี่ยนจำนวนสต๊อก"
            title="แก้ไขยอดเอกสาร"
          />
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="VAT">
              <Select
                onChange={(event) =>
                  setHeaderEdit({ ...headerEdit, vat_mode: event.target.value })
                }
                value={headerEdit.vat_mode}
              >
                <option value="none">ไม่คิด VAT</option>
                <option value="exclusive">ราคาไม่รวม VAT</option>
                <option value="inclusive">ราคารวม VAT</option>
              </Select>
            </Field>
            <Field label="อัตรา VAT (%)">
              <Input
                disabled={headerEdit.vat_mode === "none"}
                min={0}
                onChange={(event) =>
                  setHeaderEdit({
                    ...headerEdit,
                    vat_rate: Number(event.target.value),
                  })
                }
                type="number"
                value={headerEdit.vat_rate}
              />
            </Field>
            <Field label="ส่วนลดท้ายบิล">
              <Input
                min={0}
                onChange={(event) =>
                  setHeaderEdit({
                    ...headerEdit,
                    header_discount: Number(event.target.value),
                  })
                }
                step="0.01"
                type="number"
                value={headerEdit.header_discount}
              />
            </Field>
            <Field label="ค่าขนส่ง">
              <Input
                min={0}
                onChange={(event) =>
                  setHeaderEdit({
                    ...headerEdit,
                    shipping_amount: Number(event.target.value),
                  })
                }
                step="0.01"
                type="number"
                value={headerEdit.shipping_amount}
              />
            </Field>
            <Field className="md:col-span-2" label="หมายเหตุ">
              <Textarea
                onChange={(event) =>
                  setHeaderEdit({ ...headerEdit, notes: event.target.value })
                }
                value={headerEdit.notes}
              />
            </Field>
            <Field className="md:col-span-2" label="เหตุผลการแก้ไข *">
              <Textarea
                onChange={(event) =>
                  setHeaderEdit({ ...headerEdit, reason: event.target.value })
                }
                value={headerEdit.reason}
              />
            </Field>
            <div className="flex justify-end gap-2 md:col-span-2">
              <Button
                onClick={() => setHeaderEditOpen(false)}
                type="button"
                variant="secondary"
              >
                ยกเลิก
              </Button>
              <Button onClick={saveHeaderEdit} type="button">
                บันทึก
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

/**
 * D4: the formal PO document (screen-hidden, print-only) — matches
 * PO-Form.jpg's sections: buyer/seller info, PO metadata, an itemized
 * table, totals, delivery terms, and signature blocks. Deliberately
 * separate from the operational lot/correction view above it, which stays
 * on-screen only since it's a working tool, not part of the printed
 * document.
 */
function PurchaseOrderPrintDocument({ detail }: { detail: Item }) {
  const items = (detail.items as Item[]) || [];
  const purchasedDate = detail.purchased_at
    ? new Date(String(detail.purchased_at)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })
    : "-";
  const dueDate = detail.due_date
    ? new Date(String(detail.due_date)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })
    : "-";
  const discountTotal = Number(detail.line_discount_total || 0) + Number(detail.header_discount || 0);

  return (
    <div className="hidden print:block">
      <div className="mb-6 flex items-start justify-between border-b pb-4">
        <div>
          <p className="text-lg font-black">PharmaPOS</p>
          <p className="mt-1 font-bold">{String(detail.branch_name)}</p>
          <p className="text-sm">{String(detail.branch_address || "-")}</p>
          {detail.branch_tax_id ? (
            <p className="text-sm">เลขประจำตัวผู้เสียภาษี {String(detail.branch_tax_id)}</p>
          ) : null}
        </div>
        <div className="text-right">
          <p className="text-2xl font-black">ใบสั่งซื้อ</p>
          <p className="text-sm text-muted-foreground">Purchase Order</p>
        </div>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-6 text-sm">
        <div>
          <p className="font-bold text-primary">ผู้จำหน่าย</p>
          <p className="mt-1 font-bold">{String(detail.supplier_name)}</p>
          <p>{String(detail.supplier_address || "-")}</p>
          {detail.supplier_tax_id ? (
            <p>เลขประจำตัวผู้เสียภาษี {String(detail.supplier_tax_id)}</p>
          ) : null}
        </div>
        <div className="space-y-1">
          <p className="flex justify-between">
            <span className="text-muted-foreground">เลขที่</span>
            <strong>{String(detail.po_number)}</strong>
          </p>
          <p className="flex justify-between">
            <span className="text-muted-foreground">วันที่</span>
            <span>{purchasedDate}</span>
          </p>
          <p className="flex justify-between">
            <span className="text-muted-foreground">ครบกำหนด</span>
            <span>{dueDate}</span>
          </p>
          <p className="flex justify-between">
            <span className="text-muted-foreground">ผู้สั่งซื้อ</span>
            <span>{String(detail.created_by_name || "-")}</span>
          </p>
          {detail.job_name ? (
            <p className="flex justify-between">
              <span className="text-muted-foreground">ชื่องาน</span>
              <span>{String(detail.job_name)}</span>
            </p>
          ) : null}
          {detail.supplier_contact ? (
            <p className="flex justify-between">
              <span className="text-muted-foreground">ผู้ติดต่อ</span>
              <span>{String(detail.supplier_contact)}</span>
            </p>
          ) : null}
          {detail.supplier_phone ? (
            <p className="flex justify-between">
              <span className="text-muted-foreground">เบอร์โทร</span>
              <span>{String(detail.supplier_phone)}</span>
            </p>
          ) : null}
          {detail.supplier_email ? (
            <p className="flex justify-between">
              <span className="text-muted-foreground">อีเมล</span>
              <span>{String(detail.supplier_email)}</span>
            </p>
          ) : null}
        </div>
      </div>

      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b-2 border-foreground text-left">
            <th className="w-10 py-2 pr-2">#</th>
            <th className="py-2 pr-2">รายละเอียด</th>
            <th className="py-2 pr-2 text-right">จำนวน</th>
            <th className="py-2 pr-2 text-right">ราคาต่อหน่วย</th>
            <th className="py-2 pr-2 text-right">ส่วนลด</th>
            <th className="py-2 text-right">มูลค่า</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr className="border-b" key={String(item.id)}>
              <td className="py-2 pr-2 align-top">{index + 1}</td>
              <td className="py-2 pr-2 align-top">
                {String(item.product_name)}
                <span className="block text-xs text-muted-foreground">{String(item.sku)}</span>
              </td>
              <td className="py-2 pr-2 text-right align-top">
                {Number(item.received_quantity).toLocaleString("th-TH")} {String(item.unit_name || "")}
              </td>
              <td className="py-2 pr-2 text-right align-top">{currency(Number(item.unit_cost))}</td>
              <td className="py-2 pr-2 text-right align-top">
                {Number(item.line_discount) > 0 ? currency(Number(item.line_discount)) : "-"}
              </td>
              <td className="py-2 text-right align-top">{currency(Number(item.line_subtotal))}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="ml-auto mt-4 max-w-xs space-y-1 text-sm">
        <p className="flex justify-between">
          <span>รวมเป็นเงิน</span>
          <span>{currency(Number(detail.subtotal))}</span>
        </p>
        {discountTotal > 0 ? (
          <p className="flex justify-between">
            <span>ส่วนลด</span>
            <span>-{currency(discountTotal)}</span>
          </p>
        ) : null}
        {Number(detail.shipping_amount) > 0 ? (
          <p className="flex justify-between">
            <span>ค่าขนส่ง</span>
            <span>{currency(Number(detail.shipping_amount))}</span>
          </p>
        ) : null}
        {detail.vat_mode !== "none" ? (
          <p className="flex justify-between">
            <span>ภาษีมูลค่าเพิ่ม {Number(detail.vat_rate)}%</span>
            <span>{currency(Number(detail.tax_amount))}</span>
          </p>
        ) : null}
        <p className="flex justify-between border-t-2 border-foreground pt-1 text-base font-black">
          <span>จำนวนเงินรวมทั้งสิ้น</span>
          <span>{currency(Number(detail.total_amount))}</span>
        </p>
      </div>

      {detail.delivery_terms || detail.notes ? (
        <div className="mt-8 grid gap-4 text-sm md:grid-cols-2">
          {detail.delivery_terms ? (
            <div>
              <p className="font-bold">ข้อมูลการจัดส่ง</p>
              <p className="whitespace-pre-wrap">{String(detail.delivery_terms)}</p>
            </div>
          ) : null}
          {detail.notes ? (
            <div>
              <p className="font-bold">หมายเหตุ</p>
              <p className="whitespace-pre-wrap">{String(detail.notes)}</p>
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="mt-16 grid grid-cols-2 gap-10 text-center text-sm">
        <div>
          <p className="mb-12">ในนาม {String(detail.branch_name)}</p>
          <p className="border-t pt-2">ผู้สั่งซื้อ</p>
          <p className="mt-6 border-t pt-2">วันที่</p>
        </div>
        <div>
          <p className="mb-12">ในนาม {String(detail.supplier_name)}</p>
          <p className="border-t pt-2">ผู้ขาย</p>
          <p className="mt-6 border-t pt-2">วันที่</p>
        </div>
      </div>
    </div>
  );
}
