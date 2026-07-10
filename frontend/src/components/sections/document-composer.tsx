"use client";

import { startTransition, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard, InvoiceSummary } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

type ComposerProps = {
  kind: "invoice" | "quotation";
  title: string;
  description: string;
  defaultBranchId?: string;
  branches: Option[];
  products: Option[];
};

type LineState = {
  product_id: string;
  alias_id: string;
  quantity: string;
  stock_bucket: "real" | "ghost";
  override_unit_price: string;
  override_reason: string;
};

const emptyLine: LineState = {
  product_id: "",
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
  products
}: ComposerProps) {
  const router = useRouter();
  const [branchId, setBranchId] = useState(defaultBranchId || "");
  const [customerName, setCustomerName] = useState("");
  const [customerTaxID, setCustomerTaxID] = useState("");
  const [isGovernmentMode, setGovernmentMode] = useState(false);
  const [lines, setLines] = useState<LineState[]>([{ ...emptyLine }]);
  const [aliasOptions, setAliasOptions] = useState<Record<string, Option[]>>({});
  const [preview, setPreview] = useState<Record<string, unknown> | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function updateLine(index: number, key: keyof LineState, value: string) {
    setLines((current) =>
      current.map((line, currentIndex) =>
        currentIndex === index
          ? {
              ...line,
              [key]: value,
              ...(key === "product_id" ? { alias_id: "" } : {})
            }
          : line
      )
    );
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
      current.map((line) => (line.alias_id ? { ...line, alias_id: "" } : line))
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
      setMessage(response.message || `${kind} created`);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : `Failed to create ${kind}`);
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
      items: lines
        .filter((line) => line.product_id)
        .map((line) => ({
          product_id: line.product_id,
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
            Preview
          </Button>
          <Button disabled={submitting} onClick={handleSubmit} type="button">
            {submitting
              ? "Submitting..."
              : kind === "invoice"
                ? "Create Invoice"
                : "Create Quotation"}
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="grid gap-4 md:grid-cols-4">
          <label className="space-y-2">
            <span className="text-sm font-medium text-black/70">Branch</span>
            <Select
              aria-label="Branch"
              value={branchId}
              onChange={(event) => setBranchId(event.target.value)}
            >
              <option value="">Select branch</option>
              {branches.map((branch) => (
                <option key={String(branch.id)} value={String(branch.id)}>
                  {String(branch.name)}
                </option>
              ))}
            </Select>
          </label>
          <label className="space-y-2 md:col-span-2">
            <span className="text-sm font-medium text-black/70">Customer Name</span>
            <Input
              aria-label="Customer Name"
              value={customerName}
              onChange={(event) => setCustomerName(event.target.value)}
            />
          </label>
          <label className="space-y-2">
            <span className="text-sm font-medium text-black/70">Tax ID</span>
            <Input
              aria-label="Tax ID"
              value={customerTaxID}
              onChange={(event) => setCustomerTaxID(event.target.value)}
            />
          </label>
        </div>

        <label className="flex items-center gap-3 rounded-[22px] border border-black/10 bg-surface-50 px-4 py-3 text-sm">
          <input
            aria-label="Government mode"
            checked={isGovernmentMode}
            onChange={(event) => setGovernmentMode(event.target.checked)}
            type="checkbox"
          />
          Government mode
        </label>

        <div className="space-y-3">
          {lines.map((line, index) => {
            const lineAliasOptions =
              aliasOptions[`${branchId}:${line.product_id}`] || [];

            return (
              <div key={index} className="grid gap-3 rounded-[24px] border border-black/10 bg-surface-50 p-4 md:grid-cols-6">
                <Select
                  aria-label={`Product ${index + 1}`}
                  value={line.product_id}
                  onChange={(event) => updateLine(index, "product_id", event.target.value)}
                >
                  <option value="">Select product</option>
                  {products.map((product) => (
                    <option key={String(product.id)} value={String(product.id)}>
                      {String(product.name)}
                    </option>
                  ))}
                </Select>
                <Select
                  aria-label={`Alias ${index + 1}`}
                  disabled={!isGovernmentMode || !branchId || !line.product_id}
                  value={line.alias_id}
                  onChange={(event) => updateLine(index, "alias_id", event.target.value)}
                >
                  <option value="">
                    {!line.product_id
                      ? "Select product first"
                      : isGovernmentMode
                        ? "Alias display"
                        : "Enable government mode"}
                  </option>
                  {lineAliasOptions.map((alias) => (
                    <option key={String(alias.id)} value={String(alias.id)}>
                      {String(alias.alias_name)}
                    </option>
                  ))}
                </Select>
                <Input
                  aria-label={`Quantity ${index + 1}`}
                  min="1"
                  onChange={(event) => updateLine(index, "quantity", event.target.value)}
                  type="number"
                  value={line.quantity}
                />
                <Select
                  aria-label={`Stock Bucket ${index + 1}`}
                  value={line.stock_bucket}
                  onChange={(event) => updateLine(index, "stock_bucket", event.target.value)}
                >
                  <option value="real">Real</option>
                  <option value="ghost">Ghost</option>
                </Select>
                <Input
                  aria-label={`Override Price ${index + 1}`}
                  onChange={(event) => updateLine(index, "override_unit_price", event.target.value)}
                  placeholder="Override price"
                  type="number"
                  value={line.override_unit_price}
                />
                <Input
                  aria-label={`Override Reason ${index + 1}`}
                  onChange={(event) => updateLine(index, "override_reason", event.target.value)}
                  placeholder="Reason"
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
          Add Line
        </Button>

        {message ? <p className="text-sm text-black/70">{message}</p> : null}
        <InvoiceSummary summary={(preview?.summary as Record<string, unknown>) || undefined} />
      </div>
    </SectionCard>
  );
}
