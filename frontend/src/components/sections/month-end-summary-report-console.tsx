"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { SectionCard } from "@/components/sections/common";
import { Button, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Branch = { id: string; name: string };
type Reconciliation = { id: string; reconciliation_number: string; period_start: string; period_end: string; finalized_at: string };

type Movement = {
  role: string;
  branch_id: string;
  branch_name: string;
  stock_type: string;
  quantity: number;
  reason: string;
};

type ReportRow = {
  reconciliation_id: string;
  branch_id: string;
  branch_name: string;
  invoice_id: string;
  invoice_item_id: string;
  original_invoice_no: string;
  current_invoice_no: string | null;
  product_name: string;
  quantity: number;
  original_price: number;
  adjusted_price: number | null;
  discount_amount: number;
  payment_method: "cash" | "bank_transfer" | "mixed" | "unpaid";
  status: "active" | "hidden" | "adjusted";
  stock_deduction_source: "real" | "ghost" | "none";
  movements: Movement[];
};

type Report = {
  reconciliation_id?: string;
  reconciliation_ids: string[];
  period?: string;
  date_from: string;
  date_to: string;
  branch_id?: string;
  summary: {
    active_invoices_before: number;
    active_invoices_after: number;
    revenue_before: number;
    revenue_after: number;
    real_quantity_deducted: number;
    ghost_quantity_deducted: number;
    branch_real_returned: number;
    warehouse_real_received: number;
    ghost_deficit_quantity: number;
  };
  rows: ReportRow[];
  pagination: { page: number; page_size: number; total: number; total_pages: number };
};

const paymentLabels: Record<ReportRow["payment_method"], string> = {
  cash: "เงินสด",
  bank_transfer: "โอนเงิน",
  mixed: "ผสม",
  unpaid: "ยังไม่ชำระ"
};

const statusLabels: Record<ReportRow["status"], string> = {
  active: "Active",
  hidden: "Hidden/Deleted",
  adjusted: "Adjusted"
};

const movementLabels: Record<string, string> = {
  sale_source_reversal: "ย้อน Real deduction เดิม (internal)",
  branch_return_dispatch: "สาขาส่งคืน Real ไป WH",
  warehouse_return_receive: "WH รับ Real คืน",
  invoice_ghost_source: "กำหนด Ghost เป็นแหล่งตัดของบิล",
  warehouse_ghost_deficit: "บันทึก Ghost deficit"
};

function stockSource(value: ReportRow["stock_deduction_source"]) {
  if (value === "real") return "Real Stock (สต๊อกจริง)";
  if (value === "ghost") return "Ghost Stock (สต๊อกผี)";
  return "None";
}

function dateTime(value: string) {
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(new Date(value));
}

export function MonthEndSummaryReportConsole({ branches, reconciliations }: { branches: Branch[]; reconciliations: Reconciliation[] }) {
  const [reconciliationID, setReconciliationID] = useState(reconciliations[0]?.id || "");
  const selectedReconciliation = useMemo(() => reconciliations.find((item) => item.id === reconciliationID), [reconciliationID, reconciliations]);
  const [dateFrom, setDateFrom] = useState(reconciliations[0]?.period_start || "");
  const [dateTo, setDateTo] = useState(reconciliations[0]?.period_end || "");
  const [branchID, setBranchID] = useState("");
  const [report, setReport] = useState<Report | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (selectedReconciliation) {
      setDateFrom(selectedReconciliation.period_start);
      setDateTo(selectedReconciliation.period_end);
    }
  }, [selectedReconciliation]);

  const loadReport = useCallback(async (nextPage = 1) => {
    if (!reconciliationID && (!dateFrom || !dateTo)) return;
    setLoading(true);
    try {
      const query = new URLSearchParams({ page: String(nextPage), page_size: "50" });
      if (reconciliationID) query.set("reconciliation_id", reconciliationID);
      else {
        query.set("date_from", dateFrom);
        query.set("date_to", dateTo);
      }
      if (branchID) query.set("branch_id", branchID);
      const response = await proxyClient<Report>(`/admin/month-end-report?${query.toString()}`);
      setReport(response);
      setExpanded(null);
    } catch (error) {
      setReport(null);
      toast.error(error instanceof Error ? error.message : "โหลดรายงานสรุปสิ้นเดือนไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }, [branchID, dateFrom, dateTo, reconciliationID]);

  useEffect(() => {
    void loadReport(1);
  }, [loadReport]);

  if (reconciliations.length === 0) {
    return <SectionCard title="ยังไม่มีรอบสรุปสิ้นเดือน" description="เมื่อยืนยันการสรุปแล้ว รายงาน Before/After และ movement จะปรากฏที่นี่"><div /></SectionCard>;
  }

  const summary = report?.summary;
  const invoiceDifference = (summary?.active_invoices_before || 0) - (summary?.active_invoices_after || 0);
  const revenueDifference = (summary?.revenue_before || 0) - (summary?.revenue_after || 0);

  return (
    <div className="space-y-6">
      <SectionCard title="ตัวกรองรายงาน" description="เลือก reconciliation โดยตรงเป็นหลัก หรือเลือกค้นด้วยช่วงวันที่">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-[1.5fr_1fr_1fr_1fr_auto] xl:items-end">
          <label className="space-y-2 text-sm font-medium"><span>รอบ reconciliation</span><select className="h-10 w-full rounded-md border bg-white px-3 text-sm" onChange={(event) => setReconciliationID(event.target.value)} value={reconciliationID}><option value="">ค้นด้วยช่วงวันที่</option>{reconciliations.map((item) => <option key={item.id} value={item.id}>{item.reconciliation_number} · {item.period_start} ถึง {item.period_end}</option>)}</select></label>
          <label className="space-y-2 text-sm font-medium"><span>วันที่เริ่มต้น</span><input className="h-10 w-full rounded-md border bg-white px-3 text-sm" disabled={Boolean(reconciliationID)} onChange={(event) => setDateFrom(event.target.value)} type="date" value={dateFrom} /></label>
          <label className="space-y-2 text-sm font-medium"><span>วันที่สิ้นสุด</span><input className="h-10 w-full rounded-md border bg-white px-3 text-sm" disabled={Boolean(reconciliationID)} min={dateFrom} onChange={(event) => setDateTo(event.target.value)} type="date" value={dateTo} /></label>
          <label className="space-y-2 text-sm font-medium"><span>สาขาขาย</span><select className="h-10 w-full rounded-md border bg-white px-3 text-sm" onChange={(event) => setBranchID(event.target.value)} value={branchID}><option value="">ทุกสาขาในรอบ</option>{branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}</select></label>
          <Button disabled={loading} onClick={() => void loadReport(1)}>{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}โหลดรายงาน</Button>
        </div>
        {selectedReconciliation ? <p className="mt-3 text-xs text-muted-foreground">ยืนยันเมื่อ {dateTime(selectedReconciliation.finalized_at)}</p> : null}
      </SectionCard>

      {summary ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">ใบขาย Active ก่อน → หลัง</p><p className="mt-1 text-xl font-semibold">{summary.active_invoices_before.toLocaleString("th-TH")} → {summary.active_invoices_after.toLocaleString("th-TH")}</p><p className="mt-1 text-xs text-muted-foreground">ซ่อน {invoiceDifference.toLocaleString("th-TH")} ใบ</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">รายได้ก่อน → หลัง</p><p className="mt-1 text-xl font-semibold">{currency(summary.revenue_before)} → {currency(summary.revenue_after)}</p><p className="mt-1 text-xs text-muted-foreground">ลดลง {currency(revenueDifference)}</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">Branch Real returned</p><p className="mt-1 text-xl font-semibold">{summary.branch_real_returned.toLocaleString("th-TH")} ชิ้น</p><p className="mt-1 text-xs text-muted-foreground">การส่งคืนแยกจากแหล่งตัดของบิล</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">WH Real received</p><p className="mt-1 text-xl font-semibold">{summary.warehouse_real_received.toLocaleString("th-TH")} ชิ้น</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">WH Ghost deducted</p><p className="mt-1 text-xl font-semibold">{summary.ghost_quantity_deducted.toLocaleString("th-TH")} ชิ้น</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">Ghost deficit</p><p className="mt-1 text-xl font-semibold">{summary.ghost_deficit_quantity.toLocaleString("th-TH")} ชิ้น</p></div>
        </div>
      ) : null}

      <SectionCard title="เปรียบเทียบใบขายและสินค้า Before / After" description={report ? `${report.date_from} ถึง ${report.date_to} · ${report.pagination.total.toLocaleString("th-TH")} รายการสินค้า` : "กำลังโหลดข้อมูล"}>
        <TableContainer>
          <Table>
            <TableHeader><TableRow><TableHead /><TableHead>สาขา</TableHead><TableHead>Original Invoice No.</TableHead><TableHead>Current Invoice No.</TableHead><TableHead>สินค้า</TableHead><TableHead className="text-right">จำนวน</TableHead><TableHead className="text-right">ราคาเดิม</TableHead><TableHead className="text-right">ราคาหลังปรับ</TableHead><TableHead className="text-right">ส่วนลด</TableHead><TableHead>การชำระ</TableHead><TableHead>สถานะ</TableHead><TableHead>แหล่งตัดสต๊อก</TableHead></TableRow></TableHeader>
            <TableBody>
              {loading && !report ? <TableRow><TableCell className="py-12 text-center" colSpan={12}><Loader2 className="mx-auto h-5 w-5 animate-spin" /></TableCell></TableRow> : null}
              {!loading && report?.rows.length === 0 ? <TableRow><TableCell className="py-12 text-center text-muted-foreground" colSpan={12}>ไม่พบรายการในรอบและสาขาที่เลือก</TableCell></TableRow> : null}
              {report?.rows.map((row) => {
                const key = `${row.reconciliation_id}:${row.invoice_item_id}`;
                const isExpanded = expanded === key;
                return [
                  <TableRow key={key}>
                    <TableCell><button aria-label="ดู movement" className="rounded p-1 hover:bg-muted" onClick={() => setExpanded(isExpanded ? null : key)} type="button">{isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}</button></TableCell>
                    <TableCell className="whitespace-nowrap">{row.branch_name}</TableCell><TableCell className="whitespace-nowrap font-medium">{row.original_invoice_no}</TableCell><TableCell className="whitespace-nowrap">{row.current_invoice_no || "—"}</TableCell><TableCell className="min-w-48">{row.product_name}</TableCell><TableCell className="text-right">{row.quantity.toLocaleString("th-TH")}</TableCell><TableCell className="whitespace-nowrap text-right">{currency(row.original_price)}</TableCell><TableCell className="whitespace-nowrap text-right">{row.adjusted_price == null ? "N/A" : currency(row.adjusted_price)}</TableCell><TableCell className="whitespace-nowrap text-right">{currency(row.discount_amount)}</TableCell><TableCell>{paymentLabels[row.payment_method]}</TableCell><TableCell><span className={`whitespace-nowrap rounded-full px-2 py-1 text-xs font-semibold ${row.status === "hidden" ? "bg-red-100 text-red-700" : row.status === "adjusted" ? "bg-amber-100 text-amber-800" : "bg-emerald-100 text-emerald-700"}`}>{statusLabels[row.status]}</span></TableCell><TableCell className="min-w-48">{stockSource(row.stock_deduction_source)}</TableCell>
                  </TableRow>,
                  isExpanded ? <TableRow key={`${key}:movements`}><TableCell colSpan={12}><div className="m-2 rounded-lg border bg-muted/30 p-4"><p className="text-sm font-semibold">Movement details</p>{row.movements.length === 0 ? <p className="mt-2 text-sm text-muted-foreground">ไม่มี movement เพิ่มเติมในรอบนี้</p> : <div className="mt-2 grid gap-2 lg:grid-cols-2">{row.movements.map((movement, index) => <div className="rounded-md border bg-white p-3 text-sm" key={`${movement.role}:${index}`}><p className="font-medium">{movementLabels[movement.role] || movement.role}</p><p className="mt-1 text-muted-foreground">{movement.branch_name} · {movement.stock_type || "-"} · <span className={movement.quantity < 0 ? "text-red-700" : "text-emerald-700"}>{movement.quantity > 0 ? "+" : ""}{movement.quantity.toLocaleString("th-TH")}</span></p>{movement.reason ? <p className="mt-1 text-xs text-muted-foreground">Reason: {movement.reason}</p> : null}</div>)}</div>}</div></TableCell></TableRow> : null
                ];
              })}
            </TableBody>
          </Table>
        </TableContainer>
        {report ? <div className="mt-4 flex items-center justify-between text-sm"><span className="text-muted-foreground">หน้า {report.pagination.page.toLocaleString("th-TH")} จาก {report.pagination.total_pages.toLocaleString("th-TH")}</span><div className="flex gap-2"><Button disabled={loading || report.pagination.page <= 1} onClick={() => void loadReport(report.pagination.page - 1)} variant="secondary"><ChevronLeft className="h-4 w-4" />ก่อนหน้า</Button><Button disabled={loading || report.pagination.page >= report.pagination.total_pages} onClick={() => void loadReport(report.pagination.page + 1)} variant="secondary">ถัดไป<ChevronRight className="h-4 w-4" /></Button></div></div> : null}
      </SectionCard>
    </div>
  );
}
