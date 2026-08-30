"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Eye, FileClock, Loader2, LockKeyhole, RefreshCw, ShieldCheck } from "lucide-react";
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
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Row = Record<string, unknown>;

type ReconciliationItem = {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  quantity: number;
};

type ReconciliationInvoice = {
  id: string;
  branch_name: string;
  invoice_number: string;
  customer_name: string;
  created_at: string;
  total_amount: number;
  items: ReconciliationItem[];
};

type InvoiceGroup = { invoice_count: number; revenue: number };

type Overview = {
  original_revenue: number;
  suppressed_revenue: number;
  base_revenue: number;
  invoice_count: number;
  suppressed_invoice_count: number;
  invoice_groups: {
    cash_suppressed: InvoiceGroup;
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

type Preview = {
  original_revenue: number;
  suppressed_revenue: number;
  final_revenue: number;
  suppression_candidates: ReconciliationInvoice[];
  suppressed_invoice_count: number;
  stock_projection: StockProjection;
};

type HistoryItem = {
  id: string;
  reconciliation_number: string;
  period_start: string;
  period_end: string;
  original_revenue: number;
  suppressed_revenue: number;
  final_revenue: number;
  suppressed_invoice_count: number;
  finalized_by_name: string;
  finalized_at: string;
};

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

const logNames: Record<string, string> = {
  invoice_suppressed: "ซ่อนใบขาย",
  invoice_renumbered: "เรียงเลขใบขายใหม่",
  price_adjusted: "ปรับราคา (รอบเดิม)",
  stock_reversed: "ย้อนการตัด Real เดิม",
  stock_received: "WH รับ Real คืน",
  stock_deducted: "ส่งคืน Real / ตัด Ghost",
  stock_deficit: "บันทึก Ghost deficit"
};

function GroupRow({ group, label }: { group: InvoiceGroup; label: string }) {
  return (
    <div className="flex justify-between gap-3 border-b py-1.5 last:border-0">
      <span>{label}</span>
      <span className="shrink-0 font-medium tabular-nums">{group.invoice_count.toLocaleString("th-TH")} ใบ · {currency(group.revenue)}</span>
    </div>
  );
}

export function MonthEndReconciliationConsole({ branches }: { branches: Row[] }) {
  const sellingBranches = useMemo(
    () => branches.filter((branch) => Boolean(branch.active ?? true) && Boolean(branch.sales_enabled ?? true) && text(branch.branch_type) !== "main_warehouse"),
    [branches]
  );
  const [dateFrom, setDateFrom] = useState(monthStartISO());
  const [dateTo, setDateTo] = useState(todayISO());
  const [branchIDs, setBranchIDs] = useState<string[]>(sellingBranches.map((branch) => text(branch.id)));
  const [overview, setOverview] = useState<Overview | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [audit, setAudit] = useState<Row | null>(null);
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
      target_revenue: 0,
      adjustment_percent: 0
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
    setLoading(true);
    try {
      const request = { method: "POST", body: JSON.stringify(payload()) };
      const [nextOverview, nextPreview] = await Promise.all([
        proxyClient<Overview>("/accounting/month-end/reconciliation-overview", request),
        proxyClient<Preview>("/accounting/month-end/reconciliation-preview", request)
      ]);
      setOverview(nextOverview);
      setPreview(nextPreview);
      toast.success("ตรวจสอบใบขายและ projection สต๊อกล่าสุดแล้ว");
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
        description="เฉพาะผู้ดูแลระบบสูงสุด · เลือกช่วงวันที่ ซ่อนบิลเงินสดที่เข้าเงื่อนไข ส่งคืน Real ไป WH และตัด Ghost ภายในธุรกรรมเดียว"
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
          <Field hint="Legacy · ไม่มีผลกับรอบใหม่" label="ยอดขายเป้าหมาย">
            <Input disabled placeholder="ไม่ใช้ในรอบใหม่" value="" />
          </Field>
          <Field hint="Legacy · ไม่มีผลกับรอบใหม่" label="เปอร์เซ็นต์ปรับราคา">
            <Input disabled placeholder="ไม่ใช้ในรอบใหม่" value="" />
          </Field>
        </div>
        <div className="mt-5 flex justify-end">
          <Button disabled={loading} onClick={() => void loadPlan()}>
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            ตรวจสอบใบขายและสต๊อก
          </Button>
        </div>
      </SectionCard>

      {overview && preview ? (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg border bg-info-50 p-4"><p className="text-xs text-muted-foreground">ยอดขาย issued + paid ทั้งหมด</p><p className="mt-1 text-xl font-semibold">{currency(overview.original_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">{overview.invoice_count.toLocaleString("th-TH")} ใบในขอบเขต</p></div>
            <div className="rounded-lg border bg-warning-50 p-4"><p className="text-xs text-muted-foreground">ยอดบิลที่จะซ่อนทั้งหมด</p><p className="mt-1 text-xl font-semibold">{currency(overview.suppressed_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">{overview.suppressed_invoice_count.toLocaleString("th-TH")} ใบเงินสดล้วน ไม่ขอใบกำกับเต็มรูป</p></div>
            <div className="rounded-lg border bg-success-50 p-4"><p className="text-xs text-muted-foreground">ยอดขายหลังซ่อน</p><p className="mt-1 text-xl font-semibold">{currency(overview.base_revenue)}</p><p className="mt-1 text-xs text-muted-foreground">สูตร: ยอดทั้งหมด − ยอดบิลที่จะซ่อน</p></div>
            <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">Ghost deficit ที่คาดว่าจะเกิด</p><p className="mt-1 text-xl font-semibold">{preview.stock_projection.ghost_deficit_created.toLocaleString("th-TH")} ชิ้น</p><p className="mt-1 text-xs text-muted-foreground">สร้าง ledger อัตโนมัติเมื่อ Ghost Lots ไม่พอ</p></div>
          </div>

          <SectionCard title="รายละเอียดกลุ่มบิลและสูตรจากข้อมูลจริง" description="ค่าทั้งหมด fetch มาจากใบขายและการชำระเงินในขอบเขตเดียวกับรอบ">
            <div className="grid gap-5 lg:grid-cols-2">
              <div className="rounded-lg border p-4 text-sm">
                <p className="font-semibold">องค์ประกอบยอดขายทั้งหมด</p>
                <div className="mt-2 text-muted-foreground">
                  <GroupRow group={overview.invoice_groups.cash_suppressed} label="เงินสดล้วน ไม่ขอใบกำกับเต็มรูป" />
                  <GroupRow group={overview.invoice_groups.cash_full_tax} label="เงินสด ขอใบกำกับเต็มรูป" />
                  <GroupRow group={overview.invoice_groups.bank_transfer} label="โอนเงิน" />
                  <GroupRow group={overview.invoice_groups.mixed} label="ชำระผสม" />
                  {overview.invoice_groups.unclassified.invoice_count > 0 ? <GroupRow group={overview.invoice_groups.unclassified} label="ข้อมูลช่องทางชำระไม่ครบ" /> : null}
                </div>
                <p className="mt-3 text-xs text-muted-foreground">สูตร: ผลรวมยอดเต็มของทุกกลุ่ม = {currency(overview.original_revenue)}</p>
              </div>
              <div className="rounded-lg border p-4 text-sm">
                <p className="font-semibold">กติกาของรอบใหม่</p>
                <p className="mt-2 text-muted-foreground">ซ่อนทุกบิลในกลุ่มเงินสดล้วนที่ไม่ขอใบกำกับภาษีเต็มรูป ไม่มีการเลือกบางบิลและไม่มีการปรับราคา</p>
                <p className="mt-3 tabular-nums">{currency(overview.original_revenue)} − {currency(overview.suppressed_revenue)} = <strong>{currency(overview.base_revenue)}</strong></p>
              </div>
            </div>
          </SectionCard>

          <SectionCard title="Projection การเคลื่อนไหวสต๊อก" description="แหล่งตัดของบิลเป็น Ghost หนึ่งค่า ส่วนการย้อนและส่งคืน Real เป็น adjustment แยก">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {[
                ["Branch Real returned", preview.stock_projection.branch_real_returned],
                ["WH Real received", preview.stock_projection.warehouse_real_received],
                ["WH Ghost deducted", preview.stock_projection.warehouse_ghost_deducted],
                ["Ghost deficit", preview.stock_projection.ghost_deficit_created]
              ].map(([label, value]) => <div className="rounded-lg border p-4" key={String(label)}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-xl font-semibold">{Number(value).toLocaleString("th-TH")} ชิ้น</p></div>)}
            </div>
            <TableContainer className="mt-4">
              <Table>
                <TableHeader><TableRow><TableHead>สินค้า</TableHead><TableHead className="text-right">จำนวนบิล</TableHead><TableHead className="text-right">WH Ghost ก่อน</TableHead><TableHead className="text-right">WH Ghost หลัง</TableHead><TableHead className="text-right">Deficit ใหม่</TableHead></TableRow></TableHeader>
                <TableBody>
                  {preview.stock_projection.products.map((product) => <TableRow key={product.product_id}><TableCell>{product.product_name}</TableCell><TableCell className="text-right">{product.quantity.toLocaleString("th-TH")}</TableCell><TableCell className="text-right">{product.warehouse_ghost_before.toLocaleString("th-TH")}</TableCell><TableCell className="text-right">{product.warehouse_ghost_after.toLocaleString("th-TH")}</TableCell><TableCell className="text-right">{product.deficit_created.toLocaleString("th-TH")}</TableCell></TableRow>)}
                </TableBody>
              </Table>
            </TableContainer>
          </SectionCard>

          <SectionCard title={`ใบขายที่จะซ่อนทั้งหมด ${preview.suppressed_invoice_count.toLocaleString("th-TH")} ใบ`} description="เรียงตาม created_at; หลังซ่อนจะเรียงเลขใบขาย Active ที่เหลือใหม่แยกสาขา">
            <TableContainer>
              <Table>
                <TableHeader><TableRow><TableHead>เลขบิล</TableHead><TableHead>สาขา</TableHead><TableHead>วันเวลา created_at</TableHead><TableHead>สินค้า</TableHead><TableHead className="text-right">ยอดเต็ม</TableHead><TableHead>แหล่งตัดหลังสรุป</TableHead></TableRow></TableHeader>
                <TableBody>
                  {preview.suppression_candidates.length === 0 ? <TableRow><TableCell className="py-10 text-center text-muted-foreground" colSpan={6}>ไม่มีใบขายที่เข้าเงื่อนไข</TableCell></TableRow> : preview.suppression_candidates.map((invoice) => <TableRow key={invoice.id}><TableCell className="font-medium">{invoice.invoice_number}</TableCell><TableCell>{invoice.branch_name}</TableCell><TableCell className="whitespace-nowrap">{dateTime(invoice.created_at)}</TableCell><TableCell>{invoice.items.map((item) => `${item.product_name} × ${item.quantity}`).join(", ")}</TableCell><TableCell className="text-right">{currency(invoice.total_amount)}</TableCell><TableCell>Ghost Stock (สต๊อกผี)</TableCell></TableRow>)}
                </TableBody>
              </Table>
            </TableContainer>
          </SectionCard>

          <div className="flex flex-col gap-4 rounded-lg border border-warning-200 bg-warning-50 p-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex gap-3"><AlertTriangle className="mt-0.5 h-5 w-5 text-warning-700" /><div><p className="font-semibold">แจ้งเพื่อทราบก่อนยืนยัน</p><p className="mt-1 text-sm text-muted-foreground">ระบบจะซ่อน {preview.suppressed_invoice_count.toLocaleString("th-TH")} ใบ และบันทึก deficit {preview.stock_projection.ghost_deficit_created.toLocaleString("th-TH")} ชิ้น ปุ่มยืนยันใช้งานได้แม้ Ghost ไม่พอ</p></div></div>
            <Button disabled={loading} onClick={() => setConfirmOpen(true)}><LockKeyhole className="h-4 w-4" />ยืนยันและสรุปรอบ</Button>
          </div>
        </>
      ) : null}

      <SectionCard title="ประวัติการสรุป" description="Superadmin เห็นช่วงวันที่และเวลาเต็ม พร้อม Audit Log ทุกขั้น">
        <TableContainer>
          <Table>
            <TableHeader><TableRow><TableHead>เลขรายการ</TableHead><TableHead>ช่วงวันที่</TableHead><TableHead className="text-right">ยอดเดิม</TableHead><TableHead className="text-right">ยอดซ่อน</TableHead><TableHead className="text-right">ยอดหลังสรุป</TableHead><TableHead>ผู้ยืนยัน / เวลา</TableHead><TableHead /></TableRow></TableHeader>
            <TableBody>
              {history.length === 0 ? <TableRow><TableCell className="py-10 text-center text-muted-foreground" colSpan={7}>ยังไม่มีประวัติการสรุป</TableCell></TableRow> : history.map((item) => <TableRow key={item.id}><TableCell className="font-medium">{item.reconciliation_number}</TableCell><TableCell>{item.period_start} ถึง {item.period_end}</TableCell><TableCell className="text-right">{currency(item.original_revenue)}</TableCell><TableCell className="text-right">{currency(item.suppressed_revenue)}</TableCell><TableCell className="text-right font-semibold">{currency(item.final_revenue)}</TableCell><TableCell><p>{item.finalized_by_name}</p><p className="text-xs text-muted-foreground">{dateTime(item.finalized_at)}</p></TableCell><TableCell className="text-right"><Button onClick={() => void openAudit(item.id)} variant="secondary"><Eye className="h-4 w-4" />Audit</Button></TableCell></TableRow>)}
            </TableBody>
          </Table>
        </TableContainer>
      </SectionCard>

      <Dialog onOpenChange={setConfirmOpen} open={confirmOpen}>
        <DialogContent>
          <DialogHeader title="ยืนยันการสรุปรอบ" description="ธุรกรรมนี้ซ่อนใบขาย ส่ง Real กลับ WH รับ Real เข้า WH ตัด Ghost และเรียงเลขบิล Active ใหม่" />
          <div className="rounded-lg border border-warning-200 bg-warning-50 p-4 text-sm"><p className="font-semibold">ผลที่จะบันทึก</p><p className="mt-2">{selectedBranches.map((branch) => text(branch.name)).join(", ")}</p><p className="mt-1">{dateFrom} ถึง {dateTo} · ซ่อน {preview?.suppressed_invoice_count || 0} ใบ · deficit {preview?.stock_projection.ghost_deficit_created || 0} ชิ้น</p></div>
          <label className="mt-4 block space-y-2 text-sm"><span>พิมพ์ <strong>{closeText}</strong></span><Input autoFocus onChange={(event) => setConfirmation(event.target.value)} value={confirmation} /></label>
          <div className="mt-5 flex justify-end gap-2"><Button onClick={() => setConfirmOpen(false)} variant="secondary">ยกเลิก</Button><Button disabled={confirmation !== closeText || loading} onClick={() => void finalize()}>{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}ยืนยันธุรกรรม</Button></div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { if (!open) setAudit(null); }} open={Boolean(audit)}>
        <DialogContent className="max-w-5xl">
          <DialogHeader title={`Audit Log · ${text(audit?.reconciliation_number)}`} description={`${text(audit?.period_start)} ถึง ${text(audit?.period_end)} · ${text(audit?.finalized_by_name)} · ${audit?.finalized_at ? dateTime(text(audit.finalized_at)) : ""}`} />
          <div className="mt-4 max-h-[60vh] overflow-auto rounded-lg border">
            <Table><TableHeader><TableRow><TableHead>เวลา</TableHead><TableHead>เหตุการณ์</TableHead><TableHead>เลขบิล</TableHead><TableHead>สินค้า</TableHead><TableHead>บทบาท movement</TableHead><TableHead>จำนวน</TableHead></TableRow></TableHeader><TableBody>
              {(audit?.logs as Row[] | undefined || []).map((log) => <TableRow key={text(log.id)}><TableCell className="whitespace-nowrap">{dateTime(text(log.created_at))}</TableCell><TableCell><span className="inline-flex items-center gap-1 font-medium"><FileClock className="h-4 w-4" />{logNames[text(log.log_type)] || text(log.log_type)}</span></TableCell><TableCell><p>{text(log.original_invoice_number) || "-"}</p>{log.new_invoice_number ? <p className="text-xs text-muted-foreground">→ {text(log.new_invoice_number)}</p> : null}</TableCell><TableCell>{text(log.product_name) || "-"}</TableCell><TableCell>{text(log.movement_role) || "-"}</TableCell><TableCell>{text(log.stock_bucket) ? `${text(log.stock_bucket) === "ghost" ? "Ghost" : "Real"} ${num(log.stock_quantity).toLocaleString("th-TH")}` : "-"}</TableCell></TableRow>)}
            </TableBody></Table>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
