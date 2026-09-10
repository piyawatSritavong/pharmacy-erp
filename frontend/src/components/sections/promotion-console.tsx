"use client";

import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Trash2 } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Badge, Button, EmptyState, Input, Notice, Select } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

type PromotionItemRow = {
  product_id: string;
  quantity: string;
  role: "condition" | "reward" | "bundle_item";
};

const TYPE_LABELS: Record<string, string> = {
  buy_x_get_y: "ซื้อ X แถม Y",
  percent: "ลดเป็นเปอร์เซ็นต์",
  amount: "ลดเป็นจำนวนเงิน",
  bundle: "ราคาชุด (จัดเซ็ต)",
  bill_giveaway: "ของแถมท้ายบิล"
};

const ROLE_LABELS: Record<string, string> = {
  condition: "เงื่อนไข (ต้องซื้อ)",
  reward: "ของแถม",
  bundle_item: "สินค้าในชุด"
};

// Which item roles each promotion type needs, so the form only asks for what
// the backend will actually validate.
const ROLES_BY_TYPE: Record<string, PromotionItemRow["role"][]> = {
  buy_x_get_y: ["condition", "reward"],
  percent: ["condition"],
  amount: ["condition"],
  bundle: ["bundle_item"],
  bill_giveaway: ["reward"]
};

export function PromotionConsole({
  promotions,
  products,
  branches,
  ownBranchName
}: {
  promotions: Option[];
  products: Option[];
  branches: Option[];
  /**
   * Set for a shop, empty for head office. A shop's promotions are its own —
   * the server pins every write to the branch the user signed in from — so the
   * branch picker has nothing to ask and says which shop instead.
   */
  ownBranchName?: string;
}) {
  const router = useRouter();
  const [promoType, setPromoType] = useState<keyof typeof TYPE_LABELS>("buy_x_get_y");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [branchId, setBranchId] = useState("");
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState("");
  const [minQuantity, setMinQuantity] = useState("");
  const [minAmount, setMinAmount] = useState("");
  const [discountPercent, setDiscountPercent] = useState("");
  const [discountAmount, setDiscountAmount] = useState("");
  const [bundlePrice, setBundlePrice] = useState("");
  const [maxUses, setMaxUses] = useState("0");
  const [rows, setRows] = useState<PromotionItemRow[]>([{ product_id: "", quantity: "1", role: "condition" }]);
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);

  const allowedRoles = ROLES_BY_TYPE[promoType] || ["condition"];
  const productName = useMemo(() => {
    const lookup = new Map(products.map((product) => [String(product.id), String(product.name)]));
    return (id: string) => lookup.get(id) || id;
  }, [products]);

  function updateRow(index: number, patch: Partial<PromotionItemRow>) {
    setRows((current) => current.map((row, position) => (position === index ? { ...row, ...patch } : row)));
  }

  function resetForm() {
    setCode(""); setName(""); setStartsAt(""); setEndsAt("");
    setMinQuantity(""); setMinAmount(""); setDiscountPercent(""); setDiscountAmount("");
    setBundlePrice(""); setMaxUses("0");
    setRows([{ product_id: "", quantity: "1", role: allowedRoles[0] }]);
  }

  async function save() {
    setSaving(true);
    setMessage("");
    try {
      const response = await proxyClient<{ message?: string }>("/promotions", {
        method: "POST",
        body: JSON.stringify({
          code,
          name,
          promo_type: promoType,
          branch_id: branchId,
          starts_at: startsAt,
          ends_at: endsAt,
          active: true,
          min_quantity: Number(minQuantity || 0),
          min_amount: Number(minAmount || 0),
          discount_percent: Number(discountPercent || 0),
          discount_amount: Number(discountAmount || 0),
          bundle_price: bundlePrice ? Number(bundlePrice) : null,
          max_uses_per_bill: Number(maxUses || 0),
          items: rows
            .filter((row) => row.product_id)
            .map((row) => ({
              product_id: row.product_id,
              quantity: Number(row.quantity || 1),
              role: row.role
            }))
        })
      });
      setMessage(response.message || "สร้างโปรโมชั่นแล้ว");
      resetForm();
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "บันทึกไม่สำเร็จ");
    } finally {
      setSaving(false);
    }
  }

  async function remove(id: string) {
    try {
      await proxyClient(`/promotions/${id}`, { method: "DELETE" });
      setMessage("ลบโปรโมชั่นแล้ว");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ลบไม่สำเร็จ");
    }
  }

  return (
    <div className="space-y-6">
      <SectionCard
        title="สร้างโปรโมชั่น"
        description="โปรโมชั่นที่เปิดใช้งานจะถูกคิดให้อัตโนมัติที่หน้าขาย และของแถมจะตัดสต๊อกจริงตามจำนวนที่แถม"
      >
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <Field label="ชนิดโปรโมชั่น">
            <Select
              aria-label="ชนิดโปรโมชั่น"
              onChange={(event) => {
                const next = event.target.value as keyof typeof TYPE_LABELS;
                setPromoType(next);
                setRows([{ product_id: "", quantity: "1", role: (ROLES_BY_TYPE[next] || ["condition"])[0] }]);
              }}
              value={promoType}
            >
              {Object.entries(TYPE_LABELS).map(([value, label]) => (
                <option key={value} value={value}>{label}</option>
              ))}
            </Select>
          </Field>
          <Field label="รหัสโปรโมชั่น">
            <Input aria-label="รหัสโปรโมชั่น" onChange={(event) => setCode(event.target.value)} placeholder="เช่น BUY10GET1" value={code} />
          </Field>
          <Field label="ชื่อโปรโมชั่น">
            <Input aria-label="ชื่อโปรโมชั่น" onChange={(event) => setName(event.target.value)} placeholder="เช่น ซื้อ 10 แถม 1" value={name} />
          </Field>
          <Field hint={ownBranchName ? "โปรโมชั่นนี้ใช้ที่สาขานี้เท่านั้น" : undefined} label="สาขา">
            {ownBranchName ? (
              <Input aria-label="สาขาโปรโมชั่น" disabled readOnly value={ownBranchName} />
            ) : (
              <Select aria-label="สาขาโปรโมชั่น" onChange={(event) => setBranchId(event.target.value)} value={branchId}>
                <option value="">ทุกสาขา</option>
                {branches.map((branch) => (
                  <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="เริ่มวันที่">
            <Input aria-label="เริ่มวันที่" onChange={(event) => setStartsAt(event.target.value)} type="date" value={startsAt} />
          </Field>
          <Field label="สิ้นสุดวันที่">
            <Input aria-label="สิ้นสุดวันที่" onChange={(event) => setEndsAt(event.target.value)} type="date" value={endsAt} />
          </Field>

          {promoType === "percent" ? (
            <Field label="ส่วนลด (%)">
              <Input aria-label="ส่วนลดเปอร์เซ็นต์" inputMode="decimal" onChange={(event) => setDiscountPercent(event.target.value)} value={discountPercent} />
            </Field>
          ) : null}
          {promoType === "amount" ? (
            <Field label="ส่วนลด (บาท)">
              <Input aria-label="ส่วนลดจำนวนเงิน" inputMode="decimal" onChange={(event) => setDiscountAmount(event.target.value)} value={discountAmount} />
            </Field>
          ) : null}
          {promoType === "bundle" ? (
            <Field label="ราคาชุด (บาท)">
              <Input aria-label="ราคาชุด" inputMode="decimal" onChange={(event) => setBundlePrice(event.target.value)} value={bundlePrice} />
            </Field>
          ) : null}
          {promoType === "bill_giveaway" ? (
            <Field hint="ยอดซื้อขั้นต่ำที่ลูกค้าต้องซื้อจึงจะได้ของแถม" label="ยอดซื้อขั้นต่ำ (บาท)">
              <Input aria-label="ยอดซื้อขั้นต่ำ" inputMode="decimal" onChange={(event) => setMinAmount(event.target.value)} value={minAmount} />
            </Field>
          ) : null}
          {promoType === "percent" || promoType === "amount" ? (
            <Field hint="เว้นว่างได้หากไม่มีเงื่อนไขขั้นต่ำ" label="ซื้อขั้นต่ำ (ชิ้น)">
              <Input aria-label="ซื้อขั้นต่ำ" inputMode="numeric" onChange={(event) => setMinQuantity(event.target.value)} value={minQuantity} />
            </Field>
          ) : null}
          <Field hint="0 = ไม่จำกัดจำนวนครั้งต่อบิล" label="จำกัดต่อบิล (ครั้ง)">
            <Input aria-label="จำกัดต่อบิล" inputMode="numeric" onChange={(event) => setMaxUses(event.target.value)} value={maxUses} />
          </Field>
        </div>

        <div className="mt-6 space-y-3">
          <p className="text-sm font-semibold">สินค้าในโปรโมชั่น</p>
          {rows.map((row, index) => (
            <div className="grid gap-3 md:grid-cols-[2fr_1fr_1fr_auto]" key={index}>
              <Select
                aria-label={`สินค้า ${index + 1}`}
                onChange={(event) => updateRow(index, { product_id: event.target.value })}
                value={row.product_id}
              >
                <option value="">เลือกสินค้า</option>
                {products.map((product) => (
                  <option key={String(product.id)} value={String(product.id)}>{String(product.name)}</option>
                ))}
              </Select>
              <Select
                aria-label={`บทบาท ${index + 1}`}
                onChange={(event) => updateRow(index, { role: event.target.value as PromotionItemRow["role"] })}
                value={row.role}
              >
                {allowedRoles.map((role) => (
                  <option key={role} value={role}>{ROLE_LABELS[role]}</option>
                ))}
              </Select>
              <Input
                aria-label={`จำนวน ${index + 1}`}
                inputMode="numeric"
                onChange={(event) => updateRow(index, { quantity: event.target.value })}
                placeholder="จำนวน"
                value={row.quantity}
              />
              <Button
                aria-label={`ลบแถว ${index + 1}`}
                onClick={() => setRows((current) => current.filter((_, position) => position !== index))}
                type="button"
                variant="ghost"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <Button
            onClick={() => setRows((current) => [...current, { product_id: "", quantity: "1", role: allowedRoles[0] }])}
            type="button"
            variant="secondary"
          >
            เพิ่มสินค้า
          </Button>
        </div>

        {message ? <Notice className="mt-4">{message}</Notice> : null}

        <div className="mt-5">
          <Button disabled={saving || !code || !name} onClick={save} type="button">
            {saving ? "กำลังบันทึก..." : "สร้างโปรโมชั่น"}
          </Button>
        </div>
      </SectionCard>

      <SectionCard title="โปรโมชั่นทั้งหมด" description="โปรที่เปิดใช้งานและอยู่ในช่วงวันที่จะถูกใช้ที่หน้าขายทันที">
        {promotions.length === 0 ? (
          <EmptyState description="สร้างโปรโมชั่นรายการแรกจากแบบฟอร์มด้านบน" />
        ) : (
          <div className="space-y-3">
            {promotions.map((promotion) => (
              <div className="rounded-lg border p-4" key={String(promotion.id)}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-semibold">{String(promotion.name)}</p>
                      <Badge>{TYPE_LABELS[String(promotion.promo_type)] || String(promotion.promo_type)}</Badge>
                      <Badge className={promotion.active ? "bg-success/10 text-success" : "bg-muted"}>
                        {promotion.active ? "เปิดใช้งาน" : "ปิดอยู่"}
                      </Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      รหัส {String(promotion.code)} · {String(promotion.branch_name || "ทุกสาขา")}
                      {promotion.starts_at ? ` · เริ่ม ${String(promotion.starts_at)}` : ""}
                      {promotion.ends_at ? ` · ถึง ${String(promotion.ends_at)}` : ""}
                    </p>
                    <p className="mt-2 text-xs text-muted-foreground">
                      {((promotion.items as Option[]) || [])
                        .map((item) => `${ROLE_LABELS[String(item.role)]}: ${productName(String(item.product_id))} × ${Number(item.quantity)}`)
                        .join(" | ") || "ไม่ได้ระบุสินค้า"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {Number(promotion.discount_percent) > 0 ? `ลด ${Number(promotion.discount_percent)}% ` : ""}
                      {Number(promotion.discount_amount) > 0 ? `ลด ${currency(Number(promotion.discount_amount))} ` : ""}
                      {promotion.bundle_price ? `ราคาชุด ${currency(Number(promotion.bundle_price))} ` : ""}
                      {Number(promotion.min_amount) > 0 ? `· ซื้อขั้นต่ำ ${currency(Number(promotion.min_amount))}` : ""}
                    </p>
                  </div>
                  {promotion.editable === false ? (
                    <span className="whitespace-nowrap rounded-full bg-muted px-3 py-1 text-xs font-medium text-muted-foreground">
                      ตั้งจากสำนักงานใหญ่
                    </span>
                  ) : (
                    <Button
                      aria-label={`ลบโปรโมชั่น ${String(promotion.name)}`}
                      onClick={() => remove(String(promotion.id))}
                      type="button"
                      variant="ghost"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  );
}
