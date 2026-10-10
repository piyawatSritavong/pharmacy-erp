"use client";

import { useState, type FormEvent } from "react";

type Row = Record<string, unknown>;
type Submit = (path: string, payload: Row) => Promise<void>;
const field = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
const button = "min-h-11 rounded-xl bg-primary px-4 py-2 font-semibold text-primary-foreground disabled:opacity-50";
const panel = "grid gap-3 rounded-xl border bg-card p-4 sm:grid-cols-2";
const text = (v: unknown) => v == null ? "" : String(v);
const rows = (v: unknown): Row[] => Array.isArray(v) ? v as Row[] : [];
const money = (v: unknown) => new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" }).format(Number(v || 0) / 100);
function cents(v: FormDataEntryValue | null) {
  const raw = text(v).trim();
  if (!/^\d+(\.\d{1,2})?$/.test(raw)) throw new Error("ระบุจำนวนเงินไม่ติดลบ ทศนิยมไม่เกิน 2 ตำแหน่ง");
  const [whole, frac = ""] = raw.split(".");
  const n = Number(whole) * 100 + Number(frac.padEnd(2, "0"));
  if (!Number.isSafeInteger(n)) throw new Error("ยอดเงินเกินขอบเขต");
  return n;
}
function Input({ label, name, value, required = true }: { label: string; name: string; value?: string; required?: boolean }) {
  return <label className="grid gap-1 text-sm">{label}<input className={field} defaultValue={value} name={name} required={required} /></label>;
}
function DrawerPick({ drawers }: { drawers: Row[] }) {
  return <label className="grid gap-1 text-sm">รอบเงินสดของบัญชีนี้<select className={field} name="drawer" required><option value="">เลือกรอบที่เปิด</option>{drawers.filter((d) => !d.closed_at).map((d) => <option key={text(d.id)} value={text(d.id)}>{text(d.opened_at)} · คาด {money(d.current_expected_cents)}</option>)}</select></label>;
}
async function send(e: FormEvent<HTMLFormElement>, path: string, build: (f: FormData) => Row, submit: Submit, onError: (message: string) => void) {
  e.preventDefault();
  try { await submit(path, build(new FormData(e.currentTarget))); }
  catch (error) { onError(error instanceof Error ? error.message : "บันทึกไม่ได้"); }
}

export function V2CancellationForm({ document, busy, submit, onError }: { document: Row; busy: boolean; submit: Submit; onError: (message: string) => void }) {
  if (document.kind !== "ar_invoice" || document.status !== "issued") return null;
  return <details className="rounded-xl border p-4"><summary className="min-h-11 cursor-pointer font-semibold">ยกเลิกบิลและจัดการคืนลูกค้า</summary>
    <p className="mb-3 text-sm">บิลและหลักฐานชำระเดิมจะคงไว้ ระบบออกใบลดหนี้ส่วนที่เหลือและตั้งยอดรอคืน เงินสด เงินโอน หรือสินค้า ให้ทำต่อที่ “คืนบิลที่ยกเลิก”</p>
    <form className={panel} onSubmit={(e) => void send(e, "/documents/cancel", (f) => ({ document_id: document.id, reference: f.get("reference"), reason: f.get("reason"), return_stock: f.get("return_stock") === "on" }), submit, onError)}>
      <Input label="อ้างอิงการยกเลิก" name="reference" /><Input label="เหตุผลยกเลิก" name="reason" />
      <label className="flex min-h-11 items-center gap-2 text-sm"><input name="return_stock" type="checkbox" />ได้รับสินค้าทั้งหมดคืนแล้ว ให้รับส่วนที่ยังไม่คืนเข้ากักกัน</label>
      <label className="flex min-h-11 items-center gap-2 text-sm"><input required type="checkbox" />ยืนยันยกเลิกบิลและรับผิดชอบยอดรอคืน</label>
      <button className={button} disabled={busy}>ยกเลิกบิล</button>
    </form>
  </details>;
}

export function V2Refunds({ data, catalog, drawers, busy, submit, onError }: { data: Row; catalog: Row; drawers: Row[]; busy: boolean; submit: Submit; onError: (message: string) => void }) {
  return <div className="space-y-4"><p className="text-sm text-muted-foreground">ยอดรอคืนติดตามต่อเนื่องข้ามกะจนคืนครบ การคืนเป็นสินค้าใช้ราคาปัจจุบันรวม VAT และมูลค่าต้องเท่ากับยอดที่เลือกคืน</p>
    {!rows(data.items).length ? <p className="rounded-xl border p-4">ยังไม่มีบิลยกเลิก</p> : null}
    {rows(data.items).map((c) => <RefundCard key={`${text(c.id)}:${text(c.pending_cents)}`} cancellation={c} catalog={catalog} drawers={drawers} busy={busy} submit={submit} onError={onError} />)}
  </div>;
}

function RefundCard({ cancellation: c, catalog, drawers, busy, submit, onError }: { cancellation: Row; catalog: Row; drawers: Row[]; busy: boolean; submit: Submit; onError: (message: string) => void }) {
  const [method, setMethod] = useState("bank_transfer");
  const [goods, setGoods] = useState([{ product_id: "", unit_id: "", quantity: "1", vat: "7" }]);
  return <article className="space-y-3 rounded-xl border bg-card p-4"><h2 className="break-all font-semibold">{text(c.document_number)}</h2><p>ยอดต้องคืน {money(c.refund_due_cents)} · ยังรอคืน {money(c.pending_cents)}</p>
    {c.cross_day && Number(c.pending_cents) > 0 ? <p className="text-destructive" role="alert">รายการคืนข้ามวัน ต้องรับทราบก่อนปิดกะ</p> : null}
    {Number(c.pending_cents) > 0 ? <form className={panel} onSubmit={(e) => void send(e, "/refunds", (f) => ({ cancellation_id: c.id, method, amount_cents: cents(f.get("amount")), reference: f.get("reference"), reason: f.get("reason"), drawer_id: text(f.get("drawer")), goods: method === "goods" ? goods.map((g) => ({ product_id: g.product_id, unit_id: g.unit_id, quantity: Number(g.quantity), vat_bps: Math.round(Number(g.vat) * 100) })) : [] }), submit, onError)}>
      <label className="grid gap-1 text-sm">วิธีคืน<select className={field} onChange={(e) => setMethod(e.target.value)} value={method}><option value="bank_transfer">เงินโอน</option><option value="cash">เงินสด</option><option value="goods">สินค้ามูลค่าเท่ากัน</option></select></label>
      <Input label="ยอดคืนครั้งนี้ (บาท)" name="amount" value={(Number(c.pending_cents) / 100).toFixed(2)} /><Input label="หลักฐาน/อ้างอิง" name="reference" /><Input label="เหตุผล" name="reason" />
      {method === "cash" ? <DrawerPick drawers={drawers} /> : null}
      {method === "goods" ? <div className="col-span-full space-y-3">{goods.map((g, i) => <fieldset className="grid gap-2 rounded-xl border p-3 sm:grid-cols-2" key={i}><legend>สินค้าแทนเงินคืน {i + 1}</legend>
        <label className="grid gap-1 text-sm">สินค้า<select className={field} required value={g.product_id} onChange={(e) => setGoods(goods.map((line, n) => n === i ? { ...line, product_id: e.target.value, unit_id: "" } : line))}><option value="">เลือกสินค้า</option>{rows(catalog.products).map((p) => <option key={text(p.id)} value={text(p.id)}>{text(p.name)} · {money(p.price_cents)}</option>)}</select></label>
        <label className="grid gap-1 text-sm">หน่วย<select className={field} value={g.unit_id} onChange={(e) => setGoods(goods.map((line, n) => n === i ? { ...line, unit_id: e.target.value } : line))}><option value="">หน่วยฐาน</option>{rows(catalog.units).filter((u) => u.product_id === g.product_id).map((u) => <option key={text(u.id)} value={text(u.id)}>{text(u.unit_name)} × {text(u.conversion_qty)}</option>)}</select></label>
        <label className="grid gap-1 text-sm">จำนวน<input className={field} type="number" required min="1" step="1" value={g.quantity} onChange={(e) => setGoods(goods.map((line, n) => n === i ? { ...line, quantity: e.target.value } : line))} /></label>
        <label className="grid gap-1 text-sm">VAT (%)<input className={field} type="number" required min="0" max="100" step="0.01" value={g.vat} onChange={(e) => setGoods(goods.map((line, n) => n === i ? { ...line, vat: e.target.value } : line))} /></label>
        <button type="button" className="min-h-11 text-left text-destructive" disabled={goods.length === 1 || busy} onClick={() => setGoods(goods.filter((_, n) => n !== i))}>ลบรายการ</button>
      </fieldset>)}<button className="min-h-11 rounded-xl border px-4" type="button" disabled={busy} onClick={() => setGoods([...goods, { product_id: "", unit_id: "", quantity: "1", vat: "7" }])}>+ เพิ่มสินค้า</button></div> : null}
      <label className="flex min-h-11 items-center gap-2 text-sm"><input required type="checkbox" />ยืนยันว่าลูกค้าได้รับเงินหรือสินค้าแล้ว</label><button className={button} disabled={busy}>บันทึกการคืน</button>
    </form> : <p className="text-sm">คืนครบแล้ว</p>}
    {rows(c.settlements).map((r) => <p className="rounded-lg bg-muted p-3 text-sm" key={text(r.id)}>{text(r.created_at)} · {text(r.method)} · {money(r.amount_cents)} · {text(r.reference)}</p>)}
  </article>;
}

export function V2Drawers({ data, busy, submit, onError }: { data: Row; busy: boolean; submit: Submit; onError: (message: string) => void }) {
  const warnings = rows(data.warnings);
  return <div className="space-y-4"><p className="text-sm text-muted-foreground">หนึ่งรอบต่อบัญชี ยอดค้างจากกะก่อนส่งต่อเป็นรายการแจ้งเตือน เงินตั้งต้นใช้ยอดที่นับจริง ไม่รวมยอดค้างซ้ำ</p>
    {warnings.length ? <section className="space-y-2 rounded-xl border border-destructive p-4" aria-label="ยอดค้างที่ส่งต่อ" role="alert"><h2 className="font-semibold">รายการค้างที่ต้องรับทราบก่อนปิดกะ</h2>{warnings.map((w) => <div className="space-y-2 border-b pb-3" key={text(w.id)}><p>{w.kind === "refund" ? "คืนลูกค้า" : "ส่วนต่างกะก่อน"} · {money(w.amount_cents)}{w.cross_day ? " · ข้ามวัน" : ""}</p><p className="break-all text-sm">{text(w.reference)} · {text(w.reason)}</p>{w.kind === "variance" ? <form className="grid gap-2 sm:grid-cols-2" onSubmit={(e) => void send(e, "/drawers/issues/resolve", (f) => ({ issue_id: w.id, reason: f.get("reason") }), submit, onError)}><Input label="ผลตรวจสอบและเหตุผลอนุมัติ (Superadmin)" name="reason" /><button className={button} disabled={busy}>บันทึกผลตรวจสอบยอดค้าง</button></form> : null}</div>)}</section> : null}
    <form className={panel} onSubmit={(e) => void send(e, "/drawers", (f) => ({ opening_cents: cents(f.get("opening")) }), submit, onError)}><Input label="เงินตั้งต้นนับจริง (บาท)" name="opening" value="0" /><button className={button} disabled={busy || rows(data.items).some((d) => !d.closed_at)}>เปิดรอบเงินสด</button></form>
    {rows(data.items).map((d) => <article className="space-y-3 rounded-xl border bg-card p-4" key={text(d.id)}><h2 className="font-semibold">รอบ {text(d.opened_at)}</h2><p>ตั้งต้น {money(d.opening_cents)} · คาด {money(d.current_expected_cents)}{d.closed_at ? ` · ปิดแล้ว นับ ${money(d.counted_cents)} · ส่วนต่าง ${money(Number(d.counted_cents) - Number(d.expected_cents))}` : " · เปิดอยู่"}</p>
      {!d.closed_at ? <><form className={panel} onSubmit={(e) => void send(e, "/drawers/cash", (f) => ({ id: d.id, amount_cents: cents(f.get("amount")) * (f.get("direction") === "out" ? -1 : 1), reason: f.get("reason") }), submit, onError)}><label className="grid gap-1 text-sm">นำเงิน<select className={field} name="direction"><option value="in">เข้า</option><option value="out">ออก</option></select></label><Input label="จำนวน (บาท)" name="amount" /><Input label="เหตุผล" name="reason" /><button className={button} disabled={busy}>บันทึกนำเงิน</button></form>
        <form className={panel} onSubmit={(e) => void send(e, "/drawers/close", (f) => ({ id: d.id, counted_cents: cents(f.get("counted")), reason: f.get("reason"), warnings_token: f.get("ack") === "on" ? data.warnings_token : "" }), submit, onError)}><Input label="ยอดนับจริง (บาท)" name="counted" /><Input label="เหตุผลส่วนต่าง" name="reason" required={false} />{warnings.length ? <label className="flex min-h-11 items-center gap-2 text-sm"><input key={text(data.warnings_token)} name="ack" type="checkbox" required />รับทราบรายการค้างทั้งหมด และส่งต่อกะใหม่</label> : null}<button className={button} disabled={busy}>ปิดรอบเงินสด</button></form>
      </> : null}
    </article>)}
  </div>;
}

export { DrawerPick };
