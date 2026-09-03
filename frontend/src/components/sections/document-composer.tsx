"use client";

import { startTransition, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard, InvoiceSummary } from "@/components/sections/common";
import { ProductSearchPicker } from "@/components/sections/product-search-picker";
import { Button, CheckboxField, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

type ComposerProps = {
  kind: "invoice" | "quotation";
  title: string;
  description: string;
  defaultBranchId?: string;
  branches: Option[];
  products: Option[];
  governmentMode?: boolean;
  canUseGhost?: boolean;
  onCreated?: () => void;
};

type LineState = {
  product_id: string;
  inventory_lot_id: string;
  alias_id: string;
  quantity: string;
  stock_bucket: "real" | "ghost";
  override_unit_price: string;
  override_reason: string;
};

const emptyLine: LineState = {
  product_id: "",
  inventory_lot_id: "",
  alias_id: "",
  quantity: "1",
  stock_bucket: "real",
  override_unit_price: "",
  override_reason: ""
};

export function DocumentComposer({
  kind,
  title,
  description,
  defaultBranchId,
  branches,
  products,
  governmentMode,
  canUseGhost = false,
  onCreated
}: ComposerProps) {
  const router = useRouter();
  const [branchId, setBranchId] = useState(defaultBranchId || "");
  const [customerName, setCustomerName] = useState("");
  const [customerTaxID, setCustomerTaxID] = useState("");
  const [isGovernmentMode, setGovernmentMode] = useState(governmentMode ?? false);
  const [fullTaxInvoice, setFullTaxInvoice] = useState(false);
  const [lines, setLines] = useState<LineState[]>([{ ...emptyLine }]);
  const [aliasOptions, setAliasOptions] = useState<Record<string, Option[]>>({});
  const [lotOptions, setLotOptions] = useState<Record<string, Option[]>>({});
  const [preview, setPreview] = useState<Record<string, unknown> | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // Brief inline notice when a duplicate product+stock-type line was merged
  // into an existing row instead of being kept as a second line.
  const [mergeNotice, setMergeNotice] = useState("");
  useEffect(() => {
    if (!mergeNotice) return;
    const timer = window.setTimeout(() => setMergeNotice(""), 5000);
    return () => window.clearTimeout(timer);
  }, [mergeNotice]);

  function updateLine(index: number, key: keyof LineState, value: string) {
    setLines((current) => {
      const next = current.map((line, currentIndex) =>
        currentIndex === index
          ? {
              ...line,
              [key]: value,
              ...((key === "product_id" || key === "stock_bucket")
                ? { alias_id: key === "product_id" ? "" : line.alias_id, inventory_lot_id: "" }
                : {})
            }
          : line
      );
      // Same product + same stock type = one line: merge the quantity into
      // the earlier row instead of keeping a duplicate
      // (business-flow.md, duplicate line-item rule).
      if (key !== "product_id" && key !== "stock_bucket") return next;
      const edited = next[index];
      if (!edited.product_id) return next;
      const targetIndex = next.findIndex(
        (line, currentIndex) =>
          currentIndex !== index &&
          line.product_id === edited.product_id &&
          line.stock_bucket === edited.stock_bucket
      );
      if (targetIndex === -1) return next;
      const productName = String(products.find((item) => String(item.id) === edited.product_id)?.name || "สินค้านี้");
      setMergeNotice(`รวมจำนวน “${productName}” เข้ากับรายการเดิมแล้ว (สินค้าและประเภทสต๊อกเดียวกัน)`);
      return next
        .filter((_, currentIndex) => currentIndex !== index)
        .map((line, currentIndex) =>
          currentIndex === (targetIndex > index ? targetIndex - 1 : targetIndex)
            ? { ...line, quantity: String((Number(line.quantity) || 0) + (Number(edited.quantity) || 0)) }
            : line
        );
    });
  }

  const requestedAliasKeys = useMemo(() => {
    if (!isGovernmentMode || !branchId) {
      return [];
    }
    return Array.from(
      new Set(
        lines
          .map((line) => line.product_id.trim())
          .filter(Boolean)
          .map((productId) => `${branchId}:${productId}`)
      )
    );
  }, [branchId, isGovernmentMode, lines]);

  useEffect(() => {
    if (isGovernmentMode) {
      return;
    }
    setLines((current) =>
      current.map((line) => (line.alias_id ? { ...line, alias_id: "" } : line))
    );
  }, [isGovernmentMode]);

  useEffect(() => {
    setLines((current) =>
      current.map((line) => ({ ...line, alias_id: "", inventory_lot_id: "" }))
    );
    setPreview(null);
  }, [branchId]);

  useEffect(() => {
    const missingKeys = requestedAliasKeys.filter((key) => aliasOptions[key] === undefined);
    if (!missingKeys.length) {
      return;
    }

    let active = true;
    void Promise.all(
      missingKeys.map(async (key) => {
        const [, productId] = key.split(":");
        const result = await proxyClient<{ items: Option[] }>(
          `/aliases?branch_id=${encodeURIComponent(branchId)}&product_id=${encodeURIComponent(productId)}`
        );
        return {
          key,
          items: result.items.filter((item) => Boolean(item.active ?? true))
        };
      })
    )
      .then((results) => {
        if (!active) {
          return;
        }
        setAliasOptions((current) => {
          const next = { ...current };
          results.forEach((result) => {
            next[result.key] = result.items;
          });
          return next;
        });
      })
      .catch(() => {
        // Backend validation remains authoritative; the form can still submit and surface API errors.
      });

    return () => {
      active = false;
    };
  }, [aliasOptions, branchId, requestedAliasKeys]);

  const requestedLotKeys = useMemo(() => {
    if (kind !== "invoice" || !branchId) return [];
    return Array.from(new Set(lines
      .filter((line) => line.product_id)
      .map((line) => `${branchId}:${line.product_id}:${line.stock_bucket}`)));
  }, [branchId, kind, lines]);

  useEffect(() => {
    const missingKeys = requestedLotKeys.filter((key) => lotOptions[key] === undefined);
    if (!missingKeys.length) return;
    let active = true;
    void Promise.all(missingKeys.map(async (key) => {
      const [requestedBranchID, productID, bucket] = key.split(":");
      const query = new URLSearchParams({ branch_id: requestedBranchID, product_id: productID, stock_bucket: bucket });
      const result = await proxyClient<{ items: Option[] }>(`/sales/lot-options?${query.toString()}`);
      return { key, items: result.items };
    })).then((results) => {
      if (!active) return;
      setLotOptions((current) => {
        const next = { ...current };
        results.forEach((result) => { next[result.key] = result.items; });
        return next;
      });
    }).catch((caught) => setMessage(caught instanceof Error ? caught.message : "โหลด Lot ไม่สำเร็จ"));
    return () => { active = false; };
  }, [lotOptions, requestedLotKeys]);

  async function handlePreview() {
    setMessage(null);
    const result = await proxyClient<Record<string, unknown>>(
      kind === "invoice" ? "/invoices/preview" : "/quotations/preview",
      {
        method: "POST",
        body: JSON.stringify(buildPayload())
      }
    );
    setPreview(result);
  }

  async function handleSubmit() {
    setSubmitting(true);
    setMessage(null);

    try {
      const response = await proxyClient<{ message?: string }>(
        kind === "invoice" ? "/invoices" : "/quotations",
        {
          method: "POST",
          body: JSON.stringify(buildPayload())
        }
      );
      setMessage(response.message || (kind === "invoice" ? "สร้างใบขายแล้ว" : "สร้างใบเสนอราคาแล้ว"));
      onCreated?.();
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "สร้างเอกสารไม่สำเร็จ");
    } finally {
      setSubmitting(false);
    }
  }

  function buildPayload() {
    return {
      branch_id: branchId,
      customer_name: customerName,
      customer_tax_id: customerTaxID,
      is_government_mode: isGovernmentMode,
      full_tax_invoice: kind === "invoice" && fullTaxInvoice,
      items: lines
        .filter((line) => line.product_id)
        .map((line) => ({
          product_id: line.product_id,
          inventory_lot_id: kind === "invoice" ? line.inventory_lot_id : undefined,
          alias_id: isGovernmentMode && line.alias_id ? line.alias_id : undefined,
          quantity: Number(line.quantity),
          stock_bucket: line.stock_bucket,
          override_unit_price: line.override_unit_price
            ? Number(line.override_unit_price)
            : undefined,
          override_reason: line.override_reason
        }))
    };
  }

  return (
    <SectionCard
      title={title}
      description={description}
      actions={
        <div className="flex gap-2">
          <Button variant="secondary" onClick={handlePreview} type="button">
            ตรวจสอบยอด
          </Button>
          <Button disabled={submitting} onClick={handleSubmit} type="button">
            {submitting
              ? "กำลังบันทึก..."
              : kind === "invoice"
                ? "สร้างใบขาย"
                : "สร้างใบเสนอราคา"}
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="grid gap-4 md:grid-cols-4">
          <label className="space-y-2">
            <span className="text-sm font-medium text-muted-foreground">สาขา</span>
            <Select
              aria-label="สาขา"
              value={branchId}
              onChange={(event) => setBranchId(event.target.value)}
            >
              <option value="">เลือกสาขา</option>
              {branches.filter((branch) => Boolean(branch.sales_enabled ?? true)).map((branch) => (
                <option key={String(branch.id)} value={String(branch.id)}>
                  {String(branch.name)}
                </option>
              ))}
            </Select>
          </label>
          <label className="space-y-2 md:col-span-2">
            <span className="text-sm font-medium text-muted-foreground">ชื่อลูกค้า</span>
            <Input
              aria-label="ชื่อลูกค้า"
              value={customerName}
              onChange={(event) => setCustomerName(event.target.value)}
            />
          </label>
          <label className="space-y-2">
            <span className="text-sm font-medium text-muted-foreground">เลขประจำตัวผู้เสียภาษี</span>
            <Input
              aria-label="เลขประจำตัวผู้เสียภาษี"
              value={customerTaxID}
              onChange={(event) => setCustomerTaxID(event.target.value)}
            />
          </label>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          {governmentMode === undefined ? (
            <CheckboxField
              aria-label="โหมดราชการ"
              checked={isGovernmentMode}
              label="โหมดราชการ"
              onChange={(event) => setGovernmentMode(event.target.checked)}
            />
          ) : null}
          {kind === "invoice" ? (
            <CheckboxField
              aria-label="ใบกำกับภาษีเต็มรูป"
              checked={fullTaxInvoice}
              label="ออกใบกำกับภาษีเต็มรูป"
              onChange={(event) => setFullTaxInvoice(event.target.checked)}
            />
          ) : null}
        </div>

        {mergeNotice ? (
          <p className="rounded-lg bg-info-50 px-4 py-2 text-sm text-info-800" role="status">
            {mergeNotice}
          </p>
        ) : null}

        <div className="space-y-3">
          {lines.map((line, index) => {
            const lineAliasOptions =
              aliasOptions[`${branchId}:${line.product_id}`] || [];
            const lineLotOptions = lotOptions[`${branchId}:${line.product_id}:${line.stock_bucket}`] || [];

            return (
              <div
                key={index}
                className="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 md:grid-cols-7"
              >
                <ProductSearchPicker
                  ariaLabel={`สินค้า ${index + 1}`}
                  initialOptions={products}
                  onChange={(value) => updateLine(index, "product_id", value)}
                  value={line.product_id}
                />
                {kind === "invoice" ? (
                  <Select
                    aria-label={`Lot ${index + 1}`}
                    disabled={!branchId || !line.product_id}
                    value={line.inventory_lot_id}
                    onChange={(event) => updateLine(index, "inventory_lot_id", event.target.value)}
                  >
                    <option value="">เลือก Lot</option>
                    {lineLotOptions.map((lot) => (
                      <option key={String(lot.id)} value={String(lot.id)}>
                        {`${String(lot.lot_number)} · คงเหลือ ${Number(lot.remaining_quantity || 0).toLocaleString("th-TH")} · ต้นทุน ${Number(lot.unit_cost || 0).toLocaleString("th-TH")} บาท`}
                      </option>
                    ))}
                  </Select>
                ) : null}
                <Select
                  aria-label={`ชื่อบนเอกสาร ${index + 1}`}
                  disabled={!isGovernmentMode || !branchId || !line.product_id}
                  value={line.alias_id}
                  onChange={(event) => updateLine(index, "alias_id", event.target.value)}
                >
                  <option value="">
                    {!line.product_id
                      ? "เลือกสินค้าก่อน"
                      : isGovernmentMode
                        ? "เลือกชื่อที่แสดง"
                        : "เปิดโหมดราชการ"}
                  </option>
                  {lineAliasOptions.map((alias) => (
                    <option key={String(alias.id)} value={String(alias.id)}>
                      {String(alias.alias_name)}
                    </option>
                  ))}
                </Select>
                <Input
                  aria-label={`จำนวน ${index + 1}`}
                  min="1"
                  onChange={(event) => updateLine(index, "quantity", event.target.value)}
                  type="number"
                  value={line.quantity}
                />
                <Select
                  aria-label={`ประเภทสต๊อก ${index + 1}`}
                  value={line.stock_bucket}
                  onChange={(event) => updateLine(index, "stock_bucket", event.target.value)}
                >
                  <option value="real">สต๊อกจริง</option>
                  {canUseGhost ? <option value="ghost">สต๊อกผี</option> : null}
                </Select>
                <Input
                  aria-label={`ราคาพิเศษ ${index + 1}`}
                  onChange={(event) => updateLine(index, "override_unit_price", event.target.value)}
                  placeholder="ราคาพิเศษ"
                  type="number"
                  value={line.override_unit_price}
                />
                <Input
                  aria-label={`เหตุผลราคาพิเศษ ${index + 1}`}
                  onChange={(event) => updateLine(index, "override_reason", event.target.value)}
                  placeholder="เหตุผล"
                  value={line.override_reason}
                />
              </div>
            );
          })}
        </div>

        <Button
          onClick={() => setLines((current) => [...current, { ...emptyLine }])}
          type="button"
          variant="secondary"
        >
          เพิ่มรายการ
        </Button>

        {message ? <p className="text-sm text-muted-foreground">{message}</p> : null}
        <InvoiceSummary summary={(preview?.summary as Record<string, unknown>) || undefined} />
      </div>
    </SectionCard>
  );
}
