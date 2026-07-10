"use client";

import { FormEvent, startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function ProductConsole({
  branches,
  products
}: {
  branches: Option[];
  products: Option[];
}) {
  const router = useRouter();
  const [mode, setMode] = useState<"product" | "alias">("product");
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    try {
      if (mode === "product") {
        await proxyClient("/products", {
          method: "POST",
          body: JSON.stringify({
            sku: formData.get("sku"),
            name: formData.get("name"),
            description: formData.get("description"),
            cost_price: Number(formData.get("cost_price")),
            base_selling_price: Number(formData.get("base_selling_price")),
            unit_name: formData.get("unit_name"),
            tax_exempt: formData.get("tax_exempt") === "on",
            active: true
          })
        });
      } else {
        await proxyClient("/aliases", {
          method: "POST",
          body: JSON.stringify({
            product_id: formData.get("product_id"),
            branch_id: formData.get("branch_id") || undefined,
            alias_code: formData.get("alias_code"),
            alias_name: formData.get("alias_name"),
            default_government_price: Number(formData.get("default_government_price"))
          })
        });
      }
      setMessage("Saved");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Save failed");
    }
  }

  return (
    <SectionCard title="Catalog Actions" description="Products and alias mapping are stored centrally in PostgreSQL.">
      <div className="mb-4">
        <Select value={mode} onChange={(event) => setMode(event.target.value as "product" | "alias")}>
          <option value="product">Create Product</option>
          <option value="alias">Create Alias</option>
        </Select>
      </div>
      <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-3" onSubmit={submit}>
        {mode === "product" ? (
          <>
            <Input name="sku" placeholder="SKU" />
            <Input name="name" placeholder="Product name" />
            <Input name="description" placeholder="Description" />
            <Input name="cost_price" placeholder="Cost price" type="number" />
            <Input name="base_selling_price" placeholder="Base selling price" type="number" />
            <Input name="unit_name" placeholder="Unit" />
          </>
        ) : (
          <>
            <Select name="product_id">
              <option value="">Select product</option>
              {products.map((product) => (
                <option key={String(product.id)} value={String(product.id)}>
                  {String(product.name)}
                </option>
              ))}
            </Select>
            <Select name="branch_id">
              <option value="">Global alias</option>
              {branches.map((branch) => (
                <option key={String(branch.id)} value={String(branch.id)}>
                  {String(branch.name)}
                </option>
              ))}
            </Select>
            <Input name="alias_code" placeholder="Alias code" />
            <Input name="alias_name" placeholder="Alias name" />
            <Input name="default_government_price" placeholder="Default government price" type="number" />
          </>
        )}
        <div className="md:col-span-2 xl:col-span-3 flex items-center gap-3">
          <Button type="submit">Save</Button>
          {message ? <p className="text-sm text-black/70">{message}</p> : null}
        </div>
      </form>
    </SectionCard>
  );
}
