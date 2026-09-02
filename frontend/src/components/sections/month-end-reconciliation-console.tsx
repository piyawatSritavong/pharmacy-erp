"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, ChevronDown, ChevronRight, Eye, FileClock, Loader2, LockKeyhole, RefreshCw, ShieldCheck } from "lucide-react";
import { toast } from "sonner";

import { PageIntro, SectionCard } from "@/components/sections/common";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  Field,
  Input,
  MultiSelect,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/primitives";
import { groupByPeriod, shortDate } from "@/lib/month-end-groups";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Row = Record<string, unknown>;

type ReconciliationItem = {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  quantity: number;
  unit_price: number;
  line_total: number;
  cost_basis: number;
  new_unit_price: number;
  new_line_total: number;
  variance_amount: number;
  repriced: boolean;
  missing_cost: boolean;
  ghost_stock_available: number;
};

type ReconciliationInvoice = {
  id: string;
  branch_name: string;
  invoice_number: string;
  customer_name: string;
  created_at: string;
  total_amount: number;
  final_total: number;
  variance_amount: number;
  classification: string;
  items: ReconciliationItem[];
};

type InvoiceGroup = { invoice_count: number; revenue: number };

type PlanSummary = {
  original_revenue: number;
  cash_no_tax_revenue: number;
  hidden_revenue: number;
  repriced_original_revenue: number;
  repriced_final_revenue: number;
  adjustment_reduction: number;
  unchanged_revenue: number;
  final_revenue: number;
  invoice_count: number;
  hidden_invoice_count: number;
  repriced_invoice_count: number;
  unchanged_invoice_count: number;
  adjusted_item_count: number;
  missing_cost_item_count: number;
  adjustment_percent: number;
  minimum_adjustment_percent: number;
  maximum_adjustment_percent: number;
};

type Overview = PlanSummary & {
  invoice_groups: {
    cash_hidden_ghost: InvoiceGroup;
    cash_repriced: InvoiceGroup;
    cash_full_tax: InvoiceGroup;
    bank_transfer: InvoiceGroup;
    mixed: InvoiceGroup;
    unclassified: InvoiceGroup;
  };
};

type StockProjection = {
  branch_real_returned: number;
  warehouse_real_received: number;
  warehouse_ghost_deducted: number;
  ghost_deficit_created: number;
  products: Array<{
    product_id: string;
    product_name: string;
    quantity: number;
    warehouse_ghost_before: number;
    warehouse_ghost_after: number;
    deficit_created: number;
  }>;
};

type Preview = PlanSummary & {
  suppression_candidates: ReconciliationInvoice[];
  repriced_invoices: ReconciliationInvoice[];
  stock_projection: StockProjection;
};

type HistoryItem = {
  id: string;
  reconciliation_number: string;
  period_start: string;
  period_end: string;
  original_revenue: number;
  suppressed_revenue: number;
  adjustment_reduction: number;
  adjustment_percent: number;
  final_revenue: number;
  suppressed_invoice_count: number;
  reconciliation_mode: string;
  finalized_by_name: string;
  finalized_at: string;
};

const MIN_MARKUP = 5;
const MAX_MARKUP = 10;

function todayISO() {
  return new Intl.DateTimeFormat("en-CA", { day: "2-digit", month: "2-digit", timeZone: "Asia/Bangkok", year: "numeric" }).format(new Date());
}

function monthStartISO() {
  return `${todayISO().slice(0, 7)}-01`;
}

function dateTime(value: string) {
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(new Date(value));
}

function text(value: unknown) {
  return value == null ? "" : String(value);
}

function num(value: unknown) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}

function count(value: number) {
  return value.toLocaleString("th-TH");
}

/** Markup must be 5–10 with at most two decimals; returns null when invalid. */
function parseMarkup(value: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < MIN_MARKUP || parsed > MAX_MARKUP) return null;
  if (Math.abs(parsed * 100 - Math.round(parsed * 100)) > 0.000001) return null;
  return parsed;
}

const logNames: Record<string, string> = {
  invoice_suppressed: "ซ่อนใบขาย",
  invoice_renumbered: "เรียงเลขใบขายใหม่",
  invoice_repriced: "บันทึกบิลที่ต้นทุน + %",
  price_adjusted: "ปรับราคาบรรทัดเป็นต้นทุน + %",
  stock_reversed: "ย้อนการตัด Real เดิม",
  stock_received: "WH รับ Real คืน",
  stock_deducted: "ส่งคืน Real / ตัด Ghost",
  stock_deficit: "บันทึก Ghost deficit"
};

function GroupRow({ group, label }: { group: InvoiceGroup; label: string }) {
  return (
    <div className="flex justify-between gap-3 border-b py-1.5 last:border-0">
      <span>{label}</span>
      <span className="shrink-0 font-medium tabular-nums">{count(group.invoice_count)} ใบ · {currency(group.revenue)}</span>
    </div>
  );
}

function itemSummary(item: ReconciliationItem) {
  if (item.missing_cost) return `${item.product_name} × ${count(item.quantity)} (ไม่มีต้นทุน คงราคาเดิม)`;
  if (!item.repriced) return `${item.product_name} × ${count(item.quantity)} (คงราคาเดิม)`;
  return `${item.product_name} × ${count(item.quantity)}: ${currency(item.unit_price)} → ${currency(item.new_unit_price)} (ต้นทุน ${currency(item.cost_basis)})`;
}

export function MonthEndReconciliationConsole({ branches }: { branches: Row[] }) {
  const sellingBranches = useMemo(
    () => branches.filter((branch) => Boolean(branch.active ?? true) && Boolean(branch.sales_enabled ?? true) && text(branch.branch_type) !== "main_warehouse"),
    [branches]
  );
  const [dateFrom, setDateFrom] = useState(monthStartISO());
  const [dateTo, setDateTo] = useState(todayISO());
  const [branchIDs, setBranchIDs] = useState<string[]>(sellingBranches.map((branch) => text(branch.id)));
  const [markupPercent, setMarkupPercent] = useState("5");
  const [overview, setOverview] = useState<Overview | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [audit, setAudit] = useState<Row | null>(null);
  const [foldedMonths, setFoldedMonths] = useState<Record<string, boolean>>({});
  const [confirmation, setConfirmation] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [loading, setLoading] = useState(false);

  const branchOptions = useMemo(
    () => sellingBranches.map((branch) => ({ label: text(branch.name), value: text(branch.id) })),
    [sellingBranches]
  );
  const selectedBranches = useMemo(
    () => sellingBranches.filter((branch) => branchIDs.includes(text(branch.id))),
    [branchIDs, sellingBranches]
  );
  // History is grouped by the month a round covers: rounds accumulate for years,
  // and one month can hold several of them because the range is user-chosen.
  const historyMonths = useMemo(() => groupByPeriod(history), [history]);
  const markup = parseMarkup(markupPercent);
  const markupFactor = markup == null ? null : (1 + markup / 100).toFixed(4).replace(/0+$/, "").replace(/\.$/, "");

  const loadHistory = useCallback(async () => {
    try {
      const response = await proxyClient<{ items: HistoryItem[] }>("/accounting/month-end/reconciliations");
      setHistory(response.items);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "โหลดประวัติสรุปสิ้นเดือนไม่สำเร็จ");
    }
  }, []);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory]);

  function resetPlan() {
    setOverview(null);
    setPreview(null);
  }

  function payload() {
    return {
      date_from: dateFrom,
      date_to: dateTo,
      branch_ids: branchIDs,
      adjustment_percent: markup ?? MIN_MARKUP
    };
  }

  async function loadPlan() {
    if (!dateFrom || !dateTo || branchIDs.length === 0) {
      toast.error("กรุณาเลือกช่วงวันที่และอย่างน้อยหนึ่งสาขาขาย");
      return;
    }
    if (dateTo < dateFrom) {
      toast.error("วันที่สิ้นสุดต้องไม่ก่อนวันที่เริ่มต้น");
      return;
    }
    if (markup == null) {
      toast.error(`กำไรเหนือต้นทุนต้องอยู่ระหว่าง ${MIN_MARKUP} ถึง ${MAX_MARKUP}% ทศนิยมไม่เกิน 2 ตำแหน่ง`);
      return;
    }
    setLoading(true);
    try {
      const request = { method: "POST", body: JSON.stringify(payload()) };
      const [nextOverview, nextPreview] = await Promise.all([
        proxyClient<Overview>("/accounting/month-end/reconciliation-overview", request),
        proxyClient<Preview>("/accounting/month-end/reconciliation-preview", request)
      ]);
      setOverview(nextOverview);
      setPreview(nextPreview);
      toast.success("คำนวณยอดเป้าหมายและ projection สต๊อกล่าสุดแล้ว");
    } catch (error) {
      setOverview(null);
      setPreview(null);
      toast.error(error instanceof Error ? error.message : "ตรวจสอบรอบสรุปสิ้นเดือนไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  const closeText = `ยืนยันสรุป ${dateFrom} ถึง ${dateTo}`;

  async function finalize() {
    if (!preview || confirmation !== closeText) return;
    setLoading(true);
    try {
      const response = await proxyClient<{ item: Row; message: string }>("/accounting/month-end/reconciliations", {
        method: "POST",
        body: JSON.stringify(payload())
      });
      toast.success(response.message);
      setAudit(response.item);
      setConfirmOpen(false);
      setConfirmation("");
      resetPlan();
      await loadHistory();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "ยืนยันสรุปสิ้นเดือนไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  async function openAudit(id: string) {
    setLoading(true);
    try {
      setAudit(await proxyClient<Row>(`/accounting/month-end/reconciliations/${id}`));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "โหลด Audit Log ไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageIntro
        title="สรุปสิ้นเดือน"
        description="เฉพาะผู้ดูแลระบบสูงสุด · บิลเงินสดที่มีในสต๊อกผีจะถูกส่งคืน WH ตัด Ghost และซ่อน ส่วนบิลเงินสดที่ไม่มีในสต๊อกผีจะบันทึกที่ต้นทุน + กำไร ภายในธุรกรรมเดียว"
      />

      <SectionCard title="กำหนดขอบเขตรอบ" description="เลือกสาขาขายเท่านั้น ระบบไม่อนุญาตให้เลือกโกดัง WH เป็นสาขาต้นทาง">
        <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-[1fr_1fr_1.7fr_1fr_1fr]">
          <Field label="วันที่เริ่มต้น">
            <Input max={dateTo || todayISO()} onChange={(event) => { setDateFrom(event.target.value); resetPlan(); }} type="date" value={dateFrom} />
          </Field>
          <Field label="วันที่สิ้นสุด">
            <Input max={todayISO()} min={dateFrom} onChange={(event) => { setDateTo(event.target.value); resetPlan(); }} type="date" value={dateTo} />
          </Field>
          <Field hint={`เลือก ${branchIDs.length}/${sellingBranches.length}`} label="สาขาขาย">
            <MultiSelect label="สาขาขาย" onChange={(next) => { setBranchIDs(next); resetPlan(); }} options={branchOptions} placeholder="เลือกสาขา" searchPlaceholder="ค้นหาสาขา" value={branchIDs} />
          </Field>
          <Field hint={markup == null ? `ต้องอยู่ระหว่าง ${MIN_MARKUP}–${MAX_MARKUP}%` : `ต้นทุน × ${markupFactor}`} label="กำไรเหนือต้นทุน (%)">
            <Input max={MAX_MARKUP} min={MIN_MARKUP} onChange={(event) => { setMarkupPercent(event.target.value); resetPlan(); }} step="0.01" type="number" value={markupPercent} />
          </Field>
          <Field hint="คำนวณจากกติกา ไม่ต้องกรอก" label="ยอดเป้าหมาย">
            <Input disabled readOnly value={preview ? currency(preview.final_revenue) : ""} placeholder="กดตรวจสอบเพื่อคำนวณ" />
          </Field>
        </div>
        <div className="mt-5 flex justify-end">
          <Button disabled={loading} onClick={() => void loadPlan()}>
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            ตรวจสอบใบขายและคำนวณยอดเป้าหมาย
          </Button>
        </div>
      </SectionCard>

      {overview && preview ? (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg border bg-info-50 p-4"><p className="text-xs text-muted-foreground">ยอดขาย issued + paid ทั้งหมด</p><p className="mt-1 text-xl font-semibold">{currency(overview.original_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">{count(overview.invoice_count)} ใบในขอบเขต</p></div>
            <div className="rounded-lg border bg-warning-50 p-4"><p className="text-xs text-muted-foreground">ซ่อน · เงินสด มีในสต๊อกผี</p><p className="mt-1 text-xl font-semibold">{currency(overview.hidden_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">{count(overview.hidden_invoice_count)} ใบ ส่งคืน WH และตัด Ghost แล้วตัดออกจากการคำนวณ</p></div>
            <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">บันทึกที่ต้นทุน + {overview.adjustment_percent}% · เงินสด ไม่มีในสต๊อกผี</p><p className="mt-1 text-xl font-semibold">{currency(overview.repriced_original_revenue)} → {currency(overview.repriced_final_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">{count(overview.repriced_invoice_count)} ใบ · ส่วนต่าง {currency(overview.adjustment_reduction)}</p></div>
            <div className="rounded-lg border bg-success-50 p-4"><p className="text-xs text-muted-foreground">ยอดเป้าหมาย</p><p className="mt-1 text-xl font-semibold">{currency(overview.final_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">บิลไม่เข้าเงื่อนไข {currency(overview.unchanged_revenue)} + บันทึกใหม่ {currency(overview.repriced_final_revenue)}</p></div>
          </div>

          <SectionCard title="รายละเอียดกลุ่มบิลและสูตรจากข้อมูลจริง" description="ค่าทั้งหมด fetch มาจากใบขาย การชำระเงิน และ Ghost Stock ที่ WH ในขอบเขตเดียวกับรอบ">
            <div className="grid gap-5 lg:grid-cols-2">
              <div className="rounded-lg border p-4 text-sm">
                <p className="font-semibold">องค์ประกอบยอดขายทั้งหมด</p>
                <div className="mt-2 text-muted-foreground">
                  <GroupRow group={overview.invoice_groups.cash_hidden_ghost} label="เงินสด ไม่ขอใบกำกับเต็มรูป · มีในสต๊อกผี (ซ่อน)" />
                  <GroupRow group={overview.invoice_groups.cash_repriced} label="เงินสด ไม่ขอใบกำกับเต็มรูป · ไม่มีในสต๊อกผี (ต้นทุน + %)" />
                  <GroupRow group={overview.invoice_groups.cash_full_tax} label="เงินสด ขอใบกำกับเต็มรูป" />
                  <GroupRow group={overview.invoice_groups.bank_transfer} label="โอนเงิน" />
                  <GroupRow group={overview.invoice_groups.mixed} label="ชำระผสม" />
                  {overview.invoice_groups.unclassified.invoice_count > 0 ? <GroupRow group={overview.invoice_groups.unclassified} label="ข้อมูลช่องทางชำระไม่ครบ" /> : null}
                </div>
                <p className="mt-3 text-xs text-muted-foreground">สูตร: ผลรวมยอดเต็มของทุกกลุ่ม = {currency(overview.original_revenue)}</p>
              </div>
              <div className="rounded-lg border p-4 text-sm">
                <p className="font-semibold">กติกาของรอบ</p>
                <ol className="mt-2 list-decimal space-y-1 pl-5 text-muted-foreground">
                  <li>บิลเงินสดที่ Ghost Stock ที่ WH มีพอทุกบรรทัด → ส่งคืนโกดัง ตัดผี และซ่อน (ตัดออกจากการคำนวณ)</li>
                  <li>บิลเงินสดที่ Ghost Stock ไม่พอ → คงบิลไว้ บันทึกราคาต่อหน่วย = ต้นทุน × {markupFactor} ({overview.adjustment_percent}% = {100 + overview.adjustment_percent}%)</li>
                  <li>บิลโอน / ชำระผสม / ขอใบกำกับเต็มรูป → ไม่แตะต้อง</li>
                </ol>
                <div className="mt-3 space-y-1 tabular-nums">
                  <p>ยอดทั้งหมด {currency(overview.original_revenue)} − ซ่อน {currency(overview.hidden_revenue)}</p>
                  <p>บิลไม่เข้าเงื่อนไข {currency(overview.unchanged_revenue)} + (เงินสดไม่มีในสต๊อกผี {currency(overview.repriced_original_revenue)} → {currency(overview.repriced_final_revenue)})</p>
                  <p>ยอดเป้าหมาย = <strong>{currency(overview.final_revenue)}</strong> · ส่วนต่างที่บันทึกลด {currency(overview.adjustment_reduction)}</p>
                </div>
                {overview.missing_cost_item_count > 0 ? <p className="mt-3 text-xs text-warning-700">มี {count(overview.missing_cost_item_count)} บรรทัดที่ไม่มีต้นทุน ระบบคงราคาเดิมของบรรทัดนั้น</p> : null}
              </div>
            </div>
          </SectionCard>

          <SectionCard title="Projection การเคลื่อนไหวสต๊อก" description="เฉพาะบิลที่ซ่อน: แหล่งตัดเป็น Ghost หนึ่งค่า ส่วนการย้อนและส่งคืน Real เป็น adjustment แยก บิลที่บันทึกที่ต้นทุน + % ไม่แตะสต๊อก">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {[
                ["Branch Real returned", preview.stock_projection.branch_real_returned],
                ["WH Real received", preview.stock_projection.warehouse_real_received],
                ["WH Ghost deducted", preview.stock_projection.warehouse_ghost_deducted],
                ["Ghost deficit", preview.stock_projection.ghost_deficit_created]
              ].map(([label, value]) => <div className="rounded-lg border p-4" key={String(label)}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-xl font-semibold">{count(Number(value))} ชิ้น</p></div>)}
            </div>
            <TableContainer className="mt-4">
              <Table>
                <TableHeader><TableRow><TableHead>สินค้า</TableHead><TableHead className="text-right">จำนวนบิล</TableHead><TableHead className="text-right">WH Ghost ก่อน</TableHead><TableHead className="text-right">WH Ghost หลัง</TableHead><TableHead className="text-right">Deficit ใหม่</TableHead></TableRow></TableHeader>
                <TableBody>
                  {preview.stock_projection.products.length === 0 ? <TableRow><TableCell className="py-6 text-center text-muted-foreground" colSpan={5}>ไม่มีบิลที่ซ่อน จึงไม่มีการเคลื่อนไหวสต๊อก</TableCell></TableRow> : preview.stock_projection.products.map((product) => <TableRow key={product.product_id}><TableCell>{product.product_name}</TableCell><TableCell className="text-right">{count(product.quantity)}</TableCell><TableCell className="text-right">{count(product.warehouse_ghost_before)}</TableCell><TableCell className="text-right">{count(product.warehouse_ghost_after)}</TableCell><TableCell className="text-right">{count(product.deficit_created)}</TableCell></TableRow>)}
                </TableBody>
              </Table>
            </TableContainer>
          </SectionCard>

          <SectionCard title={`ใบขายที่จะซ่อน ${count(preview.hidden_invoice_count)} ใบ`} description="เงินสด ไม่ขอใบกำกับเต็มรูป และ Ghost Stock ที่ WH มีพอทุกบรรทัด (จัดสรร Ghost เรียงตาม created_at) หลังซ่อนจะเรียงเลขใบขาย Active ที่เหลือใหม่แยกสาขา">
            <TableContainer>
              <Table>
                <TableHeader><TableRow><TableHead>เลขบิล</TableHead><TableHead>สาขา</TableHead><TableHead>วันเวลา created_at</TableHead><TableHead>สินค้า</TableHead><TableHead className="text-right">ยอดเต็ม</TableHead><TableHead>แหล่งตัดหลังสรุป</TableHead></TableRow></TableHeader>
                <TableBody>
                  {preview.suppression_candidates.length === 0 ? <TableRow><TableCell className="py-10 text-center text-muted-foreground" colSpan={6}>ไม่มีใบขายที่เข้าเงื่อนไขซ่อน</TableCell></TableRow> : preview.suppression_candidates.map((invoice) => <TableRow key={invoice.id}><TableCell className="font-medium">{invoice.invoice_number}</TableCell><TableCell>{invoice.branch_name}</TableCell><TableCell className="whitespace-nowrap">{dateTime(invoice.created_at)}</TableCell><TableCell>{invoice.items.map((item) => `${item.product_name} × ${count(item.quantity)}`).join(", ")}</TableCell><TableCell className="text-right">{currency(invoice.total_amount)}</TableCell><TableCell>Ghost Stock (สต๊อกผี)</TableCell></TableRow>)}
                </TableBody>
              </Table>
            </TableContainer>
          </SectionCard>

          <SectionCard title={`ใบขายที่จะบันทึกที่ต้นทุน + ${preview.adjustment_percent}% · ${count(preview.repriced_invoice_count)} ใบ`} description="เงินสด ไม่ขอใบกำกับเต็มรูป แต่ Ghost Stock ที่ WH ไม่พอ บิลยังอยู่ เลขบิลและสต๊อกไม่เปลี่ยน ราคาต่อหน่วยบันทึกใหม่เป็นต้นทุน × (1 + %) และยอดชำระเงินสดลดตาม">
            <TableContainer>
              <Table>
                <TableHeader><TableRow><TableHead>เลขบิล</TableHead><TableHead>สาขา</TableHead><TableHead>วันเวลา created_at</TableHead><TableHead>รายการ (ราคาเดิม → ราคาที่บันทึก)</TableHead><TableHead className="text-right">ยอดเต็ม</TableHead><TableHead className="text-right">ยอดที่บันทึก</TableHead><TableHead className="text-right">ส่วนต่าง</TableHead></TableRow></TableHeader>
                <TableBody>
                  {preview.repriced_invoices.length === 0 ? <TableRow><TableCell className="py-10 text-center text-muted-foreground" colSpan={7}>ไม่มีใบขายเงินสดที่ Ghost Stock ไม่พอ</TableCell></TableRow> : preview.repriced_invoices.map((invoice) => <TableRow key={invoice.id}><TableCell className="font-medium">{invoice.invoice_number}</TableCell><TableCell>{invoice.branch_name}</TableCell><TableCell className="whitespace-nowrap">{dateTime(invoice.created_at)}</TableCell><TableCell className="min-w-64">{invoice.items.map((item) => <p key={item.id}>{itemSummary(item)}</p>)}</TableCell><TableCell className="text-right">{currency(invoice.total_amount)}</TableCell><TableCell className="text-right font-semibold">{currency(invoice.final_total)}</TableCell><TableCell className="text-right">{currency(invoice.variance_amount)}</TableCell></TableRow>)}
                </TableBody>
              </Table>
            </TableContainer>
          </SectionCard>

          <div className="flex flex-col gap-4 rounded-lg border border-warning-200 bg-warning-50 p-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex gap-3"><AlertTriangle className="mt-0.5 h-5 w-5 text-warning-700" /><div><p className="font-semibold">แจ้งเพื่อทราบก่อนยืนยัน</p><p className="mt-1 text-sm text-muted-foreground">ระบบจะซ่อน {count(preview.hidden_invoice_count)} ใบ บันทึก {count(preview.repriced_invoice_count)} ใบที่ต้นทุน + {preview.adjustment_percent}% ({count(preview.adjusted_item_count)} บรรทัด) และบันทึกยอดเป้าหมาย {currency(preview.final_revenue)} ธุรกรรมนี้ย้อนกลับไม่ได้</p></div></div>
            <Button disabled={loading} onClick={() => setConfirmOpen(true)}><LockKeyhole className="h-4 w-4" />ยืนยันและสรุปรอบ</Button>
          </div>
        </>
      ) : null}

      <SectionCard title="ประวัติการสรุป" description="ยุบเป็นกลุ่มตามเดือนของรอบ · Superadmin เห็นช่วงวันที่และเวลาเต็ม พร้อม Audit Log ทุกขั้น">
        {historyMonths.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">ยังไม่มีประวัติการสรุป</p>
        ) : (
          <div className="space-y-3">
            {historyMonths.map((bucket, index) => {
              const folded = foldedMonths[bucket.key] ?? index > 0;
              const monthTotal = bucket.rounds.reduce((sum, item) => sum + Number(item.final_revenue || 0), 0);
              return (
                <div className="overflow-hidden rounded-lg border" key={bucket.key}>
                  <button
                    aria-expanded={!folded}
                    className="flex w-full items-center gap-3 bg-muted/40 px-4 py-3 text-left transition hover:bg-muted"
                    onClick={() => setFoldedMonths((current) => ({ ...current, [bucket.key]: !folded }))}
                    type="button"
                  >
                    {folded ? <ChevronRight className="h-4 w-4 shrink-0" /> : <ChevronDown className="h-4 w-4 shrink-0" />}
                    <span className="min-w-0 flex-1">
                      <span className="block font-semibold">{bucket.label}</span>
                      <span className="block text-xs text-muted-foreground">{bucket.rounds.map((item) => item.reconciliation_number).join(" · ")}</span>
                    </span>
                    <span className="shrink-0 text-right text-sm">
                      <span className="block font-semibold tabular-nums">{currency(monthTotal)}</span>
                      <span className="block text-xs text-muted-foreground">{count(bucket.rounds.length)} รอบ</span>
                    </span>
                  </button>
                  {folded ? null : (
                    <TableContainer>
                      <Table>
                        <TableHeader><TableRow><TableHead>เลขรายการ</TableHead><TableHead>ช่วงวันที่</TableHead><TableHead className="text-right">ยอดเดิม</TableHead><TableHead className="text-right">ยอดซ่อน</TableHead><TableHead className="text-right">ส่วนต่างต้นทุน + %</TableHead><TableHead className="text-right">ยอดเป้าหมาย</TableHead><TableHead>ผู้ยืนยัน / เวลา</TableHead><TableHead /></TableRow></TableHeader>
                        <TableBody>
                          {bucket.rounds.map((item) => <TableRow key={item.id}><TableCell className="font-medium">{item.reconciliation_number}</TableCell><TableCell className="whitespace-nowrap">{shortDate(item.period_start)} ถึง {shortDate(item.period_end)}</TableCell><TableCell className="text-right tabular-nums">{currency(item.original_revenue)}</TableCell><TableCell className="text-right tabular-nums">{currency(item.suppressed_revenue)}</TableCell><TableCell className="text-right tabular-nums">{currency(item.adjustment_reduction)}{item.adjustment_percent > 0 ? <span className="block text-xs text-muted-foreground">{item.adjustment_percent}%</span> : null}</TableCell><TableCell className="text-right font-semibold tabular-nums">{currency(item.final_revenue)}</TableCell><TableCell><p>{item.finalized_by_name}</p><p className="text-xs text-muted-foreground">{dateTime(item.finalized_at)}</p></TableCell><TableCell className="text-right"><Button onClick={() => void openAudit(item.id)} variant="secondary"><Eye className="h-4 w-4" />Audit</Button></TableCell></TableRow>)}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </SectionCard>

      <Dialog onOpenChange={setConfirmOpen} open={confirmOpen}>
        <DialogContent>
          <DialogHeader title="ยืนยันการสรุปรอบ" description="ธุรกรรมนี้ซ่อนใบขายที่มีในสต๊อกผี ส่ง Real กลับ WH ตัด Ghost บันทึกบิลที่ไม่มีในสต๊อกผีที่ต้นทุน + % และเรียงเลขบิล Active ใหม่" />
          <div className="rounded-lg border border-warning-200 bg-warning-50 p-4 text-sm"><p className="font-semibold">ผลที่จะบันทึก</p><p className="mt-2">{selectedBranches.map((branch) => text(branch.name)).join(", ")}</p><p className="mt-1">{dateFrom} ถึง {dateTo} · ซ่อน {preview?.hidden_invoice_count || 0} ใบ · บันทึกที่ต้นทุน + {preview?.adjustment_percent ?? markup}% {preview?.repriced_invoice_count || 0} ใบ · ยอดเป้าหมาย {currency(preview?.final_revenue || 0)}</p></div>
          <label className="mt-4 block space-y-2 text-sm"><span>พิมพ์ <strong>{closeText}</strong></span><Input autoFocus onChange={(event) => setConfirmation(event.target.value)} value={confirmation} /></label>
          <div className="mt-5 flex justify-end gap-2"><Button onClick={() => setConfirmOpen(false)} variant="secondary">ยกเลิก</Button><Button disabled={confirmation !== closeText || loading} onClick={() => void finalize()}>{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}ยืนยันธุรกรรม</Button></div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { if (!open) setAudit(null); }} open={Boolean(audit)}>
        <DialogContent className="max-w-5xl">
          <DialogHeader title={`Audit Log · ${text(audit?.reconciliation_number)}`} description={`${text(audit?.period_start)} ถึง ${text(audit?.period_end)} · ${text(audit?.finalized_by_name)} · ${audit?.finalized_at ? dateTime(text(audit.finalized_at)) : ""}${num(audit?.adjustment_percent) > 0 ? ` · ต้นทุน + ${num(audit?.adjustment_percent)}%` : ""} · ยอดเป้าหมาย ${currency(num(audit?.final_revenue))}`} />
          <div className="mt-4 max-h-[60vh] overflow-auto rounded-lg border">
            <Table><TableHeader><TableRow><TableHead>เวลา</TableHead><TableHead>เหตุการณ์</TableHead><TableHead>เลขบิล</TableHead><TableHead>สินค้า</TableHead><TableHead>บทบาท movement</TableHead><TableHead>จำนวน / ราคา</TableHead></TableRow></TableHeader><TableBody>
              {(audit?.logs as Row[] | undefined || []).map((log) => <TableRow key={text(log.id)}><TableCell className="whitespace-nowrap">{dateTime(text(log.created_at))}</TableCell><TableCell><span className="inline-flex items-center gap-1 font-medium"><FileClock className="h-4 w-4" />{logNames[text(log.log_type)] || text(log.log_type)}</span></TableCell><TableCell><p>{text(log.original_invoice_number) || "-"}</p>{log.new_invoice_number ? <p className="text-xs text-muted-foreground">→ {text(log.new_invoice_number)}</p> : null}</TableCell><TableCell>{text(log.product_name) || "-"}</TableCell><TableCell>{text(log.movement_role) || "-"}</TableCell><TableCell className="whitespace-nowrap">{log.old_unit_price != null ? `${currency(num(log.old_unit_price))} → ${currency(num(log.new_unit_price))}` : text(log.log_type) === "invoice_repriced" ? `ส่วนต่าง ${currency(num(log.variance_amount))}` : text(log.stock_bucket) ? `${text(log.stock_bucket) === "ghost" ? "Ghost" : "Real"} ${count(num(log.stock_quantity))}` : "-"}</TableCell></TableRow>)}
            </TableBody></Table>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
