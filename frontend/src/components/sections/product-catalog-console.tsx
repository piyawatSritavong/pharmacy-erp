"use client";

import { FormEvent, startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Plus, Search } from "lucide-react";
import { z } from "zod";

import { DataTable, SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogHeader,
  Input,
  Pagination,
  Select,
  Textarea
} from "@/components/ui/primitives";
import type { PaginationState } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";
import { rules, useFormErrors } from "@/lib/validation";

type Option = Record<string, unknown>;

const CHANNEL_LABEL: Record<string, string> = {
  in_store: "หน้าร้านเท่านั้น",
  online: "ออนไลน์เท่านั้น",
  both: "หน้าร้าน + ออนไลน์"
};

const productSchema = z.object({
  sku: rules.text("SKU", { max: 40 }),
  name: rules.text("ชื่อสินค้า", { max: 200 }),
  unit_name: rules.text("หน่วยนับ", { max: 20 })
});

const blankProduct: Option = {
  sku: "",
  barcode: "",
  category_id: "",
  name: "",
  description: "",
  cost_price: 0,
  base_selling_price: 0,
  max_discount_amount: 0,
  unit_name: "",
  tax_exempt: false,
  active: true,
  sales_channel: "in_store",
  requires_fda_report: false,
  fda_registration_no: ""
};

// Part B, Rule 2 — the browse/manage screen the "central Product Catalog"
// rule was missing. products is already the one cross-branch table (no new
// data layer here beyond sales_channel + the อย. flags added alongside it);
// this is that page.
export function ProductCatalogConsole({
  initialItems,
  categories,
  defaultSearch,
  defaultCategoryId,
  defaultSalesChannel,
  pagination
}: {
  initialItems: Option[];
  categories: Option[];
  defaultSearch: string;
  defaultCategoryId: string;
  defaultSalesChannel: string;
  pagination: PaginationState;
}) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Option>(blankProduct);
  const [editingId, setEditingId] = useState<string | null>(null);
  const errors = useFormErrors(productSchema);

  const rows = useMemo(
    () =>
      initialItems.map((item) => ({
        ...item,
        sales_channel_label: CHANNEL_LABEL[String(item.sales_channel || "in_store")] || String(item.sales_channel)
      })),
    [initialItems]
  );

  function openCreate() {
    setEditing({ ...blankProduct });
    setEditingId(null);
    errors.setErrors({});
    setMessage("");
    setDialogOpen(true);
  }

  function openEdit(item: Option) {
    setEditing({ ...blankProduct, ...item, category_id: item.category_id ? String(item.category_id) : "" });
    setEditingId(String(item.id));
    errors.setErrors({});
    setMessage("");
    setDialogOpen(true);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const fields = {
      sku: String(editing.sku || ""),
      name: String(editing.name || ""),
      unit_name: String(editing.unit_name || "")
    };
    const result = errors.validate(fields);
    if (!result.success) return;

    const body = {
      sku: editing.sku,
      barcode: editing.barcode || undefined,
      category_id: editing.category_id || undefined,
      name: editing.name,
      description: editing.description || "",
      cost_price: Number(editing.cost_price) || 0,
      base_selling_price: Number(editing.base_selling_price) || 0,
      max_discount_amount: Number(editing.max_discount_amount) || 0,
      low_stock_real_threshold: Number(editing.low_stock_real_threshold) || 0,
      low_stock_ghost_threshold: Number(editing.low_stock_ghost_threshold) || 0,
      tracks_expiry: Boolean(editing.tracks_expiry),
      expiry_warning_days: Number(editing.expiry_warning_days) || 30,
      unit_name: editing.unit_name,
      tax_exempt: Boolean(editing.tax_exempt),
      active: Boolean(editing.active),
      sales_channel: editing.sales_channel || "in_store",
      requires_fda_report: Boolean(editing.requires_fda_report),
      fda_registration_no: editing.fda_registration_no || ""
    };

    try {
      if (editingId) {
        await proxyClient(`/products/${editingId}`, { method: "PUT", body: JSON.stringify(body) });
        setMessage("บันทึกสินค้าแล้ว");
      } else {
        await proxyClient("/products", { method: "POST", body: JSON.stringify(body) });
        setMessage("เพิ่มสินค้าใหม่แล้ว");
      }
      setDialogOpen(false);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "บันทึกสินค้าไม่สำเร็จ");
    }
  }

  function updateQuery(next: {
    search?: string;
    category_id?: string;
    sales_channel?: string;
    page?: number;
    page_size?: number;
  }) {
    const params = new URLSearchParams();
    const search = next.search ?? defaultSearch;
    const categoryId = next.category_id ?? defaultCategoryId;
    const salesChannel = next.sales_channel ?? defaultSalesChannel;
    const pageSize = next.page_size ?? pagination.page_size;
    // Any filter change resets to page 1 — staying on page 7 of a narrower
    // result set just shows an empty table.
    const page = next.page ?? 1;
    if (search) params.set("search", search);
    if (categoryId) params.set("category_id", categoryId);
    if (salesChannel) params.set("sales_channel", salesChannel);
    if (page > 1) params.set("page", String(page));
    params.set("page_size", String(pageSize));
    router.push(`/product-catalog?${params.toString()}`);
  }

  return (
    <div className="space-y-4">
      {message ? <p className="rounded-2xl border bg-card px-4 py-3 text-sm shadow-card">{message}</p> : null}
      <SectionCard
        description="สินค้าทั้งหมดในระบบ — ต้นทาง ราคา หมวดหมู่ และช่องทางขาย ในที่เดียว ทุกสาขาดึงข้อมูลจากรายการนี้"
        title="รายการสินค้า"
      >
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <Field className="w-64" label="ค้นหา">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-9"
                defaultValue={defaultSearch}
                onKeyDown={(event) => {
                  if (event.key === "Enter") updateQuery({ search: (event.target as HTMLInputElement).value });
                }}
                placeholder="ชื่อ, SKU, บาร์โค้ด"
              />
            </div>
          </Field>
          <Field className="w-48" label="หมวดสินค้า">
            <Select aria-label="กรองตามหมวดสินค้า" onChange={(event) => updateQuery({ category_id: event.target.value })} value={defaultCategoryId}>
              <option value="">ทุกหมวด</option>
              {categories.map((category) => (
                <option key={String(category.id)} value={String(category.id)}>{String(category.name)}</option>
              ))}
            </Select>
          </Field>
          <Field className="w-48" label="ช่องทางขาย">
            <Select aria-label="กรองตามช่องทางขาย" onChange={(event) => updateQuery({ sales_channel: event.target.value })} value={defaultSalesChannel}>
              <option value="">ทุกช่องทาง</option>
              <option value="in_store">หน้าร้านเท่านั้น</option>
              <option value="online">ออนไลน์เท่านั้น</option>
              <option value="both">หน้าร้าน + ออนไลน์</option>
            </Select>
          </Field>
          <Button className="ml-auto" onClick={openCreate} type="button">
            <Plus className="h-4 w-4" />
            เพิ่มสินค้าใหม่
          </Button>
        </div>

        <DataTable
          columns={[
            { key: "sku", label: "SKU" },
            { key: "name", label: "ชื่อสินค้า" },
            { key: "category_name", label: "หมวดสินค้า" },
            { key: "base_selling_price", label: "ราคาขายตั้งต้น", type: "currency" },
            { key: "sales_channel_label", label: "ช่องทางขาย" }
          ]}
          emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
          rowActions={(row) => (
            <Button onClick={() => openEdit(row)} type="button" variant="secondary">
              แก้ไข
            </Button>
          )}
          rows={rows}
        />

        <Pagination
          className="mt-4"
          onPageChange={(page) => updateQuery({ page })}
          onPageSizeChange={(page_size) => updateQuery({ page_size })}
          page={pagination.page}
          pageSize={pagination.page_size}
          total={pagination.total}
          totalPages={pagination.total_pages}
        />
      </SectionCard>

      <Dialog onOpenChange={setDialogOpen} open={dialogOpen}>
        <DialogContent className="max-w-3xl">
          <DialogHeader description="ข้อมูลนี้ใช้ร่วมกันทุกสาขา ราคาต่อสาขาแก้ไขแยกได้ที่หน้าสต๊อก" title={editingId ? "แก้ไขสินค้า" : "เพิ่มสินค้าใหม่"} />
          <form className="grid gap-3 md:grid-cols-2" onSubmit={submit}>
            <Field error={errors.errors.sku} label="SKU">
              <Input onChange={(event) => { setEditing((current) => ({ ...current, sku: event.target.value })); errors.clearError("sku"); }} value={String(editing.sku || "")} />
            </Field>
            <Field label="บาร์โค้ด" hint="ไม่บังคับ">
              <Input onChange={(event) => setEditing((current) => ({ ...current, barcode: event.target.value }))} value={String(editing.barcode || "")} />
            </Field>
            <Field className="md:col-span-2" error={errors.errors.name} label="ชื่อสินค้า">
              <Input onChange={(event) => { setEditing((current) => ({ ...current, name: event.target.value })); errors.clearError("name"); }} value={String(editing.name || "")} />
            </Field>
            <Field className="md:col-span-2" label="รายละเอียด" hint="ไม่บังคับ">
              <Textarea onChange={(event) => setEditing((current) => ({ ...current, description: event.target.value }))} value={String(editing.description || "")} />
            </Field>
            <Field label="หมวดสินค้า">
              <Select aria-label="หมวดสินค้า" onChange={(event) => setEditing((current) => ({ ...current, category_id: event.target.value }))} value={String(editing.category_id || "")}>
                <option value="">ไม่ระบุหมวด</option>
                {categories.map((category) => (
                  <option key={String(category.id)} value={String(category.id)}>{String(category.name)}</option>
                ))}
              </Select>
            </Field>
            <Field error={errors.errors.unit_name} label="หน่วยนับ">
              <Input onChange={(event) => { setEditing((current) => ({ ...current, unit_name: event.target.value })); errors.clearError("unit_name"); }} value={String(editing.unit_name || "")} />
            </Field>
            <Field label="ราคาทุน">
              <Input onChange={(event) => setEditing((current) => ({ ...current, cost_price: event.target.value }))} type="number" value={String(editing.cost_price ?? 0)} />
            </Field>
            <Field label="ราคาขายตั้งต้น (คลังหลัก)">
              <Input onChange={(event) => setEditing((current) => ({ ...current, base_selling_price: event.target.value }))} type="number" value={String(editing.base_selling_price ?? 0)} />
            </Field>
            <Field label="ส่วนลดสูงสุด">
              <Input onChange={(event) => setEditing((current) => ({ ...current, max_discount_amount: event.target.value }))} type="number" value={String(editing.max_discount_amount ?? 0)} />
            </Field>
            <Field label="ช่องทางขาย">
              <Select aria-label="ช่องทางขาย" onChange={(event) => setEditing((current) => ({ ...current, sales_channel: event.target.value }))} value={String(editing.sales_channel || "in_store")}>
                <option value="in_store">หน้าร้านเท่านั้น</option>
                <option value="online">ออนไลน์เท่านั้น</option>
                <option value="both">หน้าร้าน + ออนไลน์</option>
              </Select>
            </Field>
            {/* Full-height rows that line up with the inputs beside them — these
                used to collapse to the checkbox's own height and read as thin,
                cramped pills. */}
            <label className="flex min-h-12 cursor-pointer items-center gap-3 rounded-xl border px-4 py-3 text-sm font-medium transition hover:bg-muted">
              <Checkbox checked={Boolean(editing.tax_exempt)} onChange={(event) => setEditing((current) => ({ ...current, tax_exempt: event.target.checked }))} />
              ยกเว้นภาษีมูลค่าเพิ่ม
            </label>
            <label className="flex min-h-12 cursor-pointer items-center gap-3 rounded-xl border px-4 py-3 text-sm font-medium transition hover:bg-muted">
              <Checkbox checked={Boolean(editing.active)} onChange={(event) => setEditing((current) => ({ ...current, active: event.target.checked }))} />
              เปิดใช้งาน
            </label>
            {/* The อย. flag + registration number moved out with the FDA (อย.)
                feature into PharmaPOS Pro; the product still carries the fields
                (kept on save) so nothing is lost when that feature returns. */}
            <div className="md:col-span-2 flex justify-end gap-2">
              <Button onClick={() => setDialogOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
              <Button type="submit">{editingId ? "บันทึก" : "เพิ่มสินค้า"}</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
