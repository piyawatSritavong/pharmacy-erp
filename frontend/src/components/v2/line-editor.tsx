"use client";
import { cloneElement, isValidElement, useId, type ReactNode } from "react";
type Row = Record<string, unknown>;
export type Entry = { product_id: string; unit_id: string; quantity: string; cost: string; discount: string; vat: string; lot_number: string; expires_on: string; lot_id: string };
export const blankLine = (): Entry => ({ product_id: "", unit_id: "", quantity: "1", cost: "0", discount: "0", vat: "7", lot_number: "", expires_on: "", lot_id: "" });
const inputClass = "min-h-11 w-full rounded-xl border bg-background px-3 py-2 text-sm";
function str(value: unknown) { return value == null ? "" : String(value); }
function rows(value: unknown): Row[] { return Array.isArray(value) ? value as Row[] : []; }
function Field({ label, children }: { label: string; children: ReactNode }) {
  const id = useId();
  return <label className="grid min-w-0 gap-1 text-sm font-medium"><span id={id}>{label}</span>{isValidElement<{ "aria-labelledby"?: string }>(children) ? cloneElement(children, { "aria-labelledby": id }) : children}</label>;
}
export function V2Lines({ entries, setEntries, catalog, stock, receipt = false, purchase = false }: { entries: Entry[]; setEntries: (lines: Entry[]) => void; catalog: Row; stock: boolean; receipt?: boolean; purchase?: boolean }) {
  const update = (index: number, key: keyof Entry, value: string) => setEntries(entries.map((line, i) => i === index ? { ...line, [key]: value, ...(key === "product_id" ? { unit_id: "" } : {}) } : line));
  return <div className="col-span-full space-y-3">{entries.map((line, index) => <fieldset className="grid gap-3 rounded-xl border p-3 sm:grid-cols-2 lg:grid-cols-4" key={index}>
    <legend className="px-1 text-sm">รายการ {index + 1}</legend>
    <Field label="สินค้า"><select className={inputClass} onChange={(e) => update(index, "product_id", e.target.value)} required value={line.product_id}><option value="">เลือกสินค้า</option>{rows(catalog.products).map((item) => <option key={str(item.id)} value={str(item.id)}>{str(item.name)} · {str(item.sku)}</option>)}</select></Field>
    <Field label="หน่วย"><select className={inputClass} onChange={(e) => update(index, "unit_id", e.target.value)} value={line.unit_id}><option value="">หน่วยฐาน</option>{rows(catalog.units).filter((item) => str(item.product_id) === line.product_id).map((item) => <option key={str(item.id)} value={str(item.id)}>{str(item.unit_name)} × {str(item.conversion_qty)}</option>)}</select></Field>
    <Field label="จำนวน"><input className={inputClass} min="1" onChange={(e) => update(index, "quantity", e.target.value)} required step="1" type="number" value={line.quantity} /></Field>
    {receipt || purchase ? <Field label="ต้นทุนต่อหน่วย (บาท)"><input className={inputClass} inputMode="decimal" onChange={(e) => update(index, "cost", e.target.value)} required value={line.cost} /></Field> : null}
    {!stock || receipt ? <Field label="ส่วนลดรายการ (บาท)"><input className={inputClass} inputMode="decimal" onChange={(e) => update(index, "discount", e.target.value)} required value={line.discount} /></Field> : null}
    {!stock ? <Field label="VAT (%)"><input className={inputClass} min="0" max="100" onChange={(e) => update(index, "vat", e.target.value)} required step="0.01" type="number" value={line.vat} /></Field> : null}
    {receipt ? <><Field label="เลขล็อต"><input className={inputClass} onChange={(e) => update(index, "lot_number", e.target.value)} required value={line.lot_number} /></Field><Field label="วันหมดอายุ"><input className={inputClass} onChange={(e) => update(index, "expires_on", e.target.value)} type="date" value={line.expires_on} /></Field></> : null}
    <button className="text-left text-sm text-destructive disabled:opacity-40" disabled={entries.length === 1} onClick={() => setEntries(entries.filter((_, i) => i !== index))} type="button">ลบรายการ</button>
  </fieldset>)}<button className="min-h-11 rounded-xl border px-4" onClick={() => setEntries([...entries, blankLine()])} type="button">+ เพิ่มรายการ</button></div>;
}
