"use client";

import { cloneElement, isValidElement, useId, useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { v2API } from "@/services/v2";
import { V2Workflows } from "./workflows";
import { V2Lines, blankLine, type Entry } from "./line-editor";
import { V2DocumentTools } from "./document-tools";
import { DrawerPick, V2CancellationForm, V2Drawers, V2Refunds } from "./refund-shifts";

type Row = Record<string, unknown>;
const inputClass = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
const buttonClass = "min-h-11 rounded-xl bg-primary px-4 py-2 font-semibold text-primary-foreground disabled:opacity-50";
const tabs = [ ["policy", "ตั้งค่า"], ["customers", "ลูกค้า"], ["inventory", "สต๊อก"], ["shipments", "โอนระหว่างทาง"], ["counts", "ตรวจนับ"], ["returns", "คืนสินค้า"], ["reservations", "จองสินค้า"], ["documents", "เอกสาร"], ["price-rules", "ราคาขายส่ง"], ["promotions", "โปรโมชั่น"], ["finance", "รับ/จ่ายเงิน"], ["credits", "ใช้เครดิต/คืนเงิน"], ["statements", "Statement"], ["stock-report", "รายงานสต๊อก"], ["expiry", "วันหมดอายุ"], ["cancellations", "คืนบิลที่ยกเลิก"], ["drawers", "รอบเงินสด"], ["jobs", "งานเบื้องหลัง"], ["ledger", "ประวัติ"] ];
function str(value: unknown) { return value == null ? "" : String(value); }
function rows(value: unknown): Row[] { return Array.isArray(value) ? value as Row[] : []; }
function money(value: unknown) { return new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" }).format(Number(value || 0) / 100); }
function cents(value: FormDataEntryValue | string | null) {
  const text = str(value).trim();
  if (!/^\d+(\.\d{1,2})?$/.test(text)) throw new Error("จำนวนเงินต้องเป็นตัวเลขไม่ติดลบและทศนิยมไม่เกิน 2 ตำแหน่ง");
  const [whole, fraction = ""] = text.split(".");
  const amount = Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
  if (!Number.isSafeInteger(amount)) throw new Error("จำนวนเงินเกินขอบเขต");
  return amount;
}
function bangkokDate() { const parts = new Intl.DateTimeFormat("en", { timeZone: "Asia/Bangkok", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date()); return ["year", "month", "day"].map((type) => parts.find((p) => p.type === type)?.value).join("-"); }
function Field({ label, children }: { label: string; children: ReactNode }) {
  const id = useId();
  return <label className="grid min-w-0 gap-1 text-sm font-medium"><span id={id}>{label}</span>{isValidElement<{ "aria-labelledby"?: string }>(children) ? cloneElement(children, { "aria-labelledby": id }) : children}</label>;
}
function Text({ label, name, value = "", type = "text", required = true }: { label: string; name: string; value?: string; type?: string; required?: boolean }) {
  return <Field label={label}><input className={inputClass} defaultValue={value} name={name} required={required} type={type} /></Field>;
}
function Pick({ label, name, options, required = true }: { label: string; name: string; options: Row[]; required?: boolean }) {
  return <Field label={label}><select className={inputClass} name={name} required={required}><option value="">เลือก{label}</option>{options.map((item) => <option key={str(item.id)} value={str(item.id)}>{str(item.name || item.document_number || item.reference || item.id)}</option>)}</select></Field>;
}


export function V2Workspace({ branches }: { branches: Array<{ id: string; name: string }> }) {
  const [branchID, setBranchID] = useState("");
  const [tab, setTab] = useState("policy");
  const [catalog, setCatalog] = useState<Row>({});
  const [customers, setCustomers] = useState<Row[]>([]);
  const [data, setData] = useState<Row>({});
  const [entries, setEntries] = useState<Entry[]>([blankLine()]);
  const [stockAction, setStockAction] = useState("receive");
  const [documentKind, setDocumentKind] = useState("quotation");
  const [paymentDirection, setPaymentDirection] = useState("receive");
  const [paymentMethod, setPaymentMethod] = useState("bank_transfer");
  const [approvalID, setApprovalID] = useState("");
  const [detail, setDetail] = useState<Row | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const keys = useRef(new Map<string, string>());
  const saving = useRef(false);
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current;
    if (!branchID) { setData({}); setCatalog({}); setCustomers([]); return; }
    setLoading(true); setError("");
    try {
      const query = `?branch_id=${encodeURIComponent(branchID)}`;
      const [nextCatalog, nextCustomers, nextData, nextPromotions, nextDrawers] = await Promise.all([
        v2API<Row>(`/catalog${query}`), v2API<Row>(`/customers${query}`),
        tab === "policy" || tab === "customers" ? Promise.resolve({}) : v2API<Row>(`/${tab === "credits" ? "finance" : tab}${query}`),
        v2API<Row>(`/promotions${query}`),
        ["finance", "cancellations", "credits"].includes(tab) ? v2API<Row>(`/drawers${query}`) : Promise.resolve({ items: [] })
      ]);
      if (generation !== loadGeneration.current) return;
      setCatalog({ ...nextCatalog, promotions: rows(nextPromotions.items), drawers: rows(nextDrawers.items) }); setCustomers(rows(nextCustomers.items)); setData(nextData);
    } catch (caught) { if (generation === loadGeneration.current) setError(caught instanceof Error ? caught.message : "โหลดข้อมูลไม่สำเร็จ"); }
    finally { if (generation === loadGeneration.current) setLoading(false); }
  }, [branchID, tab]);
  useEffect(() => { void load(); }, [load]);

  async function submit(path: string, payload: Row) {
    if (saving.current || !branchID) return;
    const input = { ...payload, branch_id: branchID };
    const signature = `${path}:${JSON.stringify(input)}`;
    let key = keys.current.get(signature);
    if (!key) { key = crypto.randomUUID(); keys.current.set(signature, key); }
    saving.current = true;
    setBusy(true); setError(""); setMessage("");
    try {
      const result = await v2API<Row>(path, input, key);
      keys.current.delete(signature);
      setDetail(null);
      setApprovalID("");
      setMessage(`บันทึกแล้ว${result.document_number ? ` · ${str(result.document_number)}` : ""}`);
      await load();
    } catch (caught) { setError(caught instanceof Error ? caught.message : "บันทึกไม่สำเร็จ"); }
    finally { saving.current = false; setBusy(false); }
  }
  async function form(event: FormEvent<HTMLFormElement>, path: string, build: (values: FormData) => Row) {
    event.preventDefault();
    try { await submit(path, build(new FormData(event.currentTarget))); }
    catch (caught) { setError(caught instanceof Error ? caught.message : "ข้อมูลไม่ถูกต้อง"); }
  }
  function documentPayload(f: FormData): Row {
    return { kind: documentKind, promotion_id: str(f.get("promotion")), ...(documentKind === "ap_bill" ? { supplier_id: f.get("counterparty") } : { customer_id: f.get("counterparty") }), issued_on: bangkokDate(), due_on: str(f.get("due")), notes: str(f.get("notes")), lines: entries.map((line) => ({ product_id: line.product_id, unit_id: line.unit_id, quantity: Number(line.quantity), discount_cents: cents(line.discount), vat_bps: Math.round(Number(line.vat) * 100), ...(documentKind === "ap_bill" ? { unit_price_cents: cents(line.cost) } : {}) })) };
  }
  async function approveDiscount(formElement: HTMLFormElement | null) {
    if (!formElement || saving.current || !branchID) return;
    try {
      const f = new FormData(formElement);
      const reason = str(f.get("approval_reason"));
      if (!reason.trim()) throw new Error("ระบุเหตุผลอนุมัติส่วนลดก่อน");
      const payload = { branch_id: branchID, reason, document: documentPayload(f) };
      const signature = `approval:${JSON.stringify(payload)}`;
      let key = keys.current.get(signature);
      if (!key) { key = crypto.randomUUID(); keys.current.set(signature, key); }
      saving.current = true; setBusy(true); setError("");
      const result = await v2API<Row>("/discount-approvals", payload, key);
      keys.current.delete(signature); setApprovalID(str(result.id));
      setMessage("อนุมัติส่วนลดตามรายการนี้แล้ว ใช้ได้ภายใน 15 นาที หากเปลี่ยนข้อมูลต้องอนุมัติใหม่");
    } catch (caught) { setError(caught instanceof Error ? caught.message : "อนุมัติไม่ได้"); }
    finally { saving.current = false; setBusy(false); }
  }
  const stockLines = () => entries.map((line) => ({ product_id: line.product_id, unit_id: line.unit_id, quantity: Number(line.quantity), unit_cost_cents: cents(line.cost), discount_cents: cents(line.discount), lot_number: line.lot_number, expires_on: line.expires_on, lot_id: line.lot_id }));
  const today = bangkokDate();
  const panelClass = "grid gap-4 rounded-2xl border bg-card p-4 sm:grid-cols-2 lg:grid-cols-3";

  return <div className="space-y-4">
    <div className="rounded-2xl border bg-card p-4"><h1 className="text-2xl font-bold">พื้นที่ V2</h1><p className="mt-1 text-sm text-muted-foreground">ยอดและเอกสารทดลองของ V2 · ยังไม่รวมยอดธุรกรรมจาก V1</p><div className="mt-4 max-w-md"><Field label="สาขา"><select disabled={busy} className={inputClass} onChange={(e) => { setBranchID(e.target.value); setDetail(null); setEntries([blankLine()]); setApprovalID(""); }} value={branchID}><option value="">เลือกสาขา</option>{branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}</select></Field></div></div>
    <nav aria-label="งาน V2" className="flex gap-2 overflow-x-auto pb-1">{tabs.map(([id, label]) => <button aria-pressed={tab === id} disabled={busy} className={`shrink-0 rounded-xl px-4 py-3 text-sm font-semibold ${tab === id ? "bg-primary text-primary-foreground" : "border bg-card"}`} key={id} onClick={() => { setTab(id); setDetail(null); setEntries([blankLine()]); setApprovalID(""); }} type="button">{label}</button>)}</nav>
    {error ? <p className="rounded-xl border border-destructive bg-card p-3 text-sm text-destructive" role="alert">{error}</p> : null}
    {message ? <p className="rounded-xl border bg-card p-3 text-sm" role="status">{message}</p> : null}
    {!branchID ? <p className="p-4 text-muted-foreground">เลือกสาขาเพื่อเริ่มทำรายการ</p> : loading ? <p role="status">กำลังโหลดข้อมูล…</p> : <>
      {tab === "policy" ? <form className={panelClass} onSubmit={(e) => void form(e, "/policy", (f) => ({ cost_method: f.get("cost_method"), max_discount_bps: Math.round(Number(f.get("discount")) * 100), allow_branch_promotions: f.get("promotions") === "on" }))}>
        <Field label="วิธีต้นทุน"><select className={inputClass} defaultValue={str((catalog.policy as Row | null)?.cost_method) || "fifo"} name="cost_method"><option value="fifo">FIFO</option><option value="moving_average">Moving Average</option></select></Field>
        <Text label="เพดานส่วนลด (%)" name="discount" value={str(Number((catalog.policy as Row | null)?.max_discount_bps || 0) / 100)} type="number" />
        <label className="flex items-center gap-2"><input defaultChecked={Boolean((catalog.policy as Row | null)?.allow_branch_promotions)} name="promotions" type="checkbox" />อนุญาตโปรโมชั่นของสาขา</label>
        <p className="col-span-full text-sm text-muted-foreground">วิธีต้นทุนเปลี่ยนได้ก่อนมีรายการ stock เท่านั้น ส่วน FEFO ใช้เลือกล็อตจ่ายแยกจากวิธีต้นทุน</p><button className={buttonClass} disabled={busy}>บันทึกนโยบาย</button>
      </form> : null}
      {tab === "customers" ? <><form className={panelClass} onSubmit={(e) => void form(e, "/customers", (f) => ({ code: f.get("code"), type: f.get("type"), name: f.get("name"), tax_id: f.get("tax"), billing_address: f.get("billing"), shipping_address: f.get("shipping"), phone: f.get("phone"), email: f.get("email"), credit_days: Number(f.get("days")), credit_limit_cents: cents(f.get("limit")), active: true }))}>
        <Text label="รหัสลูกค้า" name="code" /><Field label="ประเภท"><select className={inputClass} name="type"><option value="person">บุคคล</option><option value="business">องค์กร</option></select></Field><Text label="ชื่อ" name="name" /><Text label="เลขภาษี" name="tax" required={false} /><Text label="ที่อยู่ออกเอกสาร" name="billing" required={false} /><Text label="ที่อยู่จัดส่ง" name="shipping" required={false} /><Text label="โทรศัพท์" name="phone" required={false} /><Text label="อีเมล" name="email" required={false} type="email" /><Text label="เครดิต (วัน)" name="days" type="number" value="0" /><Text label="วงเงินเครดิต (บาท)" name="limit" value="0" /><button className={buttonClass} disabled={busy}>เพิ่มลูกค้า</button>
      </form><div className="grid gap-3 md:grid-cols-2">{customers.map((customer) => <article className="rounded-xl border bg-card p-4" key={str(customer.id)}><strong>{str(customer.name)}</strong><p className="text-sm">{str(customer.customer_code)} · เครดิต {str(customer.credit_days)} วัน · วงเงิน {money(customer.credit_limit_cents)}</p></article>)}</div></> : null}
      {tab === "inventory" ? <><form className={panelClass} onSubmit={(e) => void form(e, `/stock/${stockAction}`, (f) => ({ reference: f.get("reference"), reason: f.get("reason"), destination_branch_id: str(f.get("destination")), items: stockLines(), header_discount_cents: stockAction === "receive" ? cents(f.get("header_discount")) : 0, shipping_cents: stockAction === "receive" ? cents(f.get("shipping")) : 0, quarantine: f.get("quarantine") === "on" }))}>
        <Field label="งานสต๊อก"><select className={inputClass} onChange={(e) => setStockAction(e.target.value)} value={stockAction}><option value="receive">รับเข้า</option><option value="issue">จ่ายสินค้า</option><option value="transfer">โอนพร้อมรับปลายทาง</option></select></Field><Text label="เอกสารอ้างอิง" name="reference" /><Text label="เหตุผล/หมายเหตุ" name="reason" required={false} />
        {stockAction === "transfer" ? <Pick label="สาขาปลายทาง" name="destination" options={branches.filter((branch) => branch.id !== branchID)} /> : null}
        {stockAction === "receive" ? <><Text label="ส่วนลดท้ายเอกสาร (บาท)" name="header_discount" value="0" /><Text label="ค่าส่งเข้าต้นทุน (บาท)" name="shipping" value="0" /><label className="flex items-center gap-2"><input name="quarantine" type="checkbox" />รับเข้า quarantine</label></> : null}
        <V2Lines catalog={catalog} entries={entries} setEntries={setEntries} stock receipt={stockAction === "receive"} /><button className={buttonClass} disabled={busy}>บันทึก{stockAction === "receive" ? "รับเข้า" : stockAction === "issue" ? "จ่ายสินค้า" : "โอนสินค้า"}</button>
      </form><div className="grid gap-3 lg:grid-cols-2">{rows(data.items).map((lot) => <article className="space-y-2 rounded-xl border bg-card p-4" key={str(lot.id)}><strong>{str(lot.product_name)}</strong><p className="text-sm">ล็อต {str(lot.lot_number)} · หมดอายุ {str(lot.expires_on) || "ไม่กำหนด"} · {str(lot.state)}</p><p className="text-sm">คงเหลือ {str(lot.remaining_quantity)} · จอง {str(lot.reserved_quantity)} · ขายได้ {str(lot.sellable_quantity)} หน่วยฐาน</p><form className="grid gap-2 sm:grid-cols-3" onSubmit={(e) => void form(e, "/lots/state", (f) => ({ lot_id: lot.id, state: f.get("state"), reason: f.get("reason") }))}><select aria-label="สถานะล็อตใหม่" className={inputClass} name="state"><option value="quarantined">กักกัน</option><option value="recalled">เรียกคืน</option><option value="available">ปล่อยขาย</option></select><input aria-label="เหตุผลเปลี่ยนสถานะล็อต" className={inputClass} name="reason" placeholder="เหตุผล" required /><button className={buttonClass} disabled={busy}>เปลี่ยนสถานะ</button></form><form className="grid gap-2 sm:grid-cols-3" onSubmit={(e) => void form(e, "/stock/write-off", (f) => ({ reference: `write-off:${str(lot.lot_number)}`, reason: f.get("reason"), items: [{ product_id: lot.product_id, lot_id: lot.id, quantity: Number(f.get("qty")) }] }))}><input aria-label="จำนวนตัดจำหน่ายหน่วยฐาน" className={inputClass} min="1" name="qty" placeholder="จำนวนฐาน" required step="1" type="number" /><input aria-label="เหตุผลตัดจำหน่าย" className={inputClass} name="reason" placeholder="เหตุผลตัดจำหน่าย" required /><button className={buttonClass} disabled={busy}>ตัดจำหน่าย</button></form></article>)}</div></> : null}
      {tab === "reservations" ? <><form className={panelClass} onSubmit={(e) => void form(e, "/reservations", (f) => ({ reference: f.get("reference"), expires_at: new Date(str(f.get("expires_at"))).toISOString(), items: stockLines() }))}><Text label="เอกสารอ้างอิง" name="reference" /><Text label="หมดอายุการจอง (เวลาของเครื่องนี้)" name="expires_at" type="datetime-local" /><V2Lines catalog={catalog} entries={entries} setEntries={setEntries} stock /><button className={buttonClass} disabled={busy}>จองสินค้า</button></form><div className="space-y-3">{rows(data.items).map((reservation) => <article className="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-4" key={str(reservation.id)}><div className="min-w-0 flex-1"><strong>{str(reservation.reference)}</strong><p className="text-sm">{str(reservation.status)} · {str(reservation.expires_at)}</p></div>{reservation.status === "active" ? <><button className={buttonClass} disabled={busy} onClick={() => void submit("/reservations/transition", { reservation_id: reservation.id, action: "consume" })}>จ่ายของที่จอง</button><button className="min-h-11 rounded-xl border px-4" disabled={busy} onClick={() => void submit("/reservations/transition", { reservation_id: reservation.id, action: "release" })}>คืนการจอง</button></> : null}</article>)}</div></> : null}
      {tab === "documents" ? <><form className={panelClass} onChange={() => setApprovalID("")} onSubmit={(e) => void form(e, "/documents", (f) => ({ ...documentPayload(f), approval_id: approvalID }))}><Field label="ประเภทเอกสาร"><select className={inputClass} onChange={(e) => setDocumentKind(e.target.value)} value={documentKind}><option value="quotation">ใบเสนอราคา</option><option value="ar_invoice">ใบขายเครดิตและจ่ายสินค้า</option><option value="ap_bill">ใบซื้อ/เจ้าหนี้</option></select></Field><Pick key={documentKind} label={documentKind === "ap_bill" ? "supplier" : "ลูกค้า"} name="counterparty" options={documentKind === "ap_bill" ? rows(catalog.suppliers) : customers} /><Text label="ครบกำหนด (ว่าง = เครดิตคู่ค้า)" name="due" required={false} type="date" /><Text label="หมายเหตุ" name="notes" required={false} />{documentKind !== "ap_bill" ? <Pick label="โปรโมชั่น (เลือกหนึ่งโปร)" name="promotion" options={rows(catalog.promotions).filter((p) => p.active === true)} required={false} /> : null}{documentKind !== "ap_bill" ? <><Text label="เหตุผลอนุมัติส่วนลดเกินเพดาน" name="approval_reason" required={false} /><button className="min-h-11 rounded-xl border px-4" disabled={busy} onClick={(e) => void approveDiscount(e.currentTarget.form)} type="button">{approvalID ? "อนุมัติแล้ว · อนุมัติใหม่" : "อนุมัติส่วนลดตามรายการ"}</button></> : null}<V2Lines catalog={catalog} entries={entries} setEntries={setEntries} stock={false} purchase={documentKind === "ap_bill"} /><button className={buttonClass} disabled={busy}>ออกเอกสาร</button></form><div className="space-y-3">{rows(data.items).map((doc) => <article className="flex flex-wrap items-center gap-3 rounded-xl border bg-card p-4" key={str(doc.id)}><div className="min-w-0 flex-1"><strong className="break-all">{str(doc.document_number)}</strong><p className="text-sm">{str(doc.kind)} · {str(doc.status)} {str(doc.payment_status)} · ยอด {money(doc.total_cents)} · ค้าง {money(doc.balance_cents)}</p></div><button className="min-h-11 rounded-xl border px-4" disabled={busy} onClick={() => { const generation = loadGeneration.current; void v2API<Row>(`/documents/${str(doc.id)}?branch_id=${branchID}`).then((next) => { if (generation === loadGeneration.current) setDetail(next); }).catch((caught) => { if (generation === loadGeneration.current) setError(caught.message); }); }}>รายละเอียด</button>{doc.kind === "quotation" && doc.status === "draft" ? <button className={buttonClass} disabled={busy} onClick={() => void submit("/documents/transition", { id: doc.id, action: "convert" })}>ออกใบขาย</button> : null}</article>)}</div>{detail ? <DocumentDetail key={str((detail.document as Row).id)} detail={detail} catalog={catalog} customers={customers} busy={busy} submit={submit} today={today} onClose={() => setDetail(null)} onError={setError} /> : null}</> : null}
      {tab === "finance" ? <><form className={panelClass} onSubmit={(e) => void form(e, "/payments", (f) => ({ direction: paymentDirection, ...(paymentDirection === "receive" ? { customer_id: f.get("counterparty") } : { supplier_id: f.get("counterparty") }), method: paymentMethod, amount_cents: cents(f.get("amount")), reference: str(f.get("reference")), drawer_id: str(f.get("drawer")), paid_on: today, cheque_number: str(f.get("cheque_number")), bank: str(f.get("bank")), cheque_due_on: str(f.get("cheque_due")), allocations: f.get("document") ? [{ document_id: f.get("document"), amount_cents: cents(f.get("allocation")) }] : [] }))}><Field label="รายการ"><select className={inputClass} onChange={(e) => setPaymentDirection(e.target.value)} value={paymentDirection}><option value="receive">รับเงินลูกค้า</option><option value="pay">จ่าย supplier</option></select></Field><Pick key={paymentDirection} label={paymentDirection === "receive" ? "ลูกค้า" : "supplier"} name="counterparty" options={paymentDirection === "receive" ? customers : rows(catalog.suppliers)} /><Field label="วิธีชำระ"><select className={inputClass} onChange={(e) => setPaymentMethod(e.target.value)} value={paymentMethod}><option value="bank_transfer">เงินโอน</option><option value="cash">เงินสด</option><option value="cheque">เช็ค</option></select></Field><Text label="ยอดเงิน (บาท)" name="amount" /><Text label="อ้างอิง" name="reference" required={false} /><Pick label="เอกสารจัดสรร (เลือกภายหลังได้)" name="document" options={rows(data.documents).filter((doc) => doc.kind === (paymentDirection === "receive" ? "ar_invoice" : "ap_bill") && Number(doc.balance_cents) > 0)} required={false} /><Text label="ยอดจัดสรร (บาท)" name="allocation" value="0" />{paymentMethod === "cash" ? <DrawerPick drawers={rows(catalog.drawers)} /> : null}{paymentMethod === "cheque" ? <><Text label="เลขเช็ค" name="cheque_number" /><Text label="ธนาคาร" name="bank" /><Text label="วันเช็ค" name="cheque_due" type="date" /></> : null}<button className={buttonClass} disabled={busy}>บันทึกเงิน</button></form><div className="grid gap-3 md:grid-cols-2">{rows(data.documents).map((doc) => <article className="rounded-xl border bg-card p-4" key={str(doc.id)}><strong className="break-all">{str(doc.document_number)}</strong><p>ค้าง {money(doc.balance_cents)} · เกินกำหนด {str(doc.overdue_days)} วัน</p></article>)}</div><div className="space-y-3">{rows(data.payments).map((payment) => <article className="space-y-3 rounded-xl border bg-card p-4" key={str(payment.id)}><p>{str(payment.direction)} · {str(payment.method)} · {money(payment.amount_cents)} · ยังไม่จัดสรร {money(payment.unallocated_cents)}</p><small className="block break-all">{str(payment.id)}</small>{Number(payment.unallocated_cents) > 0 ? <form className="grid gap-2 sm:grid-cols-3" onSubmit={(e) => void form(e, "/allocations", (f) => ({ payment_id: payment.id, allocations: [{ document_id: f.get("document"), amount_cents: cents(f.get("amount")) }] }))}><Pick label="เอกสาร" name="document" options={rows(data.documents).filter((doc) => str(doc.customer_id) === str(payment.customer_id) && str(doc.supplier_id) === str(payment.supplier_id) && Number(doc.balance_cents) > 0)} /><Text label="ยอดจัดสรร (บาท)" name="amount" /><button className={buttonClass} disabled={busy}>จัดสรรเงิน</button></form> : null}{payment.cheque_id ? <form className="grid gap-2 sm:grid-cols-3" onSubmit={(e) => void form(e, "/cheques/transition", (f) => ({ id: payment.cheque_id, revision: payment.cheque_revision, action: f.get("action"), reason: f.get("reason"), effective_on: today }))}><Field label={`เช็ค ${str(payment.cheque_number)} · ${str(payment.cheque_status)}`}><select className={inputClass} name="action"><option value="deposited">นำฝาก</option><option value="cleared">ผ่าน</option><option value="bounced">ตีกลับ</option><option value="cancelled">ยกเลิก</option></select></Field><Text label="หลักฐาน/เหตุผล" name="reason" /><button className={buttonClass} disabled={busy}>บันทึกสถานะเช็ค</button></form> : null}</article>)}</div></> : null}
      {tab === "expiry" ? <><button className={buttonClass} disabled={busy} onClick={() => void submit("/expiry/scan", {})}>สร้างงานเตือน 3/6/9 เดือน</button><div className="space-y-3">{rows(data.items).map((lot, index) => <article className="space-y-2 rounded-xl border bg-card p-4" key={`${str(lot.lot_id)}:${index}`}><strong>{str(lot.product_name)} · {str(lot.lot_number)}</strong><p className="text-sm">หมดอายุ {str(lot.expires_on)} · คงเหลือ {str(lot.remaining_quantity)} · มูลค่าล็อตประมาณ {money(lot.physical_lot_risk_estimate_cents)}</p>{lot.task_id ? <form className="grid gap-2 sm:grid-cols-3" onSubmit={(e) => void form(e, "/expiry/tasks", (f) => ({ id: lot.task_id, status: f.get("status"), note: f.get("note") }))}><Field label={`ช่วง ${str(lot.band_months)} เดือน · ${str(lot.status)}`}><select className={inputClass} name="status"><option value="acknowledged">รับทราบ</option><option value="closed">ปิดงาน</option></select></Field><Text label="บันทึกผล" name="note" /><button className={buttonClass} disabled={busy || lot.status === "closed"}>บันทึกงาน</button></form> : null}</article>)}</div></> : null}
      {tab === "drawers" ? <V2Drawers data={data} busy={busy} submit={submit} onError={setError} /> : null}
      {tab === "cancellations" ? <V2Refunds key={branchID} data={data} catalog={catalog} drawers={rows(catalog.drawers)} busy={busy} submit={submit} onError={setError} /> : null}
      {tab === "ledger" ? <div className="space-y-3">{[...rows(data.stock), ...rows(data.money)].map((item) => <article className="rounded-xl border bg-card p-3" key={str(item.id)}><strong>{str(item.event_type)}</strong><p className="text-sm">{str(item.created_at)} · {item.quantity_delta !== undefined ? `${str(item.quantity_delta)} หน่วยฐาน · ${money(item.value_delta_cents)}` : money(item.amount_cents)}</p><p className="break-all text-xs text-muted-foreground">{str(item.document_id || item.reference)} · {str(item.reason)}</p></article>)}</div> : null}
      <V2Workflows key={`${branchID}:${tab}`} tab={tab} data={data} catalog={catalog} customers={customers} branches={branches} branchID={branchID} busy={busy} submit={submit} onError={setError} />
    </>}
    {busy ? <p className="text-sm" role="status">กำลังบันทึก…</p> : null}
  </div>;
}

function DocumentDetail({ detail, catalog, customers, submit, busy, today, onClose, onError }: { detail: Row; catalog: Row; customers: Row[]; submit: (path: string, data: Row) => Promise<void>; busy: boolean; today: string; onClose: () => void; onError: (error: string) => void }) {
  const document = detail.document as Row;
  const [quantities, setQuantities] = useState<Record<string, string>>({});
  const [reason, setReason] = useState("");
  return <section aria-label="รายละเอียดเอกสาร" className="space-y-3 rounded-2xl border bg-card p-4"><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="break-all font-semibold">{str(document.document_number)}</h2><button className="min-h-11 rounded-xl border px-4" onClick={onClose}>ปิดรายละเอียด</button></div>{rows(detail.lines).map((line) => <div className="grid gap-2 border-b pb-3 sm:grid-cols-3" key={str(line.id)}><div><strong>{str(line.description)}</strong><p className="text-sm">{str(line.quantity)} {str((line.unit_snapshot as Row)?.name)} · ฐาน {str(line.base_quantity)}</p></div><p>{money(line.total_cents)}</p>{(document.kind === "ar_invoice" || document.kind === "ap_bill") && document.status === "issued" ? <Field label="จำนวนลดหนี้ (หน่วยที่ขาย/ซื้อ)"><input className={inputClass} min="0" max={Number(line.quantity)} onChange={(e) => setQuantities({ ...quantities, [str(line.id)]: e.target.value })} step="1" type="number" value={quantities[str(line.id)] || "0"} /></Field> : null}</div>)}{(document.kind === "ar_invoice" || document.kind === "ap_bill") && document.status === "issued" ? <><Field label="เหตุผลลดหนี้"><input className={inputClass} onChange={(e) => setReason(e.target.value)} value={reason} /></Field><p className="text-sm text-muted-foreground">ใบลดหนี้ปรับยอดทางการเงิน การรับสินค้าคืนต้องทำรายการแยก</p><button className={buttonClass} disabled={busy || !reason.trim()} onClick={() => { const lines = Object.entries(quantities).filter(([, qty]) => Number(qty) > 0).map(([id, qty]) => ({ source_line_id: id, quantity: Number(qty) })); if (!lines.length) { onError("เลือกรายการลดหนี้ก่อน"); return; } void submit("/credit-notes", { document_id: document.id, issued_on: today, reason, lines }); }}>ออกใบลดหนี้</button></> : null}<p className="text-sm">ยอดเอกสาร {money(document.total_cents)}</p><V2CancellationForm document={document} busy={busy} submit={submit} onError={onError} /><V2DocumentTools detail={detail} catalog={catalog} customers={customers} busy={busy} today={today} submit={submit} onError={onError} /></section>;
}
