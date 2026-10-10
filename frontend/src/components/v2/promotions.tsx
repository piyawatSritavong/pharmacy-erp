"use client";
import { useState, type FormEvent } from "react";
type Row = Record<string, unknown>;
type Member = { product_id: string; unit_id: string; quantity: string };
const blank = (): Member => ({ product_id: "", unit_id: "", quantity: "1" });
const field = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
const button = "min-h-11 rounded-xl bg-primary px-4 py-2 text-primary-foreground disabled:opacity-50";
const str = (v: unknown) => v == null ? "" : String(v);
const rows = (v: unknown): Row[] => Array.isArray(v) ? v as Row[] : [];
function cents(v: FormDataEntryValue | null) { const raw = str(v); if (!/^\d+(\.\d{1,2})?$/.test(raw)) throw new Error("เงินต้องไม่ติดลบและทศนิยมไม่เกิน 2 ตำแหน่ง"); const [whole, decimal = ""] = raw.split("."); const value = Number(whole) * 100 + Number(decimal.padEnd(2, "0")); if (!Number.isSafeInteger(value)) throw new Error("ยอดเกินขอบเขต"); return value; }
function Members({ title, value, update, catalog }: { title: string; value: Member[]; update: (next: Member[]) => void; catalog: Row }) {
  const change = (i: number, key: keyof Member, next: string) => update(value.map((m, index) => index === i ? { ...m, [key]: next, ...(key === "product_id" ? { unit_id: "" } : {}) } : m));
  return <fieldset className="col-span-full space-y-3 rounded-xl border p-3"><legend>{title}</legend>{value.map((m, i) => <div className="grid gap-2 sm:grid-cols-3" key={i}><label className="grid gap-1 text-sm">สินค้า<select className={field} required value={m.product_id} onChange={(e) => change(i, "product_id", e.target.value)}><option value="">เลือกสินค้า</option>{rows(catalog.products).map((p) => <option key={str(p.id)} value={str(p.id)}>{str(p.name)}</option>)}</select></label><label className="grid gap-1 text-sm">หน่วย<select className={field} value={m.unit_id} onChange={(e) => change(i, "unit_id", e.target.value)}><option value="">หน่วยฐาน</option>{rows(catalog.units).filter((u) => u.product_id === m.product_id).map((u) => <option key={str(u.id)} value={str(u.id)}>{str(u.unit_name)}</option>)}</select></label><label className="grid gap-1 text-sm">จำนวน<input className={field} min="1" required step="1" type="number" value={m.quantity} onChange={(e) => change(i, "quantity", e.target.value)} /></label><button className="min-h-11 text-left text-sm" type="button" onClick={() => update(value.filter((_, index) => i !== index))}>ลบสินค้า</button></div>)}<button className="min-h-11 rounded-xl border px-4" type="button" onClick={() => update([...value, blank()])}>+ เพิ่มสินค้า</button></fieldset>;
}

type EditorProps = { catalog: Row; busy: boolean; submit: (path: string, payload: Row) => Promise<void>; onError: (message: string) => void; initial?: Row };

function PromotionEditor({ catalog, busy, submit, onError, initial }: EditorProps) {
  const rule = (initial?.rule || {}) as Row;
  const members = (value: unknown): Member[] => rows(value).map((m) => ({ product_id: str(m.product_id), unit_id: str(m.unit_id), quantity: str(m.quantity) }));
  const [kind, setKind] = useState(str(initial?.promo_type) || "percent");
  const [scope, setScope] = useState(initial ? initial.branch_id ? "branch" : "central" : "branch");
  const [conditions, setConditions] = useState<Member[]>(members(rule.conditions));
  const [rewards, setRewards] = useState<Member[]>(members(rule.rewards));
  const allowed = scope === "central" || Boolean((catalog.policy as Row | null)?.allow_branch_promotions);
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const f = new FormData(event.currentTarget);
    try { await submit("/promotions", { ...(initial ? { id: initial.id, revision: initial.revision } : {}), scope, code: f.get("code"), name: f.get("name"), type: kind, starts_on: f.get("start"), ends_on: str(f.get("end")), active: f.get("active") === "on", reason: f.get("reason"), rule: { min_base_quantity: Number(f.get("minqty")), min_amount_cents: cents(f.get("minamount")), discount_bps: Math.round(Number(f.get("percent")) * 100), discount_cents: cents(f.get("amount")), bundle_price_cents: cents(f.get("bundle")), max_uses: Number(f.get("max")), conditions: conditions.map((m) => ({ ...m, quantity: Number(m.quantity) })), rewards: ["buy_x_get_y", "bill_giveaway"].includes(kind) ? rewards.map((m) => ({ ...m, quantity: Number(m.quantity) })) : [] } }); }
    catch (e) { onError(e instanceof Error ? e.message : "โปรโมชั่นไม่ถูกต้อง"); }
  }
  const baht = (v: unknown) => (Number(v || 0) / 100).toFixed(2);
  const fields = [["code", "รหัสโปร", "text", str(initial?.code)], ["name", "ชื่อโปร", "text", str(initial?.name)], ["start", "เริ่ม", "date", str(initial?.starts_on)], ["end", "สิ้นสุด (เว้นว่างได้)", "date", str(initial?.ends_on)], ["minqty", "ขั้นต่ำจำนวนฐานรวม", "number", str(rule.min_base_quantity || 0)], ["minamount", "ขั้นต่ำยอดก่อน VAT (บาท)", "text", baht(rule.min_amount_cents)], ["percent", "ลด (%)", "number", str(Number(rule.discount_bps || 0) / 100)], ["amount", "ลด (บาท)", "text", baht(rule.discount_cents)], ["bundle", "ราคาต่อชุด (บาท)", "text", baht(rule.bundle_price_cents)], ["max", "ชุดสูงสุด (0 = ไม่จำกัด)", "number", str(rule.max_uses || 0)], ["reason", "เหตุผลกำหนด/แก้ไขโปร", "text", ""]];
  return <form className="grid gap-3 rounded-2xl border bg-card p-4 sm:grid-cols-2 lg:grid-cols-3" onSubmit={(e) => void save(e)}>
    <label className="grid gap-1 text-sm">ขอบเขต<select className={field} disabled={Boolean(initial) || busy} value={scope} onChange={(e) => setScope(e.target.value)}><option value="branch">สาขานี้</option><option value="central">ส่วนกลางทุกสาขา</option></select></label><label className="grid gap-1 text-sm">ประเภท<select className={field} value={kind} onChange={(e) => setKind(e.target.value)}><option value="percent">ลดเปอร์เซ็นต์</option><option value="amount">ลดจำนวนเงิน</option><option value="buy_x_get_y">ซื้อครบจำนวนแถมสินค้า</option><option value="bundle">ราคาชุด</option><option value="bill_giveaway">ครบยอดบิลแถมสินค้า</option></select></label>
    {fields.map(([name, title, type, value]) => <label className="grid gap-1 text-sm" key={name}>{title}<input className={field} name={name} type={type} defaultValue={value} required={name !== "end"} step={name === "percent" ? "0.01" : undefined} min={type === "number" ? "0" : undefined} max={name === "percent" ? "100" : undefined} /></label>)}
    <Members title="สินค้าที่เข้าเงื่อนไข (ลด %/เงิน เว้นว่าง = ทุกสินค้า)" value={conditions} update={setConditions} catalog={catalog} />{kind === "buy_x_get_y" || kind === "bill_giveaway" ? <Members title="สินค้าแถม" value={rewards} update={setRewards} catalog={catalog} /> : null}
    <label className="flex min-h-11 items-center gap-2 text-sm"><input name="active" type="checkbox" defaultChecked={initial ? Boolean(initial.active) : true} />เปิดใช้งาน</label>
    <p className="col-span-full text-sm text-muted-foreground">เลือกหนึ่งโปรต่อเอกสาร ของแถมตัดสต๊อกและต้นทุนตามปกติ การแก้ไขสร้างประวัติรุ่นใหม่และรักษาโปรในบิลเก่า</p>{!allowed ? <p className="col-span-full text-sm text-destructive">สาขานี้ยังไม่ได้เปิดสิทธิ์จัดการโปร</p> : null}<button className={button} disabled={busy || !allowed}>{initial ? "บันทึกแก้ไขโปรโมชั่น" : "สร้างโปรโมชั่น"}</button>
  </form>;
}

export function V2Promotions({ data, ...props }: EditorProps & { data: Row }) {
  return <div className="space-y-4"><PromotionEditor {...props} />{rows(data.items).map((p) => <details className="rounded-xl border bg-card p-4" key={`${str(p.id)}:${str(p.revision)}`}><summary className="min-h-11 cursor-pointer font-semibold">{str(p.name)} · {str(p.code)} · {p.active ? "เปิดใช้" : "ปิดใช้"} · รุ่น {str(p.revision)}</summary><p className="mb-3 text-sm">{p.branch_id ? "โปรสาขา" : "โปรส่วนกลาง"} · {str(p.promo_type)} · {str(p.starts_on)} – {str(p.ends_on) || "ไม่กำหนด"}</p><PromotionEditor {...props} initial={p} /></details>)}</div>;
}
