"use client";

import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { DataTable, SectionCard } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function InventoryConsole({
  products,
  branches,
  inventory = [],
  defaultBranchId,
  mode = "manage"
}: {
  products: Option[];
  branches: Option[];
  inventory?: Option[];
  defaultBranchId?: string;
  mode?: "manage" | "check";
}) {
  const router = useRouter();
  const [actionMode, setActionMode] = useState<"adjust" | "rebalance">("adjust");
  const [branchId, setBranchId] = useState(defaultBranchId || "");
  const [productId, setProductId] = useState("");
  const [stockBucket, setStockBucket] = useState("real");
  const [quantity, setQuantity] = useState("1");
  const [toBucket, setToBucket] = useState("ghost");
  const [reason, setReason] = useState("");
  const [message, setMessage] = useState("");
  const [search, setSearch] = useState("");

  const filteredInventory = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) {
      return inventory;
    }
    return inventory.filter((item) =>
      [item.product_name, item.sku, item.branch_name]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword))
    );
  }, [inventory, search]);
  const branchLabel = String(branches.find((item) => String(item.id) === branchId)?.name || "");

  async function submit() {
    const path = actionMode === "adjust" ? "/inventory/adjust" : "/inventory/rebalance";
    const payload =
      actionMode === "adjust"
        ? {
            branch_id: branchId,
            product_id: productId,
            stock_bucket: stockBucket,
            quantity_delta: Number(quantity),
            reason
          }
        : {
            branch_id: branchId,
            product_id: productId,
            from_bucket: stockBucket,
            to_bucket: toBucket,
            quantity: Number(quantity),
            reason
          };

    try {
      const response = await proxyClient<{ message: string }>(path, {
        method: "POST",
        body: JSON.stringify(payload)
      });
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Request failed");
    }
  }

  if (mode === "check") {
    return (
      <SectionCard
        title="Inventory Check"
        description="Read-only stock search for the assigned branch. Inventory values always come from the backend."
      >
        <div className="mb-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
          <Input
            aria-label="Inventory Search"
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search by product, SKU, or branch"
            value={search}
          />
          <Input aria-label="Branch Context" disabled value={branchLabel} />
        </div>
        <DataTable
          columns={[
            { key: "product_name", label: "Product" },
            { key: "sku", label: "SKU" },
            { key: "qty_real", label: "Real" },
            { key: "qty_ghost", label: "Ghost" },
            { key: "price", label: "Price", type: "currency" }
          ]}
          rows={filteredInventory}
        />
      </SectionCard>
    );
  }

  return (
    <SectionCard title="Inventory Actions" description="All stock movement is calculated and logged by the backend.">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <Select value={actionMode} onChange={(event) => setActionMode(event.target.value as "adjust" | "rebalance")}>
          <option value="adjust">Manual Adjust</option>
          <option value="rebalance">Real ↔ Ghost</option>
        </Select>
        <Select value={branchId} onChange={(event) => setBranchId(event.target.value)}>
          <option value="">Select branch</option>
          {branches.map((branch) => (
            <option key={String(branch.id)} value={String(branch.id)}>
              {String(branch.name)}
            </option>
          ))}
        </Select>
        <Select value={productId} onChange={(event) => setProductId(event.target.value)}>
          <option value="">Select product</option>
          {products.map((product) => (
            <option key={String(product.id)} value={String(product.id)}>
              {String(product.name)}
            </option>
          ))}
        </Select>
        <Select value={stockBucket} onChange={(event) => setStockBucket(event.target.value)}>
          <option value="real">Real</option>
          <option value="ghost">Ghost</option>
        </Select>
        {actionMode === "rebalance" ? (
          <Select value={toBucket} onChange={(event) => setToBucket(event.target.value)}>
            <option value="real">Real</option>
            <option value="ghost">Ghost</option>
          </Select>
        ) : (
          <Input value={quantity} onChange={(event) => setQuantity(event.target.value)} type="number" />
        )}
        {actionMode === "rebalance" ? (
          <Input value={quantity} onChange={(event) => setQuantity(event.target.value)} type="number" />
        ) : null}
        <Input className="xl:col-span-2" value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Reason" />
        <Button onClick={submit} type="button">
          {actionMode === "adjust" ? "Apply Adjustment" : "Rebalance Stock"}
        </Button>
      </div>
      {message ? <p className="mt-4 text-sm text-black/70">{message}</p> : null}
    </SectionCard>
  );
}
