"use client";

import { useState, type FormEvent } from "react";
type Row = Record<string, unknown>;
const str = (v: unknown) => v == null ? "" : String(v);
const rows = (v: unknown): Row[] => Array.isArray(v) ? v as Row[] : [];
const money = (v: unknown) => new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" }).format(Number(v || 0) / 100);
const field = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
function parseMoney(value: FormDataEntryValue | string | null) { const raw = str(value); if (!/^\d+(\.\d{1,2})?$/.test(raw)) throw new Error("ส่วนลดต้องเป็นเงินไม่ติดลบ ทศนิยมไม่เกิน 2 ตำแหน่ง"); const [whole, fraction = ""] = raw.split("."); const result = Number(whole) * 100 + Number(fraction.padEnd(2, "0")); if (!Number.isSafeInteger(result)) throw new Error("ยอดเงินเกินขอบเขต"); return result; }

export function V2DocumentTools({ detail, catalog, customers, busy, today, submit, onError }: { detail: Row; catalog: Row; customers: Row[]; busy: boolean; today: string; submit: (path: string, payload: Row) => Promise<void>; onError: (message: string) => void }) {
  const document = detail.document as Row;
  const customer = document.counterparty_snapshot as Row;
  const [editing, setEditing] = useState(false);
  const [entries, setEntries] = useState(rows(detail.lines).filter((l) => !str(l.price_source).startsWith("giveaway:")).map((l) => ({ product_id: str(l.product_id), unit_id: str((l.unit_snapshot as Row).id), quantity: str(l.quantity), discount: "0", vat: str(Number(l.vat_bps) / 100) })));
  const change = (index: number, key: keyof typeof entries[number], value: string) => setEntries(entries.map((l, i) => i === index ? { ...l, [key]: value, ...(key === "product_id" ? { unit_id: "" } : {}) } : l));
  async function revise(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const f = new FormData(event.currentTarget);
    try {
      await submit("/quotations/revise", { id: document.id, reason: f.get("reason"), document: { kind: "quotation", customer_id: f.get("customer"), promotion_id: str(f.get("promotion")), issued_on: today, due_on: str(f.get("due")), notes: str(f.get("notes")), lines: entries.map((l) => ({ product_id: l.product_id, unit_id: l.unit_id, quantity: Number(l.quantity), discount_cents: parseMoney(l.discount), vat_bps: Math.round(Number(l.vat) * 100) })) } });
    } catch (e) { onError(e instanceof Error ? e.message : "แก้ไขใบเสนอราคาไม่ได้"); }
  }
  return <div className="space-y-3">
    <div className="v2-document-print hidden print:block">
      <h1 className="text-xl font-bold">{document.kind === "quotation" ? "ใบเสนอราคา" : document.kind === "ar_invoice" ? "ใบขายเครดิต" : document.kind === "ap_bill" ? "ใบซื้อ" : "ใบลดหนี้"} · V2</h1>
      <p>{str(document.document_number)}</p><p>วันที่ {str(document.issued_on)} · ครบกำหนด {str(document.due_on)}</p>
      <p>คู่ค้า {str(customer.name || customer.legal_name)}</p><p>{str(customer.billing_address || customer.address)}</p><p>เลขภาษี {str(customer.tax_id)}</p>
      <table className="mt-4 w-full border-collapse text-sm"><thead><tr><th className="border p-2 text-left">สินค้า</th><th className="border p-2">จำนวน/หน่วย</th><th className="border p-2">ราคา</th><th className="border p-2">ส่วนลด</th><th className="border p-2">VAT</th><th className="border p-2">รวม</th></tr></thead><tbody>{rows(detail.lines).map((l) => <tr key={str(l.id)}><td className="border p-2">{str(l.description)}</td><td className="border p-2">{str(l.quantity)} {str((l.unit_snapshot as Row).name)}</td><td className="border p-2">{money(l.unit_price_cents)}</td><td className="border p-2">{money(l.discount_cents)}</td><td className="border p-2">{money(l.vat_cents)}</td><td className="border p-2">{money(l.total_cents)}</td></tr>)}</tbody></table>
      <p className="mt-3">รวม VAT {money(document.vat_cents)} · ยอดรวม {money(document.total_cents)}</p><p>{str(document.notes)}</p><p className="mt-4 text-xs">เอกสารพื้นที่ทดลอง V2 · รอตรวจรูปแบบบริษัทและเอกสารก่อนใช้งานจริง</p>
    </div>
    <button className="v2-no-print min-h-11 rounded-xl border px-4" onClick={() => window.print()} type="button">พิมพ์ตัวอย่างเอกสาร</button>
    {document.kind === "quotation" && document.status === "draft" ? <button className="v2-no-print min-h-11 rounded-xl border px-4" onClick={() => setEditing(!editing)} type="button">แก้ใบเสนอราคา/สร้าง revision</button> : null}
    {editing ? <form className="v2-no-print grid gap-3 rounded-xl border p-3 sm:grid-cols-2" onSubmit={(e) => void revise(e)}>
      <label className="grid gap-1 text-sm">ลูกค้า<select className={field} required name="customer" defaultValue={str(document.customer_id)}>{customers.filter((c) => c.active).map((c) => <option key={str(c.id)} value={str(c.id)}>{str(c.name)}</option>)}</select></label>
      <label className="grid gap-1 text-sm">โปรโมชั่น<select className={field} name="promotion" defaultValue={str((detail.promotion as Row | null)?.promotion_id)}><option value="">ไม่ใช้โปรโมชั่น</option>{rows(catalog.promotions).filter((p) => p.active).map((p) => <option key={str(p.id)} value={str(p.id)}>{str(p.name)}</option>)}</select></label>
      {entries.map((l, i) => <fieldset className="col-span-full grid gap-3 rounded-xl border p-3 sm:grid-cols-2" key={i}><legend>รายการ {i + 1}</legend>
        <label className="grid gap-1 text-sm">สินค้า<select className={field} required value={l.product_id} onChange={(e) => change(i, "product_id", e.target.value)}><option value="">เลือกสินค้า</option>{rows(catalog.products).map((p) => <option key={str(p.id)} value={str(p.id)}>{str(p.name)}</option>)}</select></label>
        <label className="grid gap-1 text-sm">หน่วย<select className={field} value={l.unit_id} onChange={(e) => change(i, "unit_id", e.target.value)}><option value="">หน่วยฐาน</option>{rows(catalog.units).filter((u) => u.product_id === l.product_id).map((u) => <option key={str(u.id)} value={str(u.id)}>{str(u.unit_name)}</option>)}</select></label>
        <label className="grid gap-1 text-sm">จำนวน<input className={field} type="number" min="1" step="1" required value={l.quantity} onChange={(e) => change(i, "quantity", e.target.value)} /></label>
        <label className="grid gap-1 text-sm">ส่วนลดรายการก่อนโปร (บาท)<input className={field} inputMode="decimal" required value={l.discount} onChange={(e) => change(i, "discount", e.target.value)} /></label>
        <label className="grid gap-1 text-sm">VAT (%)<input className={field} type="number" min="0" max="100" step="0.01" required value={l.vat} onChange={(e) => change(i, "vat", e.target.value)} /></label>
        <button className="min-h-11 text-left text-destructive" disabled={entries.length === 1 || busy} type="button" onClick={() => setEntries(entries.filter((_, n) => n !== i))}>ลบรายการ</button>
      </fieldset>)}
      <button className="min-h-11 rounded-xl border px-4" type="button" disabled={busy} onClick={() => setEntries([...entries, { product_id: "", unit_id: "", quantity: "1", discount: "0", vat: "7" }])}>+ เพิ่มรายการ</button>
      <label className="grid gap-1 text-sm">วันครบกำหนด<input className={field} name="due" type="date" min={today} defaultValue={str(document.due_on) < today ? today : str(document.due_on)} /></label><label className="grid gap-1 text-sm">หมายเหตุ<input className={field} name="notes" defaultValue={str(document.notes)} /></label><label className="grid gap-1 text-sm">เหตุผล revision<input className={field} name="reason" required /></label>
      <p className="text-sm text-muted-foreground sm:col-span-2">ฉบับใหม่คำนวณราคาปัจจุบันตาม price rule ต้องระบุส่วนลดก่อนโปรใหม่ เพราะส่วนลดในฉบับเดิมรวมโปรไว้แล้ว ของแถมคำนวณใหม่ตามโปรที่เลือก ฉบับเดิมเก็บไว้และออกใบขายต่อไม่ได้</p>
      <label className="flex min-h-11 items-center gap-2 text-sm"><input required type="checkbox" />ตรวจรายการ ส่วนลด และโปรโมชั่นของฉบับใหม่แล้ว</label><button className="min-h-11 rounded-xl bg-primary px-4 text-primary-foreground" disabled={busy}>บันทึก revision</button>
    </form> : null}
    {document.kind === "quotation" && document.status === "draft" ? <form className="v2-no-print flex flex-wrap gap-3 rounded-xl border p-3" onSubmit={(e) => { e.preventDefault(); void submit("/documents/transition", { id: document.id, action: "cancel", reason: new FormData(e.currentTarget).get("reason") }); }}><label className="grid flex-1 gap-1 text-sm">เหตุผลยกเลิกใบเสนอราคา<input className={field} required name="reason" /></label><button className="min-h-11 rounded-xl border px-4" disabled={busy}>ยกเลิกใบเสนอราคา</button></form> : null}
    <style>{`@media print {body * {visibility:hidden} .v2-document-print,.v2-document-print * {visibility:visible} .v2-document-print {display:block!important;position:absolute;left:0;top:0;width:100%;padding:24px;color:black;background:white} .v2-no-print {display:none!important}}`}</style>
  </div>;
}
