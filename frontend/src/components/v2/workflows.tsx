"use client";

import { cloneElement, isValidElement, useId, useState, type FormEvent, type ReactNode } from "react";
import { v2API } from "@/services/v2";
import { DrawerPick } from "./refund-shifts";
import { V2Lines, blankLine, type Entry } from "./line-editor";
import { V2Promotions } from "./promotions";

type Row = Record<string, unknown>;
type Props = { tab: string; data: Row; catalog: Row; customers: Row[]; branches: Array<{id: string;name: string}>; branchID: string; busy: boolean; submit: (path: string, payload: Row) => Promise<void>; onError: (message: string) => void };
const field = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
const button = "min-h-11 rounded-xl bg-primary px-4 py-2 font-semibold text-primary-foreground disabled:opacity-50";
const panel = "grid gap-3 rounded-2xl border bg-card p-4 sm:grid-cols-2";
function text(value: unknown) { return value == null ? "" : String(value); }
function items(value: unknown): Row[] { return Array.isArray(value) ? value as Row[] : []; }
function money(value: unknown) { return new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" }).format(Number(value || 0) / 100); }
function amount(value: FormDataEntryValue | null) {
  const raw = text(value);
  if (!/^\d+(\.\d{1,2})?$/.test(raw)) throw new Error("ระบุจำนวนเงินไม่ติดลบ ทศนิยมไม่เกิน 2 ตำแหน่ง");
  const [integer, fraction = ""] = raw.split(".");
  const cents = Number(integer) * 100 + Number(fraction.padEnd(2, "0"));
  if (!Number.isSafeInteger(cents)) throw new Error("ยอดเงินเกินขอบเขต");
  return cents;
}
function Label({ title, children }: { title: string; children: ReactNode }) {
  const id = useId();
  return <label className="grid gap-1 text-sm"><span id={id}>{title}</span>{isValidElement<{ "aria-labelledby"?: string }>(children) ? cloneElement(children, { "aria-labelledby": id }) : children}</label>;
}
function Input({ title, name, type = "text", required = true, value }: { title: string; name: string; type?: string; required?: boolean; value?: string }) {
  return <Label title={title}><input className={field} name={name} type={type} required={required} defaultValue={value} min={type === "number" ? 0 : undefined} step={type === "number" ? 1 : undefined} /></Label>;
}
function Select({ title, name, options, required = true }: { title: string; name: string; options: Row[]; required?: boolean }) {
  return <Label title={title}><select className={field} name={name} required={required}><option value="">เลือก{title}</option>{options.map((row) => <option key={text(row.id)} value={text(row.id)}>{text(row.name || row.document_number || row.id)}</option>)}</select></Label>;
}

export function V2Workflows({ tab, data, catalog, customers, branches, branchID, busy, submit, onError }: Props) {
  const [selectedProducts, setSelectedProducts] = useState<string[]>([]);
  const [report, setReport] = useState<Row | null>(null);
  const [loading, setLoading] = useState(false);
  const [sourceID, setSourceID] = useState("");
  const [creditKind, setCreditKind] = useState("apply");
  const [refundMethod, setRefundMethod] = useState("bank_transfer");
  const [productID, setProductID] = useState("");
  const [trace, setTrace] = useState<Row | null>(null);
  const [shipmentLines, setShipmentLines] = useState<Entry[]>([blankLine()]);
  async function form(event: FormEvent<HTMLFormElement>, path: string, build: (f: FormData) => Row) {
    event.preventDefault();
    try { await submit(path, build(new FormData(event.currentTarget))); }
    catch (caught) { onError(caught instanceof Error ? caught.message : "ข้อมูลไม่ถูกต้อง"); }
  }
  if (tab === "promotions") return <V2Promotions data={data} catalog={catalog} busy={busy} submit={submit} onError={onError} />;
  if (tab === "customers") return <div className="space-y-3">{customers.map((c) => <details className="rounded-xl border bg-card p-4" key={`${text(c.id)}:${text(c.revision)}`}><summary className="cursor-pointer font-medium">แก้ข้อมูล {text(c.name)} · {c.active ? "เปิดใช้" : "ปิดใช้"}</summary><form className={`${panel} mt-3`} onSubmit={(e) => void form(e, "/customers", (f) => ({ id: c.id, revision: c.revision, code: f.get("code"), type: c.customer_type, name: f.get("name"), tax_id: text(f.get("tax")), billing_address: text(f.get("billing")), shipping_address: text(f.get("shipping")), phone: text(f.get("phone")), email: text(f.get("email")), credit_days: Number(f.get("days")), credit_limit_cents: amount(f.get("limit")), active: f.get("active") === "on" }))}><Input title="รหัส" name="code" value={text(c.customer_code)} /><Input title="ชื่อ" name="name" value={text(c.name)} /><Input title="เลขภาษี" name="tax" value={text(c.tax_id)} required={false} /><Input title="ที่อยู่ออกเอกสาร" name="billing" value={text(c.billing_address)} required={false} /><Input title="ที่อยู่จัดส่ง" name="shipping" value={text(c.shipping_address)} required={false} /><Input title="โทรศัพท์" name="phone" value={text(c.phone)} required={false} /><Input title="อีเมล" name="email" type="email" value={text(c.email)} required={false} /><Input title="เครดิต (วัน)" name="days" type="number" value={text(c.credit_days)} /><Input title="วงเงิน (บาท)" name="limit" value={(Number(c.credit_limit_cents) / 100).toFixed(2)} /><label className="flex items-center gap-2 text-sm"><input name="active" type="checkbox" defaultChecked={Boolean(c.active)} />เปิดใช้ลูกค้า</label><button className={button} disabled={busy}>บันทึกแก้ไข</button></form></details>)}</div>;
  if (tab === "inventory") return <details className="rounded-2xl border bg-card p-4"><summary className="cursor-pointer font-semibold">กักกันบางจำนวนจากล็อตพร้อมขาย</summary><div className="mt-3 space-y-3">{items(data.items).filter((l) => l.state === "available" && Number(l.remaining_quantity) > 0).map((l) => <form className={panel} key={text(l.id)} onSubmit={(e) => void form(e, "/lots/quarantine-part", (f) => ({ lot_id: l.id, base_quantity: Number(f.get("qty")), reason: f.get("reason") }))}><p className="text-sm sm:col-span-2">{text(l.product_name)} · ล็อต {text(l.lot_number)} · คงเหลือ {text(l.remaining_quantity)} ฐาน</p><Input title="จำนวนกักกัน (ฐาน)" name="qty" type="number" /><Input title="เหตุผลกักกัน" name="reason" /><button className={button} disabled={busy}>แยกจำนวนเข้ากักกัน</button></form>)}</div></details>;
  if (tab === "shipments") return <div className="space-y-4">
    <form className={panel} onSubmit={(e) => void form(e, "/shipments", (f) => ({ destination_branch_id: f.get("destination"), reference: f.get("reference"), reason: text(f.get("reason")), items: shipmentLines.map((line) => ({ product_id: line.product_id, unit_id: line.unit_id, quantity: Number(line.quantity) })) }))}>
      <Select title="สาขาปลายทาง" name="destination" options={branches.filter((b) => b.id !== branchID)} /><Input title="เอกสารอ้างอิง" name="reference" /><Input title="หมายเหตุส่ง" name="reason" required={false} />
      <V2Lines catalog={catalog} entries={shipmentLines} setEntries={setShipmentLines} stock /><button className={button} disabled={busy}>จ่ายต้นทางเข้าระหว่างทาง</button>
    </form>
    {items(data.items).map((sh) => <article className="space-y-3 rounded-2xl border bg-card p-4" key={text(sh.id)}><strong>{text(sh.reference)} · {text(sh.status)}</strong><p className="break-all text-sm">{branches.find((b) => b.id === sh.source_branch_id)?.name} → {branches.find((b) => b.id === sh.destination_branch_id)?.name}</p><div>{items(sh.lines).map((l) => <p className="text-sm" key={text(l.id)}>{text(l.product_name)} · ล็อต {text(l.lot_number)} · ส่ง {text(l.base_quantity)} · ค้าง {text(l.in_transit_quantity)} หน่วยฐาน</p>)}</div>
      {sh.status === "dispatched" || sh.status === "partial" ? <form className="grid gap-3 sm:grid-cols-2" onSubmit={(e) => void form(e, "/shipments/receive", (f) => ({ shipment_id: sh.id, action: text(sh.destination_branch_id) === branchID ? "receive" : "return_to_source", quarantine: f.get("quarantine") === "on", reason: text(f.get("reason")), lines: items(sh.lines).map((l) => ({ id: l.id, base_quantity: Number(f.get(`qty:${text(l.id)}`)) })).filter((l) => l.base_quantity > 0) }))}>{items(sh.lines).filter((l) => Number(l.in_transit_quantity) > 0).map((l) => <Input key={text(l.id)} title={`รับ ${text(l.product_name)} ล็อต ${text(l.lot_number)} (ฐาน)`} name={`qty:${text(l.id)}`} value="0" type="number" />)}<Input title="หลักฐาน/เหตุผลรับคืนส่วนค้าง" name="reason" required={text(sh.source_branch_id) === branchID} /><label className="flex items-center gap-2 text-sm"><input name="quarantine" type="checkbox" />รับเข้ากักกัน</label><p className="text-sm text-muted-foreground sm:col-span-2">รับบางส่วนหลายครั้งได้ ต้นทางใช้รายการนี้รับส่วนค้างกลับเข้ากักกัน ปลายทางใช้รับของที่มาถึง</p><button className={button} disabled={busy}>{text(sh.destination_branch_id) === branchID ? "บันทึกรับปลายทาง" : "รับส่วนค้างกลับต้นทาง"}</button></form> : null}
    </article>)}
  </div>;
  if (tab === "counts") return <div className="space-y-4">
    <form className={panel} onSubmit={(e) => void form(e, "/counts", (f) => ({ reference: f.get("reference"), product_ids: selectedProducts }))}>
      <Input title="อ้างอิงตรวจนับ" name="reference" />
      <Label title="ขอบเขตสินค้าที่ตรวจนับ"><select className={`${field} min-h-36`} multiple value={selectedProducts} onChange={(e) => setSelectedProducts(Array.from(e.target.selectedOptions, (o) => o.value))}>{items(catalog.products).map((p) => <option key={text(p.id)} value={text(p.id)}>{text(p.name)}</option>)}</select></Label>
      <p className="text-sm text-muted-foreground sm:col-span-2">นับรวมของจอง กักกัน และหมดอายุภายในล็อต ขณะบันทึกจำนวนจะเก็บยอดปัจจุบันประกอบ snapshot หากมี movement ภายหลังต้องเปิดรอบใหม่</p>
      <button className={button} disabled={busy || !selectedProducts.length}>เปิดรอบตรวจนับ</button>
    </form>
    {items(data.items).map((count) => <article className="space-y-3 rounded-2xl border bg-card p-4" key={text(count.id)}>
      <h2 className="font-semibold">{text(count.reference)} · {text(count.status)}</h2>
      {count.status === "open" ? <form className="space-y-3" onSubmit={(e) => void form(e, "/counts/observe", (f) => ({ id: count.id, revision: count.revision, lines: items(count.lines).map((line) => ({ snapshot_id: line.id, counted_quantity: Number(f.get(`qty:${text(line.id)}`)), increase_cost_cents: amount(f.get(`cost:${text(line.id)}`)), reason: text(f.get(`reason:${text(line.id)}`)) })) }))}>
        {items(count.lines).map((line) => <fieldset className="grid gap-3 rounded-xl border p-3 sm:grid-cols-3" key={text(line.id)}><legend>{text(line.product_name)} · ล็อต {text(line.lot_number)}</legend><p className="text-sm sm:col-span-3">snapshot {text(line.snapshot_quantity)} · ปัจจุบัน {text(line.current_quantity)} · จองเมื่อเปิด {text(line.reserved_quantity)} · {text(line.state)} · หมดอายุ {text(line.expires_on) || "ไม่กำหนด"}</p><Input title="จำนวนที่นับ (หน่วยฐาน)" name={`qty:${text(line.id)}`} type="number" /><Input title="ต้นทุนรวมส่วนที่นับเกิน (บาท)" name={`cost:${text(line.id)}`} value="0" /><Input title="เหตุผลส่วนต่าง" name={`reason:${text(line.id)}`} required={false} /></fieldset>)}
        <button className={button} disabled={busy}>บันทึกจำนวนทุกล็อต</button>
      </form> : <div className="space-y-2">{items(count.lines).map((line) => { const o = line.observation as Row | null; return <p className="text-sm" key={text(line.id)}>{text(line.product_name)} · {text(line.lot_number)} · ระบบ {text(o?.expected_quantity)} · นับ {text(o?.counted_quantity)} · {text(o?.reason)}</p>; })}</div>}
      {count.status === "open" || count.status === "counted" ? <form className="grid gap-3 sm:grid-cols-3" onSubmit={(e) => void form(e, "/counts/transition", (f) => ({ id: count.id, revision: count.revision, action: f.get("action"), reason: f.get("reason") }))}><Label title="ดำเนินการ"><select className={field} name="action">{count.status === "counted" ? <option value="confirm">อนุมัติและปรับสต๊อก</option> : null}<option value="cancel">ยกเลิกรอบ</option></select></Label><Input title="เหตุผลผู้อนุมัติ" name="reason" /><button className={button} disabled={busy}>บันทึก</button></form> : null}
    </article>)}
  </div>;
  if (tab === "returns") return <div className="space-y-4"><p className="text-sm">คืนสินค้าเป็นหน่วยฐานตาม movement ขายเดิม รับเข้ากักกันและต้นทุนเดิม การลดหนี้/คืนเงินทำแยกต่างหาก</p>
    {items(data.sources).filter((source) => Number(source.returnable_base_quantity) > 0).map((source) => <form className={panel} key={text(source.id)} onSubmit={(e) => void form(e, "/returns", (f) => ({ source_event_id: source.id, base_quantity: Number(f.get("qty")), reason: f.get("reason") }))}><div className="sm:col-span-2"><strong>{text(source.product_name)} · ล็อต {text(source.lot_number)}</strong><p className="break-all text-sm">{text(source.document_number)} · คืนได้ {text(source.returnable_base_quantity)} หน่วยฐาน</p></div><Input title="จำนวนคืน (ฐาน)" name="qty" type="number" /><Input title="เหตุผลรับคืน" name="reason" /><button className={button} disabled={busy}>รับคืนเข้ากักกัน</button></form>)}
    {items(data.items).map((row) => <p className="rounded-xl border p-3 text-sm" key={text(row.id)}>คืน {text(row.base_quantity)} หน่วยฐาน · ต้นทุน {money(row.value_cents)} · {text(row.reason)}</p>)}
  </div>;
  if (tab === "credits") {
    const documents = items(data.documents);
    const source = documents.find((d) => text(d.id) === sourceID);
    return <form className={panel} onSubmit={(e) => void form(e, "/credits/use", (f) => ({ source_document_id: sourceID, target_document_id: creditKind === "apply" ? f.get("target") : "", kind: creditKind, amount_cents: amount(f.get("amount")), method: creditKind === "refund" ? refundMethod : "", drawer_id: text(f.get("drawer")), reference: f.get("reference"), reason: f.get("reason") }))}>
      <Label title="เครดิตจากเอกสาร"><select className={field} required value={sourceID} onChange={(e) => setSourceID(e.target.value)}><option value="">เลือกเครดิต</option>{documents.filter((d) => Number(d.balance_cents) < 0).map((d) => <option key={text(d.id)} value={text(d.id)}>{text(d.document_number)} · {money(-Number(d.balance_cents))}</option>)}</select></Label>
      <Label title="การใช้เครดิต"><select className={field} value={creditKind} onChange={(e) => setCreditKind(e.target.value)}><option value="apply">นำไปหักหนี้อีกใบ</option><option value="refund">คืน/รับคืนเงิน</option></select></Label>
      {creditKind === "apply" ? <Select title="เอกสารหนี้ปลายทาง" name="target" options={documents.filter((d) => source && d.kind === source.kind && text(d.customer_id) === text(source.customer_id) && text(d.supplier_id) === text(source.supplier_id) && Number(d.balance_cents) > 0)} /> : <><Label title="วิธีคืนเงิน"><select className={field} value={refundMethod} onChange={(e) => setRefundMethod(e.target.value)}><option value="bank_transfer">โอน</option><option value="cash">เงินสด</option></select></Label>{refundMethod === "cash" ? <DrawerPick drawers={items(catalog.drawers)} /> : null}</>}
      <Input title="จำนวนเงิน (บาท)" name="amount" /><Input title="อ้างอิง" name="reference" /><Input title="เหตุผล" name="reason" /><p className="text-sm text-muted-foreground sm:col-span-2">เครดิตคงเหลือเกิดเมื่อยอดลดหนี้มากกว่ายอดหนี้ที่ยังค้าง คืนเงินไม่ทำรายการรับสินค้าเอง</p><button className={button} disabled={busy}>บันทึกใช้เครดิต</button>
    </form>;
  }
  if (tab === "price-rules") return <div className="space-y-4"><form className={panel} onSubmit={(e) => void form(e, "/price-rules", (f) => ({ customer_id: text(f.get("customer")), product_id: productID, unit_id: text(f.get("unit")), min_base_quantity: Number(f.get("minimum")), unit_price_cents: amount(f.get("price")), starts_on: f.get("start"), ends_on: text(f.get("end")) }))}>
    <Select title="ลูกค้า (ว่าง = ราคาส่งทั่วไป)" name="customer" options={customers} required={false} /><Label title="สินค้า"><select className={field} required value={productID} onChange={(e) => setProductID(e.target.value)}><option value="">เลือกสินค้า</option>{items(catalog.products).map((p) => <option key={text(p.id)} value={text(p.id)}>{text(p.name)}</option>)}</select></Label><Select key={productID} title="หน่วย (ว่าง = หน่วยฐาน)" name="unit" options={items(catalog.units).filter((u) => text(u.product_id) === productID).map((u) => ({ ...u, name: u.unit_name }))} required={false} /><Input title="ขั้นต่ำรวมสินค้าเดียวกัน (หน่วยฐาน)" name="minimum" type="number" value="1" /><Input title="ราคาต่อหน่วยที่เลือก (บาท)" name="price" /><Input title="เริ่มใช้" name="start" type="date" /><Input title="สิ้นสุด (เว้นว่างได้)" name="end" type="date" required={false} /><button className={button} disabled={busy}>เพิ่มราคา</button>
  </form>{items(data.items).map((rule) => <p className="rounded-xl border p-3 text-sm" key={text(rule.id)}>{text(items(catalog.products).find((p) => p.id === rule.product_id)?.name)} · ขั้นต่ำ {text(rule.min_base_quantity)} ฐาน · ราคา {money(rule.unit_price_cents)} · {text(rule.starts_on)} – {text(rule.ends_on) || "ไม่กำหนด"}</p>)}</div>;
  if (tab === "statements") {
    const view = report || data;
    return <div className="space-y-4"><form className={panel} onSubmit={(event) => { event.preventDefault(); const f = new FormData(event.currentTarget); const query = new URLSearchParams({ branch_id: branchID, from: text(f.get("from")), to: text(f.get("to")), customer_id: text(f.get("customer")) }); setLoading(true); void v2API<Row>(`/statements?${query}`).then(setReport).catch((e) => onError(e.message)).finally(() => setLoading(false)); }}><Input title="วันที่เริ่ม" name="from" type="date" /><Input title="ข้อมูล ณ วันที่" name="to" type="date" /><Select title="ลูกค้า (ว่าง = ทั้งหมด)" name="customer" options={customers} required={false} /><button className={button} disabled={loading}>แสดง statement</button></form>
      {items(view.aging).map((a) => <article className="rounded-xl border bg-card p-4" key={text(a.kind)}><strong>{text(a.kind)}</strong><p className="text-sm">ยังไม่ครบกำหนด {money(a.current_cents)} · 1–30 วัน {money(a.days_1_30_cents)} · 31–60 วัน {money(a.days_31_60_cents)} · 61–90 วัน {money(a.days_61_90_cents)} · เกิน 90 วัน {money(a.days_91_plus_cents)} · เครดิต {money(a.credit_cents)}</p></article>)}
      {items(view.items).map((d) => <article className="rounded-xl border bg-card p-4" key={text(d.id)}><strong className="break-all">{text(d.document_number)}</strong><p className="text-sm">ยกมา {money(d.opening_cents)} · เพิ่ม {money(d.increases_cents)} · ลด {money(d.decreases_cents)} · คงเหลือ {money(d.closing_cents)} · ครบกำหนด {text(d.due_on)}</p></article>)}
      {items(view.events).map((e) => <p className="rounded-xl border p-3 text-sm" key={text(e.id)}>{text(e.effective_on)} · {text(e.document_number)} · {text(e.event_type)} · {money(e.amount_cents)}</p>)}
    </div>;
  }
  if (tab === "stock-report") return <div className="space-y-4"><p className="text-sm text-muted-foreground">มูลค่าจากต้นทุนคงเหลือของสินค้า กระจายตามจำนวนคงเหลือแต่ละล็อต เศษสตางค์รวมไว้ที่ล็อตตามลำดับ ID ใช้ดูความเสี่ยงแยกจากต้นทุนตัดจำหน่ายจริง</p>
    {items(data.reconciliation).map((r) => <article className={`rounded-xl border bg-card p-3 ${r.base_quantity !== r.lot_quantity || r.base_quantity !== r.ledger_quantity || r.value_cents !== r.ledger_value_cents ? "border-destructive" : ""}`} key={text(r.product_id)}><strong>{text(items(catalog.products).find((p) => p.id === r.product_id)?.name)}</strong><p className="text-sm">ยอดบัญชี {text(r.base_quantity)} · ล็อต {text(r.lot_quantity)} · ledger {text(r.ledger_quantity)} · ต้นทุน {money(r.value_cents)} · ledger {money(r.ledger_value_cents)}</p></article>)}
    {items(data.lots).map((l) => <article className="rounded-xl border bg-card p-3" key={text(l.id)}><strong>{text(l.product_name)} · {text(l.lot_number)}</strong><p className="text-sm">{text(l.remaining_quantity)} ฐาน · {text(l.cost_method)} · มูลค่าที่กระจาย {money(l.carrying_value_cents)} · หมดอายุ {text(l.expires_on) || "ไม่กำหนด"}</p><button className="min-h-11 rounded-xl border px-4" disabled={loading} onClick={() => { setLoading(true); void v2API<Row>(`/lots/${text(l.id)}/trace?branch_id=${branchID}`).then(setTrace).catch((e) => onError(e.message)).finally(() => setLoading(false)); }}>ติดตามล็อต</button></article>)}
    {trace ? <section className="space-y-2 rounded-xl border p-4"><h2 className="font-semibold">เส้นทางล็อต</h2>{items(trace.lots).map((l) => <p className="text-sm" key={text(l.id)}>{text(l.lot_number)} · สาขา {text(l.branch_id)} · {text(l.state)} · คงเหลือ {text(l.remaining_quantity)}</p>)}{items(trace.movements).map((m) => { const e = m.event as Row; const customer = m.customer_snapshot as Row | null; return <p className="text-sm" key={text(e.id)}>{text(e.event_type)} · {text(e.quantity_delta)} ฐาน · {text(m.document_number || e.reference)} · {customer ? text(customer.name) : "รายการนี้ไม่มีข้อมูลระบุตัวลูกค้า"}</p>; })}</section> : null}
    <h2 className="font-semibold">ตัดจำหน่ายจริง</h2>{items(data.write_offs).map((w) => <p className="rounded-xl border p-3 text-sm" key={text(w.id)}>{text(w.created_at)} · {text(w.reference)} · {money(-Number(w.value_delta_cents))} · {text(w.reason)}</p>)}
  </div>;
  if (tab === "jobs") return <div className="space-y-3"><p className="text-sm text-muted-foreground">งานภายใน V2 ต้องเปิด worker แยกตามการตั้งค่า ไม่มีการส่ง LINE/SMS จากหน้านี้</p>{items(data.items).map((job) => <article className="space-y-2 rounded-xl border bg-card p-4" key={text(job.id)}><strong>{text(job.event_type)} · {text(job.status)}</strong><p className="text-sm">พยายาม {text(job.attempts)} ครั้ง · รอบถัดไป {text(job.available_at)} · {text(job.last_error)}</p>{job.status === "dead" ? <form className="grid gap-3 sm:grid-cols-2" onSubmit={(e) => void form(e, "/jobs/retry", (f) => ({ id: job.id, reason: f.get("reason") }))}><Input title="เหตุผลลองใหม่" name="reason" /><button className={button} disabled={busy}>Retry</button></form> : null}</article>)}</div>;
  return null;
}
