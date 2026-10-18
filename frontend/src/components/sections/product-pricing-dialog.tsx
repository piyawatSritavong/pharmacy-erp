"use client";

import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";

import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  FeedbackNotice,
  Input,
  LoadingState,
  Select
} from "@/components/ui/primitives";
import type { Feedback } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

type UnitRow = { id: string; unit_name: string; conversion_qty: string; selling_price: string; barcode: string; is_base: boolean };
type TierRow = { unit_id: string; branch_id: string; customer_tier: string; min_quantity: string; unit_price: string };

/**
 * แตกราคาขายปลีก–ส่ง for one product: the units it sells in (ชิ้น/แผง/กล่อง)
 * and the tier prices — a quantity break for everyone, or a price for
 * wholesale customers only. The till picks the lowest price a line qualifies for.
 */
export function ProductPricingDialog({
  product,
  canEditUnits,
  canEditTiers,
  onClose
}: {
  product: Option | null;
  canEditUnits: boolean;
  canEditTiers: boolean;
  onClose: () => void;
}) {
  const [units, setUnits] = useState<UnitRow[]>([]);
  const [tiers, setTiers] = useState<TierRow[]>([]);
  // Price levels 2-5 per selling unit, keyed by unit id ("" = the base unit).
  const [levelPrices, setLevelPrices] = useState<Record<string, Record<number, string>>>({});
  const [priceLevels, setPriceLevels] = useState<Array<{ level: number; name: string; default_discount_percent: number }>>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);
  const productId = product ? String(product.id) : "";
  const basePrice = Number(product?.base_selling_price || 0);

  useEffect(() => {
    if (!productId) return;
    setFeedback(null);
    setLoading(true);
    void Promise.all([
      proxyClient<{ items: Option[] }>(`/products/${productId}/units`),
      proxyClient<{ items: Option[] }>(`/products/${productId}/price-tiers`),
      proxyClient<{ items: Option[] }>(`/products/${productId}/level-prices`).catch(() => ({ items: [] as Option[] })),
      proxyClient<{ price_levels: Array<{ level: number; name: string; default_discount_percent: number }> }>("/member-levels").catch(() => ({ price_levels: [] }))
    ])
      .then(([unitResponse, tierResponse, levelResponse, levelSettings]) => {
        const loadedLevels: Record<string, Record<number, string>> = {};
        for (const item of levelResponse.items || []) {
          const key = String(item.unit_id || "");
          loadedLevels[key] = { ...(loadedLevels[key] || {}), [Number(item.price_level)]: String(item.unit_price) };
        }
        setLevelPrices(loadedLevels);
        setPriceLevels(levelSettings.price_levels || []);
        const loadedUnits = (unitResponse.items || []).filter((unit) => unit.active !== false).map((unit) => ({
          id: String(unit.id),
          unit_name: String(unit.unit_name),
          conversion_qty: String(unit.conversion_qty),
          selling_price: unit.selling_price == null ? "" : String(unit.selling_price),
          barcode: String(unit.barcode || ""),
          is_base: Boolean(unit.is_base)
        }));
        setUnits(loadedUnits.length ? loadedUnits : [{
          id: "", unit_name: String(product?.unit_name || "ชิ้น"), conversion_qty: "1", selling_price: "", barcode: "", is_base: true
        }]);
        setTiers((tierResponse.items || []).map((tier) => ({
          unit_id: String(tier.unit_id || ""),
          branch_id: String(tier.branch_id || ""),
          customer_tier: String(tier.customer_tier || "all"),
          min_quantity: String(tier.min_quantity),
          unit_price: String(tier.unit_price)
        })));
      })
      .catch((error) => setFeedback({ tone: "error", text: error instanceof Error ? error.message : "โหลดหน่วยขายไม่สำเร็จ" }))
      .finally(() => setLoading(false));
  }, [product, productId]);

  function unitPrice(row: UnitRow) {
    if (row.selling_price !== "") return Number(row.selling_price);
    return basePrice * Number(row.conversion_qty || 1);
  }

  async function saveUnits() {
    setBusy(true);
    setFeedback(null);
    try {
      await proxyClient(`/products/${productId}/units`, {
        method: "PUT",
        body: JSON.stringify({
          units: units.map((row, index) => ({
            id: row.id || undefined,
            unit_name: row.unit_name.trim(),
            conversion_qty: row.is_base ? 1 : Number(row.conversion_qty),
            is_base: row.is_base,
            selling_price: row.selling_price === "" ? null : Number(row.selling_price),
            barcode: row.barcode.trim(),
            sort_order: index
          }))
        })
      });
      const refreshed = await proxyClient<{ items: Option[] }>(`/products/${productId}/units`);
      setUnits((refreshed.items || []).filter((unit) => unit.active !== false).map((unit) => ({
        id: String(unit.id),
        unit_name: String(unit.unit_name),
        conversion_qty: String(unit.conversion_qty),
        selling_price: unit.selling_price == null ? "" : String(unit.selling_price),
        barcode: String(unit.barcode || ""),
        is_base: Boolean(unit.is_base)
      })));
      setFeedback({ tone: "success", text: "บันทึกหน่วยขายแล้ว" });
    } catch (error) {
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : "บันทึกหน่วยขายไม่สำเร็จ" });
    } finally {
      setBusy(false);
    }
  }

  async function saveTiers() {
    setBusy(true);
    setFeedback(null);
    try {
      await proxyClient(`/products/${productId}/price-tiers`, {
        method: "PUT",
        body: JSON.stringify({
          tiers: tiers.map((tier) => ({
            unit_id: tier.unit_id,
            branch_id: tier.branch_id,
            customer_tier: tier.customer_tier,
            min_quantity: Number(tier.min_quantity),
            unit_price: Number(tier.unit_price)
          }))
        })
      });
      setFeedback({ tone: "success", text: "บันทึกราคาส่งแล้ว มีผลกับการขายหน้าร้านทันที" });
    } catch (error) {
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : "บันทึกราคาส่งไม่สำเร็จ" });
    } finally {
      setBusy(false);
    }
  }

  async function saveLevelPrices() {
    setBusy(true);
    setFeedback(null);
    try {
      const prices: Array<{ unit_id: string; price_level: number; unit_price: number }> = [];
      for (const [unitKey, byLevel] of Object.entries(levelPrices)) {
        for (const [level, value] of Object.entries(byLevel)) {
          if (value.trim() === "") continue;
          prices.push({ unit_id: unitKey, price_level: Number(level), unit_price: Number(value) });
        }
      }
      await proxyClient(`/products/${productId}/level-prices`, { method: "PUT", body: JSON.stringify({ prices }) });
      setFeedback({ tone: "success", text: "บันทึกราคาตามระดับแล้ว มีผลกับการขายหน้าร้านทันที" });
    } catch (error) {
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : "บันทึกราคาตามระดับไม่สำเร็จ" });
    } finally {
      setBusy(false);
    }
  }

  /** What a level pays for one unit when nothing is typed: the lowest of the
   *  levels below it (typed or default discount), or the shelf price. */
  function fallbackPrice(row: UnitRow, level: number) {
    const shelf = unitPrice(row);
    const key = row.is_base ? "" : row.id;
    let best = shelf;
    for (let lower = 2; lower <= level; lower += 1) {
      const typed = levelPrices[key]?.[lower] ?? (row.is_base ? undefined : levelPrices[""]?.[lower] ? String(Number(levelPrices[""][lower]) * Number(row.conversion_qty || 1)) : undefined);
      const candidates = [] as number[];
      if (typed && lower < level) candidates.push(Number(typed));
      const percent = priceLevels.find((item) => item.level === lower)?.default_discount_percent || 0;
      if (percent > 0) candidates.push(shelf * (1 - percent / 100));
      for (const value of candidates) if (value > 0 && value < best) best = value;
    }
    return Math.round(best * 100) / 100;
  }

  const levelName = (level: number) => priceLevels.find((item) => item.level === level)?.name || `ราคา L${level}`;
  const costPrice = Number(product?.cost_price || 0);

  const savedUnits = units.filter((unit) => unit.id);
  const baseUnit = units.find((unit) => unit.is_base);

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open={Boolean(product)}>
      <DialogContent className="max-w-4xl">
        <DialogHeader
          description={`ราคาขายตั้งต้น ${currency(basePrice)} ต่อ${baseUnit?.unit_name || "หน่วย"} (ก่อนภาษี) · หน่วยที่ไม่ใส่ราคาจะคิดจากราคาตั้งต้น × จำนวนต่อหน่วย`}
          title={`หน่วยขาย ราคาตามระดับ และราคาส่ง · ${String(product?.name || "")}`}
        />
        {loading ? <LoadingState label="กำลังโหลด" /> : (
          <div className="space-y-6">
            <section className="space-y-3">
              <div className="flex items-center justify-between gap-2">
                <h3 className="font-semibold">หน่วยขาย (แตกขายปลีก–ส่ง)</h3>
                {canEditUnits ? (
                  <Button onClick={() => setUnits([...units, { id: "", unit_name: "", conversion_qty: "10", selling_price: "", barcode: "", is_base: false }])} size="sm" type="button" variant="secondary">
                    <Plus className="h-4 w-4" />เพิ่มหน่วย
                  </Button>
                ) : null}
              </div>
              <div className="overflow-x-auto rounded-xl border">
                <table className="mobile-card-table w-full text-sm sm:min-w-[640px]">
                  <thead className="bg-muted text-left">
                    <tr>
                      <th className="p-2">ชื่อหน่วย</th>
                      <th className="p-2">จำนวนต่อหน่วย</th>
                      <th className="p-2">ราคาต่อหน่วย</th>
                      <th className="p-2">บาร์โค้ด</th>
                      <th className="w-10 p-2" />
                    </tr>
                  </thead>
                  <tbody>
                    {units.map((row, index) => (
                      <tr className="border-t" key={row.id || `new-${index}`}>
                        <td data-label="ชื่อหน่วย" className="p-2">
                          <Input aria-label="ชื่อหน่วย" disabled={!canEditUnits} onChange={(event) => setUnits(units.map((item, i) => i === index ? { ...item, unit_name: event.target.value } : item))} value={row.unit_name} />
                          {row.is_base ? <span className="text-2xs text-muted-foreground">หน่วยฐาน (นับสต๊อก)</span> : null}
                        </td>
                        <td data-label="จำนวนต่อหน่วย" className="p-2">
                          <Input aria-label="จำนวนต่อหน่วย" disabled={!canEditUnits || row.is_base} min={1} onChange={(event) => setUnits(units.map((item, i) => i === index ? { ...item, conversion_qty: event.target.value } : item))} type="number" value={row.is_base ? "1" : row.conversion_qty} />
                        </td>
                        <td data-label="ราคาต่อหน่วย" className="p-2">
                          <Input aria-label="ราคาต่อหน่วย" disabled={!canEditUnits} min={0} onChange={(event) => setUnits(units.map((item, i) => i === index ? { ...item, selling_price: event.target.value } : item))} placeholder={currency(unitPrice(row))} step="0.01" type="number" value={row.selling_price} />
                        </td>
                        <td data-label="บาร์โค้ด" className="p-2">
                          <Input aria-label="บาร์โค้ดของหน่วย" disabled={!canEditUnits} onChange={(event) => setUnits(units.map((item, i) => i === index ? { ...item, barcode: event.target.value } : item))} value={row.barcode} />
                        </td>
                        <td data-actions="true" className="p-2">
                          {canEditUnits && !row.is_base ? (
                            <button aria-label="ลบหน่วย" className="grid h-9 w-9 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-destructive" onClick={() => setUnits(units.filter((_, i) => i !== index))} type="button">
                              <Trash2 className="h-4 w-4" />
                            </button>
                          ) : null}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {canEditUnits ? (
                <div className="flex justify-end">
                  <Button loading={busy} onClick={() => void saveUnits()} type="button">บันทึกหน่วยขาย</Button>
                </div>
              ) : null}
            </section>

            <section className="space-y-3">
              <div>
                <h3 className="font-semibold">ราคาตามระดับสมาชิก</h3>
                <p className="text-xs text-muted-foreground">
                  L1 คือราคาขายปกติ (ลูกค้าทั่วไป) · L2–L5 ใช้กับสมาชิกตามระดับที่ตั้งไว้ในหน้า ลูกค้าและสมาชิก · ช่องว่าง = ใช้ราคาที่แสดงจาง (ระดับก่อนหน้าหรือส่วนลดตั้งต้น)
                  {costPrice > 0 ? ` · ต้นทุน ${currency(costPrice)} ต่อ${baseUnit?.unit_name || "หน่วย"}` : ""}
                </p>
              </div>
              {savedUnits.length || units.some((unit) => unit.is_base) ? (
                <div className="overflow-x-auto rounded-xl border">
                  <table className="mobile-card-table w-full text-sm sm:min-w-[640px]">
                    <thead className="bg-muted text-left">
                      <tr>
                        <th className="p-2">หน่วย</th>
                        <th className="p-2">L1 {levelName(1)}</th>
                        {[2, 3, 4, 5].map((level) => <th className="p-2" key={level}>L{level} {levelName(level)}</th>)}
                      </tr>
                    </thead>
                    <tbody>
                      {units.filter((row) => row.is_base || row.id).map((row) => {
                        const key = row.is_base ? "" : row.id;
                        const unitCost = costPrice * Number(row.is_base ? 1 : row.conversion_qty || 1);
                        return (
                          <tr className="border-t" key={key || "base"}>
                            <td className="p-2 font-medium" data-label="หน่วย" data-primary="true">{row.unit_name || "หน่วยฐาน"}</td>
                            <td className="p-2 tabular-nums" data-label={`L1 ${levelName(1)}`}>{currency(unitPrice(row))}</td>
                            {[2, 3, 4, 5].map((level) => {
                              const value = levelPrices[key]?.[level] ?? "";
                              const below = value !== "" && unitCost > 0 && Number(value) < unitCost;
                              return (
                                <td className="p-2" data-label={`L${level} ${levelName(level)}`} key={level}>
                                  <Input
                                    aria-label={`ราคา L${level} ของ${row.unit_name || "หน่วยฐาน"}`}
                                    className={below ? "border-error text-error-800" : undefined}
                                    disabled={!canEditTiers}
                                    min={0.01}
                                    onChange={(event) => setLevelPrices((current) => ({ ...current, [key]: { ...(current[key] || {}), [level]: event.target.value } }))}
                                    placeholder={currency(fallbackPrice(row, level))}
                                    step="0.01"
                                    type="number"
                                    value={value}
                                  />
                                  {below ? <span className="mt-1 block text-xs text-error-800">ต่ำกว่าต้นทุน {currency(unitCost)}</span> : null}
                                </td>
                              );
                            })}
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              ) : null}
              {canEditTiers ? (
                <div className="flex justify-end">
                  <Button loading={busy} onClick={() => void saveLevelPrices()} type="button">บันทึกราคาตามระดับ</Button>
                </div>
              ) : null}
            </section>
            <section className="space-y-3">
              <div className="flex items-center justify-between gap-2">
                <div>
                  <h3 className="font-semibold">ราคาส่ง / ราคาตามจำนวน</h3>
                  <p className="text-xs text-muted-foreground">ระบบเลือกราคาที่ต่ำที่สุดที่บรรทัดนั้นเข้าเงื่อนไข ลูกค้าขายส่งต้องเลือกสมาชิกที่หน้าร้านก่อน</p>
                </div>
                {canEditTiers ? (
                  <Button
                    className="shrink-0 whitespace-nowrap"
                    disabled={!savedUnits.length}
                    onClick={() => setTiers([...tiers, { unit_id: savedUnits[0]?.id || "", branch_id: "", customer_tier: "all", min_quantity: "10", unit_price: "" }])}
                    size="sm"
                    type="button"
                    variant="secondary"
                  >
                    <Plus className="h-4 w-4" />เพิ่มราคา
                  </Button>
                ) : null}
              </div>
              {!savedUnits.length ? <p className="text-sm text-muted-foreground">บันทึกหน่วยขายก่อน แล้วจึงกำหนดราคาส่งต่อหน่วยได้</p> : null}
              {tiers.length ? (
                <div className="overflow-x-auto rounded-xl border">
                  <table className="mobile-card-table w-full text-sm sm:min-w-[640px]">
                    <thead className="bg-muted text-left">
                      <tr>
                        <th className="p-2">หน่วย</th>
                        <th className="p-2">ใช้กับ</th>
                        <th className="p-2">ซื้อตั้งแต่ (หน่วย)</th>
                        <th className="p-2">ราคาต่อหน่วย</th>
                        <th className="w-10 p-2" />
                      </tr>
                    </thead>
                    <tbody>
                      {tiers.map((tier, index) => (
                        <tr className="border-t" key={`tier-${index}`}>
                          <td data-label="หน่วย" className="p-2">
                            <Select aria-label="หน่วยของราคาส่ง" disabled={!canEditTiers} onChange={(event) => setTiers(tiers.map((item, i) => i === index ? { ...item, unit_id: event.target.value } : item))} value={tier.unit_id}>
                              {savedUnits.map((unit) => <option key={unit.id} value={unit.id}>{unit.unit_name}</option>)}
                            </Select>
                          </td>
                          <td data-label="ใช้กับ" className="p-2">
                            <Select aria-label="กลุ่มลูกค้า" disabled={!canEditTiers} onChange={(event) => setTiers(tiers.map((item, i) => i === index ? { ...item, customer_tier: event.target.value } : item))} value={tier.customer_tier}>
                              <option value="all">ลูกค้าทุกคน</option>
                              <option value="wholesale">เฉพาะลูกค้าขายส่ง</option>
                            </Select>
                            {tier.branch_id ? <span className="text-2xs text-muted-foreground">เฉพาะบางสาขา</span> : null}
                          </td>
                          <td data-label="ซื้อตั้งแต่ (หน่วย)" className="p-2">
                            <Input aria-label="จำนวนขั้นต่ำ" disabled={!canEditTiers} min={1} onChange={(event) => setTiers(tiers.map((item, i) => i === index ? { ...item, min_quantity: event.target.value } : item))} type="number" value={tier.min_quantity} />
                          </td>
                          <td data-label="ราคาต่อหน่วย" className="p-2">
                            <Input aria-label="ราคาส่งต่อหน่วย" disabled={!canEditTiers} min={0.01} onChange={(event) => setTiers(tiers.map((item, i) => i === index ? { ...item, unit_price: event.target.value } : item))} required step="0.01" type="number" value={tier.unit_price} />
                          </td>
                          <td data-actions="true" className="p-2">
                            {canEditTiers ? (
                              <button aria-label="ลบราคาส่ง" className="grid h-9 w-9 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-destructive" onClick={() => setTiers(tiers.filter((_, i) => i !== index))} type="button">
                                <Trash2 className="h-4 w-4" />
                              </button>
                            ) : null}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : savedUnits.length ? <p className="text-sm text-muted-foreground">ยังไม่มีราคาส่ง สินค้านี้ขายราคาเดียวทุกจำนวน</p> : null}
              {canEditTiers ? (
                <div className="flex justify-end">
                  <Button disabled={!savedUnits.length} loading={busy} onClick={() => void saveTiers()} type="button">บันทึกราคาส่ง</Button>
                </div>
              ) : null}
            </section>
            <FeedbackNotice feedback={feedback} />
            <DialogFooter>
              <Button onClick={onClose} type="button" variant="secondary">ปิด</Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
