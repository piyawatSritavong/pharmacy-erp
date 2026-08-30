"use client";

import { Fragment, startTransition, useEffect, useMemo, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import {
  AlertTriangle,
  BarChart3,
  Calculator,
  CalendarRange,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  FileSearch,
  Loader2,
  LockKeyhole,
  PackageCheck,
  RotateCcw,
  Save,
  Search,
  ShieldCheck,
  Trash2
} from "lucide-react";
import { toast } from "sonner";

import { Field } from "@/components/ui/field";
import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  Input,
  Select,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  Textarea
} from "@/components/ui/primitives";
import { cn, currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Row = Record<string, unknown>;
type ApiItem = { item: Row; message: string };
type InvoiceGroup = Row & { lines: Row[]; item_count: number; invoice_total: number };

const steps = [
  { title: "เลือกรอบบัญชี", support: "กำหนดเดือนและขอบเขต", icon: CalendarRange },
  { title: "ตรวจสอบรายการขาย", support: "ตรวจความครบถ้วนของข้อมูล", icon: FileSearch },
  { title: "วิเคราะห์ส่วนต่าง", support: "กำหนดเป้าหมายและคำนวณ", icon: BarChart3 },
  { title: "ปรับปรุงสต๊อก", support: "เลือกข้อเสนอที่อนุญาต", icon: PackageCheck },
  { title: "ตรวจสอบผลลัพธ์", support: "ทบทวนก่อนส่งอนุมัติ", icon: ClipboardCheck },
  { title: "ยืนยันและปิดรอบ", support: "อนุมัติ ล็อก และบันทึก audit", icon: LockKeyhole }
];

const statusNames: Record<string, string> = {
  OPEN: "เปิดรอบ",
  DRAFT: "ฉบับร่าง",
  CALCULATING: "กำลังคำนวณ",
  PENDING_APPROVAL: "รออนุมัติ",
  APPROVED: "อนุมัติแล้ว",
  CLOSED: "ปิดรอบแล้ว",
  REOPENED: "เปิด revision ใหม่แล้ว",
  FAILED: "คำนวณไม่สำเร็จ",
  CANCELLED: "ยกเลิกร่างแล้ว"
};

const adjustmentNames: Record<string, string> = {
  included: "คงรายการต้นฉบับ",
  locked_full_tax: "ล็อกใบกำกับภาษีเต็มรูป",
  ghost_reclassification: "จำแนกไปสต๊อกรอง",
  price_scenario: "จำลองราคาต้นทุน + กำไร",
  ghost_and_price: "จำแนกสต๊อกและจำลองราคา"
};

function monthNow() {
  return new Intl.DateTimeFormat("en-CA", { month: "2-digit", timeZone: "Asia/Bangkok", year: "numeric" })
    .format(new Date())
    .slice(0, 7);
}

function text(value: unknown) {
  return value == null ? "" : String(value);
}

function num(value: unknown) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}

function paymentName(value: unknown) {
  return {
    cash: "เงินสด",
    bank_transfer: "เงินโอน",
    mixed: "เงินสด + เงินโอน",
    unpaid: "ยังไม่ครบ"
  }[text(value)] || "ยังไม่ครบ";
}

function formatDate(value: unknown) {
  if (!value) return "-";
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "short", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(new Date(String(value)));
}

export function MonthEndConsole({ branches, workpapers }: { branches: Row[]; workpapers: Row[] }) {
  const router = useRouter();
  const [period, setPeriod] = useState<Row | null>(null);
  const [activeStep, setActiveStep] = useState(1);
  const [month, setMonth] = useState(monthNow());
  const [branchId, setBranchId] = useState("");
  const [targetRevenue, setTargetRevenue] = useState("");
  const [markupPercent, setMarkupPercent] = useState("5.00");
  const [notes, setNotes] = useState("");
  const [simulationReason, setSimulationReason] = useState("");
  const [supportDocument, setSupportDocument] = useState("");
  const [strategy, setStrategy] = useState("closest_then_oldest");
  const [search, setSearch] = useState("");
  const [paymentFilter, setPaymentFilter] = useState("");
  const [taxFilter, setTaxFilter] = useState("");
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [closeDialog, setCloseDialog] = useState(false);
  const [reopenDialog, setReopenDialog] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [reopenReason, setReopenReason] = useState("");
  const [cancelTarget, setCancelTarget] = useState<Row | null>(null);
  const [cancelConfirmation, setCancelConfirmation] = useState("");
  const [cancelReason, setCancelReason] = useState("");
  const [cancelledIds, setCancelledIds] = useState<Set<string>>(new Set());
  const pageSize = 20;

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (dirty) event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  function applyPeriod(item: Row) {
    setPeriod(item);
    setMonth(text(item.period_start).slice(0, 7) || monthNow());
    setBranchId(text(item.branch_id));
    setTargetRevenue(text(item.target_revenue));
    setMarkupPercent(text(item.markup_percent) || "5.00");
    setNotes(text(item.notes));
    setSimulationReason(text(item.simulation_reason));
    setSupportDocument(text(item.support_document_ref));
    setStrategy(text(item.calculation_strategy) || "closest_then_oldest");
    setActiveStep(Math.max(1, Math.min(6, num(item.current_step) || 1)));
    setDirty(false);
    setSearch("");
    setPage(1);
  }

  async function request(path: string, init?: RequestInit) {
    setLoading(true);
    try {
      const response = await proxyClient<ApiItem>(path, init);
      applyPeriod(response.item);
      toast.success(response.message);
      startTransition(() => router.refresh());
      return response.item;
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "ดำเนินการไม่สำเร็จ");
      return null;
    } finally {
      setLoading(false);
    }
  }

  async function createPeriod() {
    const item = await request("/accounting/month-end/periods", {
      method: "POST",
      body: JSON.stringify({ month, branch_id: branchId, notes, calculation_strategy: strategy })
    });
    if (!item) return;
    setLoading(true);
    try {
      const validation = await proxyClient<{ issues: Row[] }>(`/accounting/month-end/periods/${text(item.id)}/validate`, { method: "POST", body: "{}" });
      setPeriod((current) => current ? { ...current, validation_summary: validation.issues, current_step: 3 } : current);
      setActiveStep(2);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "ตรวจสอบรอบบัญชีไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  async function openPeriod(id: string) {
    setLoading(true);
    try {
      applyPeriod(await proxyClient<Row>(`/accounting/month-end/periods/${id}`));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "เปิดรอบบัญชีไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  function draftPayload() {
    return {
      target_revenue: targetRevenue,
      markup_percent: markupPercent,
      notes,
      simulation_reason: simulationReason,
      support_document_ref: supportDocument,
      calculation_strategy: strategy,
      lock_version: num(period?.lock_version)
    };
  }

  async function saveDraft() {
    if (!period?.id) return;
    return request(`/accounting/month-end/periods/${text(period.id)}`, { method: "PATCH", body: JSON.stringify(draftPayload()) });
  }

  async function calculate() {
    if (!period?.id) return;
    if (!simulationReason.trim()) {
      toast.error("กรุณาระบุเหตุผลของข้อมูลจำลอง");
      return;
    }
    const saved = dirty ? await saveDraft() : period;
    if (!saved) return;
    const idempotencyKey = `${text(period.id)}-${Date.now()}-${targetRevenue}-${markupPercent}`;
    const item = await request(`/accounting/month-end/periods/${text(period.id)}/calculate`, {
      method: "POST",
      body: JSON.stringify({ idempotency_key: idempotencyKey })
    });
    if (item) setActiveStep(4);
  }

  async function toggleLine(line: Row, included: boolean) {
    if (!period?.id) return;
    await request(`/accounting/month-end/periods/${text(period.id)}/proposals/${text(line.id)}`, {
      method: "PATCH",
      body: JSON.stringify({ included, reason: included ? "เลือกใช้ข้อเสนอ" : "ไม่นำข้อเสนอนี้ไปใช้" })
    });
  }

  async function statusAction(action: "submit" | "approve") {
    if (!period?.id) return;
    const item = await request(`/accounting/month-end/periods/${text(period.id)}/${action}`, { method: "POST", body: "{}" });
    if (item) setActiveStep(6);
  }

  async function closePeriod() {
    if (!period?.id) return;
    const item = await request(`/accounting/month-end/periods/${text(period.id)}/close`, {
      method: "POST",
      body: JSON.stringify({ confirmation })
    });
    if (item) {
      setCloseDialog(false);
      setConfirmation("");
    }
  }

  async function reopenPeriod() {
    if (!period?.id) return;
    const item = await request(`/accounting/month-end/periods/${text(period.id)}/reopen`, {
      method: "POST",
      body: JSON.stringify({ reason: reopenReason })
    });
    if (item) {
      setReopenDialog(false);
      setReopenReason("");
    }
  }

  function openCancelDialog(item: Row) {
    setCancelTarget(item);
    setCancelConfirmation("");
    setCancelReason("");
  }

  async function cancelDraft() {
    if (!cancelTarget?.id) return;
    const id = text(cancelTarget.id);
    setLoading(true);
    try {
      const response = await proxyClient<ApiItem>(`/accounting/month-end/periods/${id}`, {
        method: "DELETE",
        body: JSON.stringify({
          confirmation: cancelConfirmation,
          reason: cancelReason,
          lock_version: num(cancelTarget.lock_version)
        })
      });
      setCancelledIds((current) => new Set(current).add(id));
      if (text(period?.id) === id) {
        setPeriod(null);
        setActiveStep(1);
      }
      setCancelTarget(null);
      setCancelConfirmation("");
      setCancelReason("");
      toast.success(response.message);
      startTransition(() => router.refresh());
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "ลบรอบบัญชีฉบับร่างไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  function exportSimulation() {
    if (!period) return;
    const lines = (period.transactions as Row[] | undefined) || [];
    const header = ["เลขใบขาย", "วันที่", "SKU", "สินค้า", "ยอดจริง", "ยอดจำลอง", "ประเภทข้อเสนอ"];
    const rows = lines.map((line) => [line.invoice_number, line.issued_at, line.sku, line.product_name, line.original_line_total, line.scenario_line_total, adjustmentNames[text(line.adjustment_type)]]);
    const csv = `\uFEFFข้อมูลจำลอง — ไม่ใช่รายงานทางบัญชีหรือภาษี\n${[header, ...rows].map((row) => row.map((cell) => `"${text(cell).replaceAll('"', '""')}"`).join(",")).join("\n")}`;
    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `month-end-simulation-${month}.csv`;
    anchor.click();
    URL.revokeObjectURL(url);
    toast.success("ส่งออกไฟล์สำหรับเปิดด้วย Excel แล้ว");
  }

  const allTransactions = useMemo(() => (period?.transactions as Row[] | undefined) || [], [period]);
  const invoices = useMemo(() => {
    const grouped = new Map<string, InvoiceGroup>();
    allTransactions.forEach((line) => {
      const id = text(line.invoice_id) || text(line.invoice_number);
      let invoice = grouped.get(id);
      if (!invoice) {
        invoice = { ...line, lines: [], item_count: 0, invoice_total: 0 };
        grouped.set(id, invoice);
      }
      invoice.lines.push(line);
      invoice.item_count += 1;
      invoice.invoice_total += num(line.original_gross_line_total || line.original_line_total);
    });
    return Array.from(grouped.values());
  }, [allTransactions]);
  const filteredInvoices = useMemo(() => {
    const query = search.trim().toLocaleLowerCase("th");
    return invoices.filter((invoice) => {
      const matchedHeader = [invoice.invoice_number, invoice.branch_name]
        .some((value) => text(value).toLocaleLowerCase("th").includes(query));
      const matchedItem = invoice.lines.some((line) =>
        [line.sku, line.product_name].some((value) => text(value).toLocaleLowerCase("th").includes(query))
      );
      const matchedSearch = !query || matchedHeader || matchedItem;
      return matchedSearch && (!paymentFilter || invoice.payment_type === paymentFilter) && (!taxFilter || invoice.tax_invoice_type === taxFilter);
    });
  }, [invoices, paymentFilter, search, taxFilter]);
  const pageCount = Math.max(1, Math.ceil(filteredInvoices.length / pageSize));
  const visibleInvoices = filteredInvoices.slice((page - 1) * pageSize, page * pageSize);
  const proposals = allTransactions.filter((line) => num(line.allocated_ghost_quantity) > 0 || num(line.repriced_quantity) > 0);
  const validations = (period?.validation_summary as Row[] | undefined) || [];
  const blockers = validations.filter((issue) => issue.severity === "error");
  const resultSummary = (period?.result_summary as Row | undefined) || {};
  const unlockedStep = period ? Math.max(1, Math.min(6, num(period.current_step) || 1)) : 1;
  const status = text(period?.status);
  const closeText = `ยืนยันปิดรอบ ${month}`;

  function navigate(step: number) {
    if (step > unlockedStep) return;
    if (dirty) {
      toast.warning("มีข้อมูลที่ยังไม่ได้บันทึก กรุณาบันทึกร่างก่อนเปลี่ยนขั้นตอน");
      return;
    }
    setActiveStep(step);
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#f7f5ef] text-slate-950">
      <header className="shrink-0 border-b border-black/10 bg-white px-4 py-4 sm:px-6 xl:px-8">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="text-xs font-bold uppercase tracking-[0.18em] text-red-600">พื้นที่ควบคุมทางบัญชี</p>
            <div className="mt-1 flex items-center gap-3">
              <h1 className="text-2xl font-bold">สรุปสิ้นเดือน</h1>
              {period ? <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-bold">{statusNames[status] || status} · revision {num(period.revision)}</span> : null}
            </div>
          </div>
          <div className="flex gap-2">
            {period && ["OPEN", "DRAFT", "FAILED"].includes(status) ? <Button disabled={!dirty || loading} onClick={() => void saveDraft()} variant="secondary"><Save className="h-4 w-4" />บันทึกร่าง</Button> : null}
            {period ? <Button onClick={() => { setPeriod(null); setActiveStep(1); }} variant="ghost">เลือกรอบอื่น</Button> : null}
          </div>
        </div>

        <div className="mt-5 overflow-x-auto pb-1">
          <ol className="grid min-w-[900px] grid-cols-6">
            {steps.map((item, index) => {
              const number = index + 1;
              const complete = number < unlockedStep || status === "CLOSED";
              const active = number === activeStep;
              const blocked = number > unlockedStep;
              const Icon = item.icon;
              return (
                <li className="relative" key={item.title}>
                  {index ? <div className={cn("absolute left-0 right-1/2 top-5 h-1", complete || active ? "bg-red-500" : "bg-slate-200")} /> : null}
                  {index < steps.length - 1 ? <div className={cn("absolute left-1/2 right-0 top-5 h-1", complete ? "bg-red-500" : "bg-slate-200")} /> : null}
                  <button className="relative z-10 flex w-full flex-col items-center px-2 text-center disabled:cursor-not-allowed" disabled={blocked} onClick={() => navigate(number)} type="button">
                    <span className={cn("grid h-11 w-11 place-items-center rounded-full border-4 border-white shadow-sm", complete ? "bg-red-600 text-white" : active ? "bg-amber-400 text-black" : "bg-slate-200 text-slate-500")}>
                      {complete ? <Check className="h-5 w-5" /> : <Icon className="h-5 w-5" />}
                    </span>
                    <span className={cn("mt-2 text-sm font-bold", active && "text-red-700")}>{number}. {item.title}</span>
                    <span className="mt-0.5 text-[11px] text-slate-500">{item.support}</span>
                  </button>
                </li>
              );
            })}
          </ol>
        </div>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6 xl:px-8">
        {loading && !period ? <LoadingState /> : null}
        {activeStep === 1 ? (
          <StepCard title="เลือกรอบบัญชี" description="สร้างรอบใหม่หรือเปิดฉบับร่างที่มีอยู่ ข้อมูลธุรกรรมต้นฉบับจะไม่ถูกแก้ไข">
            <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(520px,1.4fr)]">
              <div className="rounded-2xl border border-black/10 bg-[#fffdf8] p-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="เดือนบัญชี"><Input aria-label="เดือนบัญชี" onChange={(event) => setMonth(event.target.value)} type="month" value={month} /></Field>
                  <Field label="ขอบเขตสาขา"><Select aria-label="ขอบเขตสาขา" onChange={(event) => setBranchId(event.target.value)} value={branchId}><option value="">ทุกสาขา</option>{branches.map((branch) => <option key={text(branch.id)} value={text(branch.id)}>{text(branch.name)}</option>)}</Select></Field>
                  <Field className="sm:col-span-2" label="หมายเหตุรอบบัญชี"><Textarea onChange={(event) => setNotes(event.target.value)} placeholder="ระบุวัตถุประสงค์หรือข้อมูลประกอบ" value={notes} /></Field>
                </div>
                <Button className="mt-5 w-full" disabled={loading || !month} onClick={() => void createPeriod()}><CalendarRange className="h-4 w-4" />สร้างรอบและตรวจสอบข้อมูล</Button>
              </div>
              <div className="rounded-2xl border border-black/10 bg-white">
                <div className="border-b px-5 py-4"><h3 className="font-bold">รอบบัญชีล่าสุด</h3><p className="text-sm text-slate-500">เปิดดูฉบับร่าง สถานะอนุมัติ และ revision ย้อนหลัง</p></div>
                <div className="max-h-[420px] overflow-y-auto">
                  {workpapers.filter((item) => !cancelledIds.has(text(item.id))).length ? workpapers.filter((item) => !cancelledIds.has(text(item.id))).map((item) => (
                    <div className="flex items-center gap-2 border-b px-3 py-2" key={text(item.id)}>
                      <button className="flex min-w-0 flex-1 items-center justify-between gap-4 rounded-xl px-2 py-2 text-left hover:bg-amber-50" onClick={() => void openPeriod(text(item.id))} type="button">
                        <span className="min-w-0"><strong className="block truncate">{text(item.workpaper_number)}</strong><small className="mt-1 block text-slate-500">{text(item.period_start).slice(0, 7)} · {text(item.branch_name)}</small></span>
                        <span className="shrink-0 rounded-full bg-slate-100 px-3 py-1 text-xs font-bold">{statusNames[text(item.status)] || text(item.status)}</span>
                      </button>
                      {text(item.status) === "DRAFT" ? (
                        <Button aria-label={`ลบร่าง ${text(item.workpaper_number)}`} className="shrink-0" disabled={loading} onClick={() => openCancelDialog(item)} type="button" variant="destructive"><Trash2 className="h-4 w-4" /><span className="hidden sm:inline">ลบร่าง</span></Button>
                      ) : null}
                    </div>
                  )) : <EmptyState description="ยังไม่มีรอบบัญชี" />}
                </div>
              </div>
            </div>
          </StepCard>
        ) : null}

        {activeStep === 2 && period ? (
          <StepCard title="ตรวจสอบรายการขาย" description={`รายการต้นฉบับ ${num(period.actual_invoice_count).toLocaleString("th-TH")} ใบขาย ยอดรวม ${currency(num(period.actual_revenue))}`}>
            <ValidationPanel issues={validations} />
            <MetricCards items={[
              ["ยอดขายจริง", period.actual_revenue], ["ใบกำกับภาษีเต็มรูป", period.full_tax_revenue], ["ยอดเงินสด", period.cash_revenue], ["จำนวนใบขาย", period.actual_invoice_count]
            ]} />
            <TransactionFilters payment={paymentFilter} search={search} setPage={setPage} setPayment={setPaymentFilter} setSearch={setSearch} setTax={setTaxFilter} tax={taxFilter} />
            <TransactionsTable invoices={visibleInvoices} />
            <Pagination count={filteredInvoices.length} page={page} pageCount={pageCount} setPage={setPage} />
          </StepCard>
        ) : null}

        {activeStep === 3 && period ? (
          <StepCard title="วิเคราะห์ส่วนต่าง" description="ยอดขายจริงและใบกำกับภาษีเต็มรูปคงเดิมทุกกรณี การคำนวณต่อไปนี้เป็นเพียงสถานการณ์จำลอง">
            <SimulationBanner />
            <div className="mt-5 grid gap-5 lg:grid-cols-[1fr_1fr]">
              <div className="rounded-2xl border bg-white p-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="ยอดจำลองเป้าหมาย" hint={`ไม่เกิน ${currency(num(period.actual_revenue))}`}><Input min="0" onChange={(event) => { setTargetRevenue(event.target.value); setDirty(true); }} step="0.01" type="number" value={targetRevenue} /></Field>
                  <Field label="กำไรเหนือต้นทุน (%)"><Input min="0" onChange={(event) => { setMarkupPercent(event.target.value); setDirty(true); }} step="0.01" type="number" value={markupPercent} /></Field>
                  <Field label="กลยุทธ์การคำนวณ"><Select onChange={(event) => { setStrategy(event.target.value); setDirty(true); }} value={strategy}><option value="closest_then_oldest">ใกล้เป้าหมายที่สุด แล้วเรียงรายการเก่า</option><option value="oldest_first">รายการเก่าก่อน</option></Select></Field>
                  <Field label="เอกสารประกอบ"><Input onChange={(event) => { setSupportDocument(event.target.value); setDirty(true); }} placeholder="เลขอ้างอิงหรือที่เก็บไฟล์" value={supportDocument} /></Field>
                  <Field className="sm:col-span-2" label="เหตุผลการจำลอง (บังคับ)"><Textarea onChange={(event) => { setSimulationReason(event.target.value); setDirty(true); }} placeholder="อธิบายวัตถุประสงค์ ห้ามใช้เพื่อแก้ธุรกรรมหรือรายงานภาษีย้อนหลัง" value={simulationReason} /></Field>
                </div>
                <Button className="mt-5" disabled={loading || blockers.length > 0 || !["OPEN", "DRAFT", "FAILED"].includes(status)} onClick={() => void calculate()}><Calculator className="h-4 w-4" />คำนวณข้อเสนอ</Button>
              </div>
              <div className="rounded-2xl border bg-[#111] p-5 text-white">
                <p className="text-sm text-white/60">ขอบเขตที่คำนวณได้</p>
                <div className="mt-5 grid gap-4 sm:grid-cols-2">
                  <DarkMetric label="ต่ำสุดที่คำนวณได้" value={currency(num(resultSummary.minimum_achievable))} />
                  <DarkMetric label="สูงสุดที่คำนวณได้" value={currency(num(resultSummary.maximum_achievable || period.actual_revenue))} />
                  <DarkMetric label="ใบขายที่จะได้รับผล" value={`${num(resultSummary.affected_invoice_count).toLocaleString("th-TH")} ใบ`} />
                  <DarkMetric label="ผลต่อ GP" value={currency(num(resultSummary.gp_after) - num(resultSummary.gp_before))} />
                </div>
                <p className="mt-5 text-xs leading-5 text-white/50">เครื่องมือใช้เงินหน่วยสตางค์และลำดับข้อมูลคงที่ ผลจาก input และ version เดียวกันจึงทำซ้ำได้</p>
              </div>
            </div>
          </StepCard>
        ) : null}

        {activeStep === 4 && period ? (
          <StepCard title="ปรับปรุงสต๊อก" description="สต๊อกหลัก = สต๊อกจริง, สต๊อกรอง/สต๊อกตรวจสอบ = ข้อมูลที่ superadmin เท่านั้นมองเห็น">
            <SimulationBanner />
            <MetricCards items={[["ยอดลดที่ต้องการ", period.requested_reduction], ["ลดจากสต๊อกผี", period.ghost_reclassification_amount], ["ลดจากราคาจำลอง", period.price_scenario_reduction_amount], ["ยอดจำลอง", period.scenario_revenue], ["เป้าหมาย", period.target_revenue], ["ยอดคงเหลือจากเป้า", period.unresolved_difference]]} />
            <div className="mt-4 rounded-2xl border border-blue-200 bg-blue-50 p-4 text-sm leading-6 text-blue-950">
              <strong>สูตรที่ใช้วิเคราะห์</strong>
              <p>
                ยอดลดที่ต้องการ = ยอดขายจริง − เป้าหมาย · ยอดจำลอง = ยอดขายจริง − ยอดลดจากสต๊อกผี − ยอดลดจากราคาจำลอง ·
                ยอดคงเหลือจากเป้า = ยอดจำลอง − เป้าหมาย (ค่าติดลบหมายถึงยอดจำลองต่ำกว่าเป้า)
              </p>
            </div>
            <div className="mt-5 overflow-hidden rounded-2xl border bg-white">
              <TableContainer>
                <Table>
                  <TableHeader><TableRow><TableHead>ใช้ข้อเสนอ</TableHead><TableHead>ใบขาย / สินค้า</TableHead><TableHead>รายละเอียดข้อเสนอ</TableHead><TableHead>ยอดรายการรวม VAT เดิม → จำลอง</TableHead><TableHead>ยอดลดจากข้อเสนอ (รวม VAT)</TableHead><TableHead>เหตุผล</TableHead></TableRow></TableHeader>
                  <TableBody>{proposals.map((line) => {
                    const adjustmentType = text(line.adjustment_type);
                    const originalGross = num(line.original_gross_line_total || line.original_line_total);
                    const scenarioGross = num(line.scenario_gross_line_total ?? line.scenario_line_total);
                    const totalReduction = Math.max(0, originalGross - scenarioGross);
                    const derivedGhostReduction = num(line.quantity) > 0
                      ? Math.round((originalGross / num(line.quantity)) * num(line.allocated_ghost_quantity) * 100) / 100
                      : 0;
                    const ghostReduction = line.ghost_reclassification_reduction == null
                      ? derivedGhostReduction
                      : num(line.ghost_reclassification_reduction);
                    const priceReduction = line.price_scenario_reduction == null
                      ? Math.max(0, totalReduction - ghostReduction)
                      : num(line.price_scenario_reduction);
                    return (
                      <TableRow key={text(line.id)}>
                        <TableCell><Checkbox checked={Boolean(line.included)} disabled={status !== "DRAFT" || loading} onChange={(event) => void toggleLine(line, event.target.checked)} /></TableCell>
                        <TableCell><strong>{text(line.invoice_number)}</strong><small className="block text-slate-500">{text(line.sku)} · {text(line.product_name)}</small></TableCell>
                        <TableCell className="min-w-64 text-sm">
                          {adjustmentType === "ghost_reclassification" || adjustmentType === "ghost_and_price" ? (
                            <p><strong>จำแนกสต๊อกจริง → สต๊อกผี</strong><span className="block text-xs text-slate-500">{num(line.allocated_ghost_quantity).toLocaleString("th-TH")} หน่วย · ลด {currency(ghostReduction)}</span></p>
                          ) : null}
                          {adjustmentType === "price_scenario" || adjustmentType === "ghost_and_price" ? (
                            <p className={adjustmentType === "ghost_and_price" ? "mt-2" : ""}><strong>จำลองราคาต่อหน่วย</strong><span className="block text-xs text-slate-500">{currency(num(line.original_unit_price))} → {currency(num(line.proposed_unit_price))} · {num(line.repriced_quantity).toLocaleString("th-TH")} หน่วย · ลด {currency(priceReduction)}</span></p>
                          ) : null}
                        </TableCell>
                        <TableCell>{currency(originalGross)} → {currency(scenarioGross)}</TableCell>
                        <TableCell className="font-bold">{currency(ghostReduction + priceReduction)}</TableCell>
                        <TableCell className="max-w-xs text-xs text-slate-500">{text(line.note)}</TableCell>
                      </TableRow>
                    );
                  })}</TableBody>
                </Table>
              </TableContainer>
              {!proposals.length ? <EmptyState description="ยังไม่มีข้อเสนอ กรุณาคำนวณในขั้นตอนก่อนหน้า" /> : null}
            </div>
          </StepCard>
        ) : null}

        {activeStep === 5 && period ? (
          <StepCard title="ตรวจสอบผลลัพธ์" description="ทบทวนยอดก่อนและหลัง ข้อเสนอที่เลือก คำเตือน และข้อมูล audit ก่อนส่งอนุมัติ">
            <SimulationBanner />
            <MetricCards items={[["ยอดขายจริง", period.actual_revenue], ["ยอดบัญชีที่เสนอ", period.scenario_revenue], ["เป้าหมาย", period.target_revenue], ["ส่วนต่าง", period.unresolved_difference], ["GP ก่อน", resultSummary.gp_before], ["GP หลัง", resultSummary.gp_after]]} />
            <div className="mt-5 grid gap-5 xl:grid-cols-2">
              <ReviewList title="ข้อเสนอที่เลือก" rows={proposals.filter((line) => line.included)} />
              <ReviewList title="ข้อเสนอที่ไม่เลือก" rows={proposals.filter((line) => !line.included)} />
            </div>
            <div className="mt-5 flex flex-wrap gap-3">
              <Button disabled={loading || status !== "DRAFT" || !proposals.length} onClick={() => void statusAction("submit")}><ShieldCheck className="h-4 w-4" />ส่งอนุมัติ</Button>
              <Button onClick={exportSimulation} variant="secondary">ส่งออก Excel/CSV (ข้อมูลจำลอง)</Button>
              <Button onClick={() => window.print()} variant="secondary">พิมพ์ / บันทึก PDF (ข้อมูลจำลอง)</Button>
            </div>
          </StepCard>
        ) : null}

        {activeStep === 6 && period ? (
          <StepCard title="ยืนยันและปิดรอบ" description="การปิดรอบจะอนุมัติ adjustment, ลง inventory ledger, สร้าง snapshot checksum และล็อกรอบ">
            <div className="grid gap-5 xl:grid-cols-[1fr_420px]">
              <div className="rounded-2xl border bg-white p-5">
                <h3 className="font-bold">รายการตรวจสอบก่อนปิดรอบ</h3>
                <Checklist checked={blockers.length === 0} text="ไม่มี validation error ที่ขัดขวาง" />
                <Checklist checked={proposals.length > 0} text="ตรวจข้อเสนอปรับปรุงและรายการที่ไม่เลือกแล้ว" />
                <Checklist checked={Boolean(simulationReason)} text="มีเหตุผลและเอกสารอ้างอิงของข้อมูลจำลอง" />
                <Checklist checked={status === "APPROVED" || status === "CLOSED"} text="ผู้มีสิทธิ์อนุมัติรายการแล้ว" />
                {period.checksum ? <p className="mt-5 break-all rounded-xl bg-slate-100 p-3 font-mono text-xs">checksum: {text(period.checksum)}</p> : null}
              </div>
              <div className="rounded-2xl border border-red-200 bg-red-50 p-5">
                <ShieldCheck className="h-8 w-8 text-red-700" />
                <h3 className="mt-3 text-lg font-bold">การดำเนินการของรอบบัญชี</h3>
                <p className="mt-2 text-sm leading-6 text-red-950/70">ธุรกรรมขายและใบกำกับภาษีต้นฉบับจะไม่เปลี่ยน ระบบลงเฉพาะ adjustment และ ledger ที่ตรวจสอบย้อนกลับได้</p>
                {status === "PENDING_APPROVAL" ? <Button className="mt-5 w-full" disabled={loading} onClick={() => void statusAction("approve")}><CheckCircle2 className="h-4 w-4" />อนุมัติรายการปรับปรุง</Button> : null}
                {status === "APPROVED" ? <Button className="mt-5 w-full" disabled={loading} onClick={() => setCloseDialog(true)}><LockKeyhole className="h-4 w-4" />ยืนยันปิดรอบ</Button> : null}
                {status === "CLOSED" ? <><p className="mt-5 rounded-xl bg-emerald-100 p-3 text-sm font-bold text-emerald-900">ปิดรอบเมื่อ {formatDate(period.closed_at)}</p><Button className="mt-3 w-full" onClick={() => setReopenDialog(true)} variant="secondary"><RotateCcw className="h-4 w-4" />เปิด revision ใหม่</Button></> : null}
              </div>
            </div>
          </StepCard>
        ) : null}
      </main>

      {period && activeStep > 1 ? (
        <footer className="flex shrink-0 items-center justify-between border-t border-black/10 bg-white px-4 py-3 sm:px-6 xl:px-8">
          <Button disabled={activeStep <= 1} onClick={() => navigate(activeStep - 1)} variant="ghost"><ChevronLeft className="h-4 w-4" />ย้อนกลับ</Button>
          <div className="hidden text-xs text-slate-500 sm:block">{dirty ? "มีข้อมูลยังไม่ได้บันทึก" : `บันทึกล่าสุด · lock version ${num(period.lock_version)}`}</div>
          <Button disabled={activeStep >= unlockedStep || activeStep >= 6} onClick={() => navigate(activeStep + 1)} variant="secondary">ขั้นตอนถัดไป<ChevronRight className="h-4 w-4" /></Button>
        </footer>
      ) : null}

      <Dialog onOpenChange={setCloseDialog} open={closeDialog}>
        <DialogContent>
          <DialogHeader title="ยืนยันปิดและล็อกรอบบัญชี" description={`พิมพ์ “${closeText}” เพื่อยืนยัน การดำเนินการนี้สร้าง ledger และ checksum ถาวร`} />
          <Field label="ข้อความยืนยัน"><Input autoFocus onChange={(event) => setConfirmation(event.target.value)} value={confirmation} /></Field>
          <div className="mt-5 flex justify-end gap-2"><Button onClick={() => setCloseDialog(false)} variant="ghost">ยกเลิก</Button><Button disabled={confirmation !== closeText || loading} onClick={() => void closePeriod()}>ยืนยันปิดรอบ</Button></div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setReopenDialog} open={reopenDialog}>
        <DialogContent>
          <DialogHeader title="เปิด revision ใหม่" description="ระบบจะย้อน inventory ledger ของ revision นี้และสร้างฉบับร่าง revision ถัดไป พร้อมเก็บ audit log ทั้งหมด" />
          <Field label="เหตุผลการเปิดรอบใหม่"><Textarea autoFocus onChange={(event) => setReopenReason(event.target.value)} value={reopenReason} /></Field>
          <div className="mt-5 flex justify-end gap-2"><Button onClick={() => setReopenDialog(false)} variant="ghost">ยกเลิก</Button><Button disabled={!reopenReason.trim() || loading} onClick={() => void reopenPeriod()}>ย้อนรายการและสร้าง revision</Button></div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { if (!open && !loading) setCancelTarget(null); }} open={Boolean(cancelTarget)}>
        <DialogContent>
          <DialogHeader title="ลบรอบบัญชีฉบับร่าง" description="ระบบจะยกเลิกร่างและเก็บรายละเอียดไว้ในประวัติ โดยไม่แก้ไขยอดสต๊อกหรือธุรกรรมต้นฉบับ" />
          <div className="space-y-4">
            <Field label={`พิมพ์เลขรอบ ${text(cancelTarget?.workpaper_number)} เพื่อยืนยัน`}><Input autoFocus onChange={(event) => setCancelConfirmation(event.target.value)} value={cancelConfirmation} /></Field>
            <Field label="เหตุผลการลบร่าง"><Textarea onChange={(event) => setCancelReason(event.target.value)} placeholder="ระบุเหตุผลเพื่อใช้ตรวจสอบย้อนหลัง" value={cancelReason} /></Field>
          </div>
          <div className="mt-5 flex justify-end gap-2"><Button disabled={loading} onClick={() => setCancelTarget(null)} variant="ghost">ยกเลิก</Button><Button disabled={loading || cancelConfirmation !== text(cancelTarget?.workpaper_number) || !cancelReason.trim()} onClick={() => void cancelDraft()} variant="destructive">{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}ยืนยันลบร่าง</Button></div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function StepCard({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return <section><div className="mb-5"><h2 className="text-xl font-bold">{title}</h2><p className="mt-1 text-sm text-slate-500">{description}</p></div>{children}</section>;
}

function MetricCards({ items }: { items: Array<[string, unknown]> }) {
  return <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">{items.map(([label, value]) => <div className="rounded-2xl border bg-white p-4" key={label}><p className="text-xs font-bold text-slate-500">{label}</p><p className="mt-2 text-xl font-bold tabular-nums">{label.includes("จำนวน") ? num(value).toLocaleString("th-TH") : currency(num(value))}</p></div>)}</div>;
}

function DarkMetric({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs text-white/50">{label}</p><p className="mt-1 text-xl font-bold">{value}</p></div>;
}

function ValidationPanel({ issues }: { issues: Row[] }) {
  return <div className="grid gap-2">{issues.map((issue) => <div className={cn("flex items-start gap-3 rounded-xl border px-4 py-3 text-sm", issue.severity === "error" ? "border-red-200 bg-red-50 text-red-900" : issue.severity === "warning" ? "border-amber-200 bg-amber-50 text-amber-950" : "border-emerald-200 bg-emerald-50 text-emerald-900")} key={text(issue.code)}>{issue.severity === "success" ? <CheckCircle2 className="h-5 w-5" /> : <AlertTriangle className="h-5 w-5" />}<span><strong>{text(issue.message)}</strong>{num(issue.count) ? ` · ${num(issue.count).toLocaleString("th-TH")} รายการ` : ""}</span></div>)}</div>;
}

function SimulationBanner() {
  return <div className="flex items-start gap-3 rounded-2xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950"><AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" /><div><strong>ข้อมูลจำลอง — ไม่ใช่รายงานทางบัญชีหรือภาษี</strong><p className="mt-1">ไม่แก้ธุรกรรมต้นฉบับ ไม่เปลี่ยนเลขเอกสาร และไม่สามารถส่งออกเป็นรายงานภาษีทางการ</p></div></div>;
}

function TransactionFilters({ search, setSearch, payment, setPayment, tax, setTax, setPage }: { search: string; setSearch: (value: string) => void; payment: string; setPayment: (value: string) => void; tax: string; setTax: (value: string) => void; setPage: (value: number) => void }) {
  return <div className="mt-5 grid gap-3 rounded-2xl border bg-white p-4 md:grid-cols-[1fr_220px_220px]"><label className="relative"><Search className="absolute left-3 top-3 h-4 w-4 text-slate-400" /><Input className="pl-9" onChange={(event) => { setSearch(event.target.value); setPage(1); }} placeholder="ค้นหาเลขใบขาย SKU สินค้า หรือสาขา" value={search} /></label><Select onChange={(event) => { setPayment(event.target.value); setPage(1); }} value={payment}><option value="">ทุกวิธีชำระ</option><option value="cash">เงินสด</option><option value="bank_transfer">เงินโอน</option><option value="mixed">เงินสด + เงินโอน</option><option value="unpaid">ยังไม่ชำระ</option></Select><Select onChange={(event) => { setTax(event.target.value); setPage(1); }} value={tax}><option value="">ทุกประเภทเอกสาร</option><option value="abbreviated">ใบกำกับอย่างย่อ</option><option value="full">ใบกำกับเต็มรูป</option></Select></div>;
}

function candidateStatus(line: Row) {
  if (text(line.tax_invoice_type) === "full") return "ไม่เข้าเงื่อนไข · ใบกำกับภาษีเต็มรูป";
  if (text(line.payment_type) === "bank_transfer") return "ไม่เข้าเงื่อนไข · ชำระด้วยเงินโอน";
  if (text(line.payment_type) === "unpaid") return "ไม่เข้าเงื่อนไข · ยังชำระไม่ครบ";
  if (text(line.original_stock_bucket) !== "real") return "ไม่เข้าเงื่อนไข · ไม่ได้ตัดจากสต๊อกจริง";
  if (num(line.cash_payment_amount) <= 0) return "ไม่เข้าเงื่อนไข · ไม่มียอดเงินสด";
  return "เข้าเงื่อนไข · ใช้ยอดเงินสดเป็นเพดาน";
}

function TransactionsTable({ invoices }: { invoices: InvoiceGroup[] }) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  function toggle(invoiceID: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(invoiceID)) next.delete(invoiceID);
      else next.add(invoiceID);
      return next;
    });
  }

  return (
    <div className="mt-4 overflow-hidden rounded-2xl border bg-white">
      <TableContainer>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12"><span className="sr-only">ดูสินค้า</span></TableHead>
              <TableHead>ลำดับ</TableHead>
              <TableHead>เลขใบขาย / วันที่</TableHead>
              <TableHead>สาขา</TableHead>
              <TableHead>ชำระ / เงินสดที่ใช้ได้</TableHead>
              <TableHead>เอกสาร</TableHead>
              <TableHead>จำนวนรายการ</TableHead>
              <TableHead>ยอดรวมใบขาย</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoices.map((invoice) => {
              const invoiceID = text(invoice.invoice_id) || text(invoice.invoice_number);
              const isExpanded = expanded.has(invoiceID);
              return (
                <Fragment key={invoiceID}>
                  <TableRow>
                    <TableCell>
                      <button
                        aria-expanded={isExpanded}
                        aria-label={`${isExpanded ? "ย่อ" : "ดู"}รายละเอียด ${text(invoice.invoice_number)}`}
                        className="grid h-8 w-8 place-items-center rounded-full border hover:bg-slate-100"
                        onClick={() => toggle(invoiceID)}
                        type="button"
                      >
                        <ChevronDown className={cn("h-4 w-4 transition-transform", isExpanded && "rotate-180")} />
                      </button>
                    </TableCell>
                    <TableCell>{num(invoice.management_sequence).toLocaleString("th-TH")}</TableCell>
                    <TableCell><strong>{text(invoice.invoice_number)}</strong><small className="block text-slate-500">{formatDate(invoice.issued_at)}</small></TableCell>
                    <TableCell>{text(invoice.branch_name)}</TableCell>
                    <TableCell><strong>{paymentName(invoice.payment_type)}</strong><small className="block text-slate-500">เงินสด {currency(num(invoice.cash_payment_amount))}</small></TableCell>
                    <TableCell>{invoice.tax_invoice_type === "full" ? "เต็มรูป" : "อย่างย่อ"}</TableCell>
                    <TableCell>{invoice.item_count.toLocaleString("th-TH")} รายการ</TableCell>
                    <TableCell className="font-bold">{currency(invoice.invoice_total)}</TableCell>
                  </TableRow>
                  {isExpanded ? (
                    <TableRow className="bg-slate-50 hover:bg-slate-50">
                      <TableCell className="p-4" colSpan={8}>
                        <div className="overflow-hidden rounded-xl border bg-white">
                          <Table>
                            <TableHeader>
                              <TableRow>
                                <TableHead>SKU / สินค้า</TableHead>
                                <TableHead>สต๊อกที่ตัด</TableHead>
                                <TableHead>จำนวน</TableHead>
                                <TableHead>ราคาต่อหน่วย</TableHead>
                                <TableHead>VAT</TableHead>
                                <TableHead>ยอดรายการ</TableHead>
                                <TableHead>สถานะการวิเคราะห์</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {invoice.lines.map((line) => (
                                <TableRow key={text(line.id) || text(line.invoice_item_id)}>
                                  <TableCell><strong>{text(line.sku)}</strong><small className="block max-w-xs text-slate-500">{text(line.product_name)}</small></TableCell>
                                  <TableCell>{text(line.original_stock_bucket) === "real" ? "สต๊อกจริง" : "สต๊อกผี"}</TableCell>
                                  <TableCell>{num(line.quantity).toLocaleString("th-TH")}</TableCell>
                                  <TableCell>{currency(num(line.original_unit_price))}</TableCell>
                                  <TableCell>{num(line.tax_rate).toLocaleString("th-TH")}%</TableCell>
                                  <TableCell>{currency(num(line.original_gross_line_total || line.original_line_total))}</TableCell>
                                  <TableCell className="max-w-xs text-xs text-slate-500">{candidateStatus(line)}</TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : null}
                </Fragment>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
      {!invoices.length ? <EmptyState description="ไม่พบใบขายตามตัวกรอง" /> : null}
    </div>
  );
}

function Pagination({ count, page, pageCount, setPage }: { count: number; page: number; pageCount: number; setPage: (value: number) => void }) {
  return <div className="mt-3 flex items-center justify-between text-sm text-slate-500"><span>พบ {count.toLocaleString("th-TH")} ใบขาย</span><div className="flex items-center gap-2"><Button disabled={page <= 1} onClick={() => setPage(page - 1)} variant="ghost"><ChevronLeft className="h-4 w-4" /></Button><span>{page} / {pageCount}</span><Button disabled={page >= pageCount} onClick={() => setPage(page + 1)} variant="ghost"><ChevronRight className="h-4 w-4" /></Button></div></div>;
}

function ReviewList({ title, rows }: { title: string; rows: Row[] }) {
  return <div className="overflow-hidden rounded-2xl border bg-white"><div className="border-b px-4 py-3 font-bold">{title} · {rows.length.toLocaleString("th-TH")}</div><div className="max-h-72 overflow-y-auto">{rows.map((line) => <div className="flex items-center justify-between gap-4 border-b px-4 py-3 text-sm" key={text(line.id)}><span><strong>{text(line.invoice_number)}</strong><small className="block text-slate-500">{text(line.sku)} · {adjustmentNames[text(line.adjustment_type)]}</small></span><span className="font-bold">{currency(num(line.original_line_total) - num(line.scenario_line_total))}</span></div>)}{!rows.length ? <EmptyState className="p-6" /> : null}</div></div>;
}

function Checklist({ checked, text: label }: { checked: boolean; text: string }) {
  return <div className="mt-3 flex items-center gap-3 rounded-xl bg-slate-50 px-4 py-3 text-sm"><span className={cn("grid h-6 w-6 place-items-center rounded-full", checked ? "bg-emerald-600 text-white" : "bg-amber-200 text-amber-900")}>{checked ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}</span>{label}</div>;
}

function LoadingState() {
  return <div className="grid h-72 place-items-center"><div className="text-center"><Loader2 className="mx-auto h-7 w-7 animate-spin" /><p className="mt-3 text-sm text-slate-500">กำลังโหลดข้อมูลรอบบัญชี</p></div></div>;
}
