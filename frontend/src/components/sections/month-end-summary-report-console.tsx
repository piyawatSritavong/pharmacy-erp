"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { SectionCard } from "@/components/sections/common";
import { Button, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { byPeriodDesc, groupByPeriod, periodLabel, shortDate } from "@/lib/month-end-groups";
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

type InvoiceGroup = {
  invoiceID: string;
  branchName: string;
  originalNo: string;
  currentNo: string | null;
  paymentMethod: ReportRow["payment_method"];
  status: ReportRow["status"];
  discountTotal: number;
  items: ReportRow[];
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
  sale_source_reversal: "คืนสต๊อกที่เคยตัดตอนขาย",
  branch_return_dispatch: "สาขาส่งของกลับโกดัง",
  warehouse_return_receive: "โกดังรับของคืน",
  invoice_ghost_source: "ตัดสต๊อกผีแทน",
  warehouse_ghost_deficit: "Ghost ไม่พอ บันทึกส่วนขาด"
};

// The stock steps of a hidden bill run in a fixed order; the report groups them
// per product and reads them top to bottom as a timeline.
const MOVEMENT_STEPS: Record<string, { order: number; title: string; detail: string }> = {
  sale_source_reversal: { order: 1, title: "คืนสต๊อกที่เคยตัดตอนขาย", detail: "ยกเลิกการตัด Real ที่สาขาเมื่อตอนออกบิล (รายการภายใน)" },
  branch_return_dispatch: { order: 2, title: "สาขาส่งของกลับโกดัง", detail: "ตัด Real ออกจากสาขา ส่งคืนไปยังโกดังกลาง" },
  warehouse_return_receive: { order: 3, title: "โกดังรับของคืน", detail: "รับ Real เข้าโกดัง" },
  invoice_ghost_source: { order: 4, title: "ตัดสต๊อกผีแทน", detail: "หักออกจาก Ghost ที่โกดัง เป็นแหล่งตัดจริงของบิลนี้" },
  warehouse_ghost_deficit: { order: 5, title: "Ghost ไม่พอ บันทึกส่วนขาด", detail: "Ghost หมด บันทึกส่วนที่ขาดไว้ใน deficit ledger" }
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
  // Open on the newest period, not the newest confirmation: closing an old
  // month today should not hide the current one behind an empty report.
  // The Audit button on ประวัติการสรุป links here with ?reconciliation_id=…, so
  // honour that round when it names a real one, otherwise fall back to newest.
  const params = useSearchParams();
  const requestedID = params.get("reconciliation_id");
  const newest = byPeriodDesc(reconciliations)[0];
  const initialRound = (requestedID && reconciliations.find((item) => item.id === requestedID)) || newest;
  const [reconciliationID, setReconciliationID] = useState(initialRound?.id || "");
  const selectedReconciliation = useMemo(() => reconciliations.find((item) => item.id === reconciliationID), [reconciliationID, reconciliations]);
  const [dateFrom, setDateFrom] = useState(initialRound?.period_start || "");
  const [dateTo, setDateTo] = useState(initialRound?.period_end || "");
  const [branchID, setBranchID] = useState("");
  const [report, setReport] = useState<Report | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [foldedRounds, setFoldedRounds] = useState<Record<string, boolean>>({});
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
      setFoldedRounds({});
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

  // The report reads per item, but a bill is the unit people recognise: fold
  // items into their invoice, and invoices into the round that closed them, so
  // a year of closes stays navigable and one bill reads as one line.
  const roundGroups = useMemo(() => {
    const meta = new Map(reconciliations.map((item) => [item.id, item]));
    const rounds = new Map<string, { id: string; label: string; period: string; invoices: InvoiceGroup[]; byInvoice: Map<string, InvoiceGroup> }>();
    for (const row of report?.rows || []) {
      if (!rounds.has(row.reconciliation_id)) {
        const round = meta.get(row.reconciliation_id);
        rounds.set(row.reconciliation_id, {
          id: row.reconciliation_id,
          label: round?.reconciliation_number || row.reconciliation_id,
          period: round ? `${periodLabel(round.period_start)} · ${shortDate(round.period_start)} ถึง ${shortDate(round.period_end)}` : "",
          invoices: [],
          byInvoice: new Map()
        });
      }
      const group = rounds.get(row.reconciliation_id)!;
      let invoice = group.byInvoice.get(row.invoice_id);
      if (!invoice) {
        invoice = {
          invoiceID: row.invoice_id,
          branchName: row.branch_name,
          originalNo: row.original_invoice_no,
          currentNo: row.current_invoice_no,
          paymentMethod: row.payment_method,
          status: row.status,
          discountTotal: 0,
          items: []
        };
        group.byInvoice.set(row.invoice_id, invoice);
        group.invoices.push(invoice);
      }
      invoice.items.push(row);
      invoice.discountTotal += row.discount_amount;
      // hidden beats adjusted beats active: the strongest outcome names the bill.
      if (row.status === "hidden" || (row.status === "adjusted" && invoice.status === "active")) invoice.status = row.status;
    }
    return [...rounds.values()];
  }, [reconciliations, report]);

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
          <label className="space-y-2 text-sm font-medium"><span>รอบ reconciliation</span><select className="h-10 w-full rounded-md border bg-white px-3 text-sm" onChange={(event) => setReconciliationID(event.target.value)} value={reconciliationID}><option value="">ค้นด้วยช่วงวันที่</option>{groupByPeriod(reconciliations).map((bucket) => <optgroup key={bucket.key} label={bucket.label}>{bucket.rounds.map((item) => <option key={item.id} value={item.id}>{item.reconciliation_number} · {shortDate(item.period_start)} ถึง {shortDate(item.period_end)}</option>)}</optgroup>)}</select></label>
          <label className="space-y-2 text-sm font-medium"><span>วันที่เริ่มต้น</span><input className="h-10 w-full rounded-md border bg-white px-3 text-sm" disabled={Boolean(reconciliationID)} onChange={(event) => setDateFrom(event.target.value)} type="date" value={dateFrom} /></label>
          <label className="space-y-2 text-sm font-medium"><span>วันที่สิ้นสุด</span><input className="h-10 w-full rounded-md border bg-white px-3 text-sm" disabled={Boolean(reconciliationID)} min={dateFrom} onChange={(event) => setDateTo(event.target.value)} type="date" value={dateTo} /></label>
          <label className="space-y-2 text-sm font-medium"><span>สาขาขาย</span><select className="h-10 w-full rounded-md border bg-white px-3 text-sm" onChange={(event) => setBranchID(event.target.value)} value={branchID}><option value="">ทุกสาขาในรอบ</option>{branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}</select></label>
          <Button disabled={loading} onClick={() => void loadReport(1)}>{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}โหลดรายงาน</Button>
        </div>
        {selectedReconciliation ? <p className="mt-3 text-xs text-muted-foreground">ยืนยันเมื่อ {dateTime(selectedReconciliation.finalized_at)}</p> : null}
      </SectionCard>

      {summary ? (
        <div className="grid gap-3 lg:grid-cols-2">
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">ใบขาย Active ก่อน → หลัง</p><p className="mt-1 text-xl font-semibold">{summary.active_invoices_before.toLocaleString("th-TH")} → {summary.active_invoices_after.toLocaleString("th-TH")}</p><p className="mt-1 text-xs text-muted-foreground">ซ่อน {invoiceDifference.toLocaleString("th-TH")} ใบ</p></div>
          <div className="rounded-lg border bg-card p-4"><p className="text-xs text-muted-foreground">รายได้ก่อน → หลัง</p><p className="mt-1 text-xl font-semibold">{currency(summary.revenue_before)} → {currency(summary.revenue_after)}</p><p className="mt-1 text-xs text-muted-foreground">ลดลง {currency(revenueDifference)}</p></div>
          {/* Branch-returned, WH-received and Ghost-deducted are the SAME physical
              quantity seen at three points of one atomic transfer, so they are
              always equal; showing them as three cards read as three facts. This
              collapses them into one pipeline and only surfaces a deficit if the
              warehouse actually ran short of Ghost Stock. */}
          <div className="rounded-lg border bg-card p-4 lg:col-span-2">
            <p className="text-xs text-muted-foreground">สินค้าที่ส่งคืนโกดังและตัดจากสต๊อกผี</p>
            <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-stretch">
              {[
                ["สาขาส่งคืน Real", summary.branch_real_returned],
                ["โกดังรับ Real", summary.warehouse_real_received],
                ["ตัด Ghost ที่โกดัง", summary.ghost_quantity_deducted]
              ].map(([label, value], index) => (
                <div className="flex flex-1 items-center gap-3" key={String(label)}>
                  <div className="flex-1 rounded-lg bg-muted/50 px-3 py-2.5">
                    <p className="text-2xl font-semibold tabular-nums">{Number(value).toLocaleString("th-TH")}<span className="ml-1 text-sm font-normal text-muted-foreground">ชิ้น</span></p>
                    <p className="text-xs text-muted-foreground">{label}</p>
                  </div>
                  {index < 2 ? <span aria-hidden className="hidden text-lg text-muted-foreground sm:block">→</span> : null}
                </div>
              ))}
            </div>
            <p className="mt-3 text-xs text-muted-foreground">สินค้าชุดเดียวกันเดินผ่าน 3 ขั้นของการซ่อนบิล ทั้งสามค่าจึงเท่ากันเสมอ · {summary.ghost_deficit_quantity > 0 ? <span className="font-medium text-red-700">Ghost ไม่พอ {summary.ghost_deficit_quantity.toLocaleString("th-TH")} ชิ้น บันทึกเป็น deficit ledger</span> : "Ghost ที่โกดังเพียงพอทุกชิ้น ไม่มีส่วนขาด"}</p>
          </div>
        </div>
      ) : null}

      <SectionCard title="เปรียบเทียบใบขายและสินค้า Before / After" description={report ? `${report.date_from} ถึง ${report.date_to} · ${report.pagination.total.toLocaleString("th-TH")} ใบขาย · กดลูกศรเพื่อดูรายการในบิล` : "กำลังโหลดข้อมูล"}>
        <TableContainer>
          <Table>
            <TableHeader><TableRow><TableHead /><TableHead>สาขา</TableHead><TableHead>Original Invoice No.</TableHead><TableHead>Current Invoice No.</TableHead><TableHead className="text-right">รายการในบิล</TableHead><TableHead className="text-right">ส่วนลดรวมของบิล</TableHead><TableHead>การชำระ</TableHead><TableHead>สถานะ</TableHead></TableRow></TableHeader>
            <TableBody>
              {loading && !report ? <TableRow><TableCell className="py-12 text-center" colSpan={8}><Loader2 className="mx-auto h-5 w-5 animate-spin" /></TableCell></TableRow> : null}
              {!loading && report?.rows.length === 0 ? <TableRow><TableCell className="py-12 text-center text-muted-foreground" colSpan={8}>ไม่พบรายการในรอบและสาขาที่เลือก</TableCell></TableRow> : null}
              {roundGroups.flatMap((group, groupIndex) => {
                // A single round needs no fold; several rounds open the newest only.
                const folded = foldedRounds[group.id] ?? (roundGroups.length > 1 && groupIndex > 0);
                const header = (
                  <TableRow className="bg-muted/40" key={`group:${group.id}`}>
                    <TableCell colSpan={8}>
                      <button
                        aria-expanded={!folded}
                        className="flex w-full items-center gap-3 text-left"
                        onClick={() => setFoldedRounds((current) => ({ ...current, [group.id]: !folded }))}
                        type="button"
                      >
                        {folded ? <ChevronRight className="h-4 w-4 shrink-0" /> : <ChevronDown className="h-4 w-4 shrink-0" />}
                        <span className="min-w-0 flex-1">
                          <span className="block font-semibold">{group.label}</span>
                          {group.period ? <span className="block text-xs text-muted-foreground">{group.period}</span> : null}
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground">{group.invoices.length.toLocaleString("th-TH")} ใบ</span>
                      </button>
                    </TableCell>
                  </TableRow>
                );
                if (folded) return [header];
                return [header, ...group.invoices.flatMap((invoice) => {
                  const key = `${group.id}:${invoice.invoiceID}`;
                  const isExpanded = expanded === key;
                  return [
                    <TableRow key={key}>
                      <TableCell><button aria-label={`ดูรายการในบิล ${invoice.originalNo}`} className="rounded p-1 hover:bg-muted" onClick={() => setExpanded(isExpanded ? null : key)} type="button">{isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}</button></TableCell>
                      <TableCell className="whitespace-nowrap">{invoice.branchName}</TableCell>
                      <TableCell className="whitespace-nowrap font-medium">{invoice.originalNo}</TableCell>
                      <TableCell className="whitespace-nowrap">{invoice.currentNo || "—"}</TableCell>
                      <TableCell className="text-right tabular-nums">{invoice.items.length.toLocaleString("th-TH")}</TableCell>
                      <TableCell className="whitespace-nowrap text-right tabular-nums">{invoice.discountTotal > 0 ? currency(invoice.discountTotal) : "—"}</TableCell>
                      <TableCell>{paymentLabels[invoice.paymentMethod]}</TableCell>
                      <TableCell><span className={`whitespace-nowrap rounded-full px-2 py-1 text-xs font-semibold ${invoice.status === "hidden" ? "bg-red-100 text-red-700" : invoice.status === "adjusted" ? "bg-amber-100 text-amber-800" : "bg-emerald-100 text-emerald-700"}`}>{statusLabels[invoice.status]}</span></TableCell>
                    </TableRow>,
                    isExpanded ? (
                      <TableRow key={`${key}:items`}>
                        <TableCell colSpan={8}>
                          <div className="m-2 space-y-3 rounded-lg border bg-muted/30 p-4">
                            <p className="text-sm font-semibold">รายการในบิล {invoice.originalNo}</p>
                            <TableContainer className="bg-white">
                              <Table>
                                <TableHeader><TableRow><TableHead>สินค้า</TableHead><TableHead className="text-right">จำนวน</TableHead><TableHead className="text-right">ราคาเดิม/หน่วย</TableHead><TableHead className="text-right">ราคาหลังปรับ/หน่วย</TableHead><TableHead className="text-right">ส่วนลด</TableHead><TableHead>สถานะ</TableHead><TableHead>แหล่งตัดสต๊อก</TableHead></TableRow></TableHeader>
                                <TableBody>
                                  {invoice.items.map((item) => (
                                    <TableRow key={item.invoice_item_id}>
                                      <TableCell className="min-w-48">{item.product_name}</TableCell>
                                      <TableCell className="text-right tabular-nums">{item.quantity.toLocaleString("th-TH")}</TableCell>
                                      <TableCell className="whitespace-nowrap text-right tabular-nums">{currency(item.original_price)}</TableCell>
                                      <TableCell className="whitespace-nowrap text-right tabular-nums">{item.adjusted_price == null ? <span className="text-muted-foreground">ไม่ปรับลด</span> : currency(item.adjusted_price)}</TableCell>
                                      <TableCell className="whitespace-nowrap text-right tabular-nums">{item.discount_amount > 0 ? currency(item.discount_amount) : "—"}</TableCell>
                                      <TableCell><span className={`whitespace-nowrap rounded-full px-2 py-1 text-xs font-semibold ${item.status === "hidden" ? "bg-red-100 text-red-700" : item.status === "adjusted" ? "bg-amber-100 text-amber-800" : "bg-emerald-100 text-emerald-700"}`}>{statusLabels[item.status]}</span></TableCell>
                                      <TableCell className="min-w-40">{stockSource(item.stock_deduction_source)}</TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </TableContainer>
                            <div>
                              <p className="text-sm font-semibold">การเคลื่อนไหวสต๊อกของบิลนี้</p>
                              {invoice.items.every((item) => item.movements.length === 0) ? (
                                <p className="mt-2 text-sm text-muted-foreground">บิลนี้ไม่มีการเคลื่อนไหวสต๊อก — การบันทึกที่ต้นทุน + % เปลี่ยนเฉพาะราคา สต๊อกยังตัดตามเดิม</p>
                              ) : (
                                <div className="mt-3 space-y-4">
                                  {invoice.items.filter((item) => item.movements.length > 0).map((item) => {
                                    const steps = [...item.movements].sort((a, b) => (MOVEMENT_STEPS[a.role]?.order ?? 99) - (MOVEMENT_STEPS[b.role]?.order ?? 99));
                                    return (
                                      <div className="rounded-lg border bg-white p-4" key={item.invoice_item_id}>
                                        <div className="flex flex-wrap items-center gap-2 border-b pb-3">
                                          <span className="text-base font-semibold">{item.product_name}</span>
                                          <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">ตัดจาก {stockSource(item.stock_deduction_source)}</span>
                                        </div>
                                        <ol className="mt-3">
                                          {steps.map((movement, index) => {
                                            const step = MOVEMENT_STEPS[movement.role];
                                            const isGhost = (movement.stock_type || "").toUpperCase() === "GHOST";
                                            const last = index === steps.length - 1;
                                            return (
                                              <li className="relative flex gap-3 pb-5 last:pb-0" key={`${movement.role}:${index}`}>
                                                {last ? null : <span aria-hidden className="absolute left-[13px] top-7 h-[calc(100%-1rem)] w-px bg-border" />}
                                                <span className="z-10 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">{step?.order ?? index + 1}</span>
                                                <div className="min-w-0 flex-1">
                                                  <div className="flex flex-wrap items-center justify-between gap-2">
                                                    <p className="font-medium">{step?.title || movementLabels[movement.role] || movement.role}</p>
                                                    <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs">
                                                      <span className="text-muted-foreground">{movement.branch_name}</span>
                                                      <span className={`rounded px-1.5 py-0.5 font-medium ${isGhost ? "bg-violet-100 text-violet-700" : "bg-sky-100 text-sky-700"}`}>{isGhost ? "Ghost" : "Real"}</span>
                                                      <span className={`font-semibold tabular-nums ${movement.quantity < 0 ? "text-red-700" : "text-emerald-700"}`}>{movement.quantity > 0 ? "+" : ""}{movement.quantity.toLocaleString("th-TH")}</span>
                                                    </span>
                                                  </div>
                                                  {step?.detail ? <p className="mt-0.5 text-xs text-muted-foreground">{step.detail}</p> : null}
                                                </div>
                                              </li>
                                            );
                                          })}
                                        </ol>
                                      </div>
                                    );
                                  })}
                                </div>
                              )}
                            </div>
                          </div>
                        </TableCell>
                      </TableRow>
                    ) : null
                  ];
                })];
              })}
            </TableBody>
          </Table>
        </TableContainer>
        {report ? <div className="mt-4 flex items-center justify-between text-sm"><span className="text-muted-foreground">หน้า {report.pagination.page.toLocaleString("th-TH")} จาก {report.pagination.total_pages.toLocaleString("th-TH")}</span><div className="flex gap-2"><Button disabled={loading || report.pagination.page <= 1} onClick={() => void loadReport(report.pagination.page - 1)} variant="secondary"><ChevronLeft className="h-4 w-4" />ก่อนหน้า</Button><Button disabled={loading || report.pagination.page >= report.pagination.total_pages} onClick={() => void loadReport(report.pagination.page + 1)} variant="secondary">ถัดไป<ChevronRight className="h-4 w-4" /></Button></div></div> : null}
      </SectionCard>
    </div>
  );
}
