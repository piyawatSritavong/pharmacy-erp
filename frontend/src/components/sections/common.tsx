import type { ReactNode } from "react";

import {
  Badge,
  Card,
  CardBody,
  CardHeader,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableEmptyState,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/primitives";
import { cn, currency, dateTime } from "@/lib/utils";

const statusLabels: Record<string, string> = {
  active: "ใช้งาน",
  approved: "อนุมัติแล้ว",
  cancelled: "ยกเลิก",
  calculated: "คำนวณแล้ว",
  completed: "เสร็จสิ้น",
  configured: "ตั้งค่าแล้ว",
  converted: "ออกใบขายแล้ว",
  draft: "ฉบับร่าง",
  finalized: "ยืนยันและล็อกแล้ว",
  ghost: "สต๊อกผี",
  in_transit: "กำลังขนส่ง",
  issued: "ออกเอกสารแล้ว",
  overdue: "เกินกำหนด",
  paid: "ชำระแล้ว",
  pending: "รอดำเนินการ",
  posted: "บันทึกแล้ว",
  processing: "กำลังดำเนินการ",
  queued: "รอประมวลผล",
  requested: "รอส่ง",
  real: "สต๊อกจริง",
  rejected: "ไม่อนุมัติ",
  unpaid: "ค้างชำระ",
  void: "ยกเลิกแล้ว",
  cash: "เงินสด",
  bank_transfer: "เงินโอน",
  other: "อื่นๆ"
};

const auditActionLabels: Record<string, string> = {
  "branch.create": "เพิ่มสาขา",
  "branch.delete": "ลบสาขา",
  "branch.update": "แก้ไขสาขา",
  "inventory.adjust": "ปรับยอดสต๊อก",
  "inventory.receive": "รับสินค้าเข้า",
  "inventory.rebalance": "ย้ายประเภทสต๊อก",
  "inventory.receipt_request.approve": "อนุมัติคำขอรับสินค้า",
  "inventory.receipt_request.create": "ส่งคำขอรับสินค้า",
  "inventory.receipt_request.reject": "ไม่รับนำเข้าสินค้า",
  "invoice.create": "สร้างใบขาย",
  "invoice.delete": "ลบใบขาย",
  "marketplace.upsert_connection": "บันทึกการเชื่อมต่อตลาดออนไลน์",
  "month_end.calculate": "คำนวณสรุปสิ้นเดือน",
  "month_end.finalize": "ยืนยันสรุปสิ้นเดือน",
  "pos.checkout": "ขายสินค้าหน้าร้าน",
  "product.create": "เพิ่มสินค้า",
  "product.delete": "ลบสินค้า",
  "product.image.delete": "ลบรูปสินค้า",
  "product.image.update": "อัปโหลดรูปสินค้า",
  "product.update": "แก้ไขสินค้า",
  "product_alias.create": "เพิ่มชื่อสินค้าสำหรับราชการ",
  "product_alias.delete": "ลบชื่อสินค้าสำหรับราชการ",
  "product_alias.update": "แก้ไขชื่อสินค้าสำหรับราชการ",
  "product_category.create": "เพิ่มหมวดสินค้า",
  "product_category.delete": "ลบหมวดสินค้า",
  "product_category.update": "แก้ไขหมวดสินค้า",
  "quotation.convert": "ออกใบขายจากใบเสนอราคา",
  "quotation.create": "สร้างใบเสนอราคา",
  "quotation.delete": "ลบใบเสนอราคา",
  "sequence.update": "แก้ไขเลขที่เอกสาร",
  "transfer.dispatch": "ส่งสินค้า",
  "transfer.receive": "รับโอนสินค้า",
  "transfer.request": "สร้างรายการโอน",
  "user.create": "เพิ่มผู้ใช้",
  "user.delete": "ลบผู้ใช้",
  "user.password.reset": "ตั้งรหัสผ่านใหม่",
  "user.update": "แก้ไขผู้ใช้"
};

const auditEntityLabels: Record<string, string> = {
  alias: "ชื่อสินค้าสำหรับราชการ",
  branch: "สาขา",
  category: "หมวดสินค้า",
  document_sequence: "เลขที่เอกสาร",
  inventory: "สต๊อก",
  inventory_receipt_request: "คำขอรับสินค้า",
  invoice: "ใบขาย",
  marketplace_connection: "การเชื่อมต่อตลาดออนไลน์",
  month_end_workpaper: "กระดาษทำการปิดเดือน",
  product: "สินค้า",
  quotation: "ใบเสนอราคา",
  transfer: "การโอนสินค้า",
  user: "ผู้ใช้"
};

export function statusLabel(value: unknown) {
  const key = String(value || "");
  return statusLabels[key] || key;
}

// PageIntro now lives with the sticky header it feeds — a page's title and
// description render up in the header bar, on one row with the bell and
// account controls, instead of on a line of their own below them. Re-exported
// here so every page can keep importing it from this module unchanged.
export { PageIntro } from "@/components/layout/page-header";

export function MetricGrid({
  items
}: {
  items: Array<{ key: string; label: string; value: string | number }>;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
      {items.map((item) => (
        <Card key={item.key}>
          <CardBody className="space-y-1.5 p-3 sm:p-5">
            <p className="text-xs font-medium text-muted-foreground">{item.label}</p>
            <p className="text-xl font-semibold tracking-tight tabular-nums sm:text-2xl">
              {typeof item.value === "number" ? item.value.toLocaleString("th-TH") : item.value}
            </p>
          </CardBody>
        </Card>
      ))}
    </div>
  );
}

export function SectionCard({
  title,
  description,
  actions,
  children,
  className
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardHeader title={title} description={description} actions={actions} />
      <CardBody>{children}</CardBody>
    </Card>
  );
}

export type DataTableColumn = {
  key: string;
  label: string;
  type?: "currency" | "date" | "datetime" | "default";
  /** Renders the cell yourself — for badges, chips or anything that isn't
   *  plain text. Falls back to the value formatting when omitted. */
  render?: (row: Record<string, unknown>) => ReactNode;
  /** Applied to both the header and every body cell in the column, so a
   *  column can be pinned to a width and kept from wrapping. */
  className?: string;
};

export function DataTable({
  columns,
  rows,
  rowActions,
  emptyDescription,
  mobileCards = false
}: {
  /** Compact record cards; keep scroll tables for cross-column comparison. */
  mobileCards?: boolean;
  columns: DataTableColumn[];
  rows: Array<Record<string, unknown>>;
  rowActions?: (row: Record<string, unknown>) => ReactNode;
  /** Optional secondary line under the shared A3 "ไม่มีรายการแสดง" message. */
  emptyDescription?: string;
}) {
  const columnCount = columns.length + (rowActions ? 1 : 0);
  return (
    <TableContainer>
      <Table className={mobileCards ? "mobile-card-table" : undefined} role="table">
        <TableHeader role="rowgroup">
          <TableRow className="hover:bg-transparent" role="row">
            {columns.map((column) => (
              <TableHead className={column.className} key={column.key} role="columnheader" scope="col">
                {column.label}
              </TableHead>
            ))}
            {rowActions ? <TableHead className="text-right">จัดการ</TableHead> : null}
          </TableRow>
        </TableHeader>
        <TableBody role="rowgroup">
          {rows.length === 0 ? (
            <TableEmptyState colSpan={columnCount} description={emptyDescription} />
          ) : (
            rows.map((row, rowIndex) => (
              <TableRow key={String(row.id || rowIndex)} className="text-sm text-foreground" role="row">
                {columns.map((column) => (
                  <TableCell className={column.className} data-label={column.label} data-primary={column.key === "name" || column.key === "product_name" || column.key === "invoice_number" || undefined} key={column.key} role="cell">
                    {column.render ? column.render(row) : renderCell(row[column.key], column.type)}
                  </TableCell>
                ))}
                {rowActions ? <TableCell className="text-right" data-actions="true" role="cell">{rowActions(row)}</TableCell> : null}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

function renderCell(value: unknown, type?: "currency" | "date" | "datetime" | "default") {
  if (value === null || value === undefined || value === "") {
    return <span className="text-muted-foreground">-</span>;
  }
  if (type === "currency" && typeof value === "number") {
    return currency(value);
  }
  if (type === "datetime" && typeof value === "string") {
    return dateTime(value);
  }
  if (type === "date" && typeof value === "string") {
    return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeZone: "Asia/Bangkok" }).format(new Date(value));
  }
  if (typeof value === "boolean") {
    return value ? "ใช่" : "ไม่";
  }
  if (typeof value === "string" && statusLabel(value) !== value) {
    return statusLabel(value);
  }
  return String(value);
}

export function InvoiceSummary({ summary }: { summary?: Record<string, unknown> }) {
  if (!summary) {
    return null;
  }

  return (
    <div className="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 sm:grid-cols-4">
      {[
        { key: "subtotal", label: "ยอดก่อนภาษี" },
        { key: "tax_rate", label: "VAT %" },
        { key: "tax_amount", label: "ภาษีมูลค่าเพิ่ม" },
        { key: "total_amount", label: "ยอดรวม" }
      ].map((item) => (
        <div key={item.key}>
          <p className="text-xs font-medium text-muted-foreground">{item.label}</p>
          <p className="mt-2 text-lg font-semibold text-black">
            {typeof summary[item.key] === "number"
              ? item.key === "tax_rate"
                ? `${summary[item.key]}%`
                : currency(Number(summary[item.key]))
              : "-"}
          </p>
        </div>
      ))}
    </div>
  );
}

// The audit log stores each change as JSON; showing it raw is unreadable. These
// turn a change object into a short list of "field: value" a person can scan.
const auditFieldLabels: Record<string, string> = {
  code: "รหัส", name: "ชื่อ", active: "สถานะใช้งาน", address: "ที่อยู่",
  branch_type: "ประเภทสาขา", sales_enabled: "เปิดขายหน้าร้าน", online_sales_enabled: "ขายออนไลน์",
  legal_name: "ชื่อบริษัท", customer_name: "ลูกค้า", invoice_number: "เลขที่ใบขาย",
  original_invoice_number: "เลขที่เดิม", new_invoice_number: "เลขที่ใหม่",
  total_amount: "ยอดรวม", subtotal: "ยอดก่อนภาษี", tax_amount: "ภาษี", quantity: "จำนวน",
  unit_price: "ราคาต่อหน่วย", cost_price: "ราคาทุน", base_selling_price: "ราคาขาย",
  sku: "SKU", tax_id: "เลขผู้เสียภาษี", reason: "เหตุผล", note: "หมายเหตุ", notes: "หมายเหตุ",
  payment_method: "ช่องทางชำระ", payment_type: "ช่องทางชำระ", status: "สถานะ",
  markup_percent: "กำไรเหนือทุน (%)", adjustment_percent: "ปรับราคา (%)", final_revenue: "ยอดเป้าหมาย",
  supplier_name: "คู่ค้า", po_number: "เลขที่ใบสั่งซื้อ", stock_bucket: "ประเภทสต๊อก",
  hidden: "ซ่อนบิล", request_full_tax_invoice: "ขอใบกำกับเต็มรูป"
};

const auditValueLabels: Record<string, string> = {
  main_warehouse: "โกดังหลัก", branch: "สาขา", cash: "เงินสด", bank_transfer: "เงินโอน",
  mixed: "ผสม", real: "สต๊อกจริง", ghost: "สต๊อกผี", paid: "ชำระแล้ว", unpaid: "ยังไม่ชำระ",
  issued: "ออกบิลแล้ว", full: "เต็มรูป", abbreviated: "อย่างย่อ"
};

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function formatAuditValue(key: string, value: unknown): string | null {
  if (value === null || value === undefined || value === "") return null;
  if (typeof value === "boolean") return value ? "ใช่" : "ไม่";
  if (typeof value === "number") {
    if (/price|amount|total|revenue|subtotal|cost/.test(key)) {
      return new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" }).format(value);
    }
    return value.toLocaleString("th-TH");
  }
  const text = String(value);
  if (uuidPattern.test(text)) return null; // a raw id says nothing to a reader
  if (/^\d{4}-\d{2}-\d{2}T/.test(text)) {
    const parsed = new Date(text);
    if (!Number.isNaN(parsed.getTime())) {
      return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(parsed);
    }
  }
  return auditValueLabels[text] || text;
}

function auditChangeEntries(raw: unknown): Array<[string, string]> {
  let parsed: unknown = null;
  if (typeof raw === "string") {
    try { parsed = JSON.parse(raw); } catch { return []; }
  } else {
    parsed = raw;
  }
  // JSON.parse("null") succeeds with null, and a change payload can also be a
  // primitive; only a plain object has fields to list.
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return [];
  const data = parsed as Record<string, unknown>;
  const entries: Array<[string, string]> = [];
  for (const [key, value] of Object.entries(data)) {
    if (key.endsWith("_id") || key === "id") continue; // ids are not reader-facing
    const formatted = formatAuditValue(key, value);
    if (formatted === null) continue;
    entries.push([auditFieldLabels[key] || key, formatted]);
  }
  return entries;
}

export function AuditTimeline({ items }: { items: Array<Record<string, unknown>> }) {
  return (
    <div className="space-y-3">
      {items.map((item, index) => (
        <div
          key={String(item.id || index)}
          className="rounded-lg border border-border bg-muted/50 p-4"
        >
          <div className="flex flex-wrap items-center gap-3">
            <Badge className="bg-white">
              {auditActionLabels[String(item.action || "")] || "ตรวจสอบข้อมูล"}
            </Badge>
            <p className="text-sm font-medium text-black">
              {auditEntityLabels[String(item.entity_type || "")] || "ข้อมูลระบบ"}
            </p>
            <p className="text-xs text-muted-foreground">{String(item.actor_name || "ระบบ")}</p>
          </div>
          {(() => {
            const entries = auditChangeEntries(item.after_data);
            if (entries.length === 0) {
              return <p className="mt-3 text-xs text-muted-foreground">ไม่มีรายละเอียดเพิ่มเติม</p>;
            }
            return (
              <dl className="mt-3 grid gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
                {entries.map(([label, value]) => (
                  <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:gap-2" key={label}>
                    <dt className="break-words text-muted-foreground sm:shrink-0">{label}</dt>
                    <dd className="min-w-0 flex-1 break-words font-medium text-foreground sm:truncate">{value}</dd>
                  </div>
                ))}
              </dl>
            );
          })()}
        </div>
      ))}
    </div>
  );
}

export function Grid({
  className,
  children
}: {
  className?: string;
  children: ReactNode;
}) {
  return <div className={cn("grid gap-6 xl:grid-cols-[1.2fr_0.8fr]", className)}>{children}</div>;
}
