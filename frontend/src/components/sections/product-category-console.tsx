"use client";

import { FormEvent, startTransition, useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Search, Tags, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";

import { Field } from "@/components/ui/field";
import {
  Button,
  CheckboxField,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  Input,
  Notice,
  Pagination,
  Select,
  usePagedRows
} from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Category = Record<string, unknown>;

const blankCategory: Category = {
  name: "",
  color: "#D71920",
  active: true,
};

export function ProductCategoryConsole({ initialItems }: { initialItems: Category[] }) {
  const router = useRouter();
  const [items, setItems] = useState(initialItems);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Category>(blankCategory);
  const [message, setMessage] = useState("");

  useEffect(() => setItems(initialItems), [initialItems]);

  const visible = useMemo(() => {
    const keyword = query.trim().toLocaleLowerCase("th-TH");
    return items.filter((item) => {
      if (keyword && !String(item.name || "").toLocaleLowerCase("th-TH").includes(keyword)) return false;
      if (statusFilter === "active" && !item.active) return false;
      if (statusFilter === "inactive" && item.active) return false;
      return true;
    });
  }, [items, query, statusFilter]);
  const { pageRows, pager, resetPage } = usePagedRows(visible);

  function openCreate() {
    setEditing({ ...blankCategory });
    setMessage("");
    setDialogOpen(true);
  }

  function openEdit(item: Category) {
    setEditing({ ...item });
    setMessage("");
    setDialogOpen(true);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      const id = String(editing.id || "");
      await proxyClient(id ? `/product-categories/${id}` : "/product-categories", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify({
          name: String(editing.name || "").trim(),
          color: String(editing.color || "#D71920"),
          active: Boolean(editing.active),
        }),
      });
      setDialogOpen(false);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "บันทึกหมวดสินค้าไม่สำเร็จ");
    }
  }

  async function deleteCategory(item: Category) {
    if (
      !window.confirm(
        `ลบหมวด "${String(item.name)}" หรือไม่? สินค้าในหมวดนี้จะถูกย้ายไปยัง "ยังไม่จัดหมวด" โดยอัตโนมัติ ไม่มีสินค้าใดถูกลบ`,
      )
    )
      return;
    try {
      const response = await proxyClient<{ message: string }>(`/product-categories/${String(item.id)}`, {
        method: "DELETE",
      });
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ลบหมวดสินค้าไม่สำเร็จ");
    }
  }

  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-3xl border bg-white shadow-card">
        <div className="flex flex-col gap-3 border-b bg-surface-warm p-3 sm:p-5 md:flex-row md:items-center md:justify-between">
          <div className="flex flex-1 flex-wrap items-center gap-3">
            <div className="relative min-w-0 flex-1 md:max-w-md">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาหมวดสินค้า"
                className="pl-9"
                onChange={(event) => { setQuery(event.target.value); resetPage(); }}
                placeholder="ค้นหาชื่อหมวดสินค้า"
                value={query}
              />
            </div>
            <Select
              aria-label="กรองตามสถานะหมวดสินค้า"
              className="w-44 shrink-0"
              onChange={(event) => { setStatusFilter(event.target.value); resetPage(); }}
              value={statusFilter}
            >
              <option value="">ทุกสถานะ</option>
              <option value="active">เปิดใช้งาน</option>
              <option value="inactive">เก็บแล้ว</option>
            </Select>
          </div>
          <Button onClick={openCreate} type="button">
            <Plus className="h-4 w-4" />
            เพิ่มหมวดสินค้า
          </Button>
        </div>

        {message ? <Notice className="rounded-none border-b" tone="error">{message}</Notice> : null}

        <div className="overflow-x-auto p-3 sm:p-0">
          <table role="table" className="responsive-table mobile-card-table w-full min-w-[720px] text-sm">
            <thead role="rowgroup" className="bg-muted text-left">
              <tr role="row">
                <th role="columnheader" scope="col" className="p-3">หมวดสินค้า</th>
                <th role="columnheader" scope="col" className="p-3">สี</th>
                <th role="columnheader" scope="col" className="p-3 text-right">จำนวนสินค้า</th>
                <th role="columnheader" scope="col" className="p-3">สถานะ</th>
                <th role="columnheader" scope="col" className="p-3 text-right">จัดการ</th>
              </tr>
            </thead>
            <tbody role="rowgroup">
              {pageRows.map((item) => (
                <tr role="row" className="border-t" data-testid="category-row" key={String(item.id)}>
                  <td role="cell" data-label="หมวดสินค้า" data-primary="true" className="p-3 font-semibold">{String(item.name)}</td>
                  <td role="cell" data-label="สี" className="p-3">
                    <span className="inline-flex items-center gap-2">
                      <span
                        aria-hidden="true"
                        className="h-5 w-5 rounded-full border"
                        style={{ backgroundColor: String(item.color || "#D71920") }}
                      />
                      {String(item.color || "#D71920")}
                    </span>
                  </td>
                  <td role="cell" data-label="จำนวนสินค้า" className="p-3 text-right font-semibold">{Number(item.product_count || 0).toLocaleString("th-TH")}</td>
                  <td role="cell" data-label="สถานะ" className="p-3">
                    <span className={item.active ? "text-emerald-700" : "text-muted-foreground"}>
                      {item.active ? "เปิดใช้งาน" : "เก็บแล้ว"}
                    </span>
                  </td>
                  <td role="cell" data-label="จัดการ" data-actions="true" className="p-3">
                    <div className="flex justify-end gap-2">
                      <Button aria-label={`แก้ไข ${String(item.name)}`} className="h-8 px-3" onClick={() => openEdit(item)} type="button" variant="secondary">
                        <Pencil className="h-4 w-4" /> แก้ไข
                      </Button>
                      <Button aria-label={`ลบ ${String(item.name)}`} className="h-8 px-3" onClick={() => void deleteCategory(item)} type="button" variant="destructive">
                        <Trash2 className="h-4 w-4" /> ลบ
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {visible.length === 0 ? <EmptyState className="border-t p-12" description="ไม่พบหมวดสินค้าที่ค้นหา" icon={Tags} /> : null}

        <Pagination className="border-t p-4" {...pager} />
      </section>

      <Dialog onOpenChange={setDialogOpen} open={dialogOpen}>
        <DialogContent>
          <DialogHeader title={editing.id ? "แก้ไขหมวดสินค้า" : "เพิ่มหมวดสินค้า"} />
          <form className="space-y-4" onSubmit={submit}>
            <Field label="ชื่อหมวดสินค้า">
              <Input
                autoFocus
                onChange={(event) => setEditing((current) => ({ ...current, name: event.target.value }))}
                required
                value={String(editing.name || "")}
              />
            </Field>
            <Field label="สีประจำหมวด">
              <div className="flex items-center gap-3 rounded-xl border bg-white p-2">
                <input
                  aria-label="สีประจำหมวด"
                  className="h-10 w-14 cursor-pointer rounded border-0 bg-transparent"
                  onChange={(event) => setEditing((current) => ({ ...current, color: event.target.value }))}
                  type="color"
                  value={String(editing.color || "#D71920")}
                />
                <Input
                  onChange={(event) => setEditing((current) => ({ ...current, color: event.target.value }))}
                  pattern="^#[0-9A-Fa-f]{6}$"
                  value={String(editing.color || "#D71920")}
                />
              </div>
            </Field>
            <CheckboxField
              checked={Boolean(editing.active)}
              label="เปิดใช้งานหมวดนี้"
              onChange={(event) => setEditing((current) => ({ ...current, active: event.target.checked }))}
            />
            {message ? <p className="text-sm text-primary">{message}</p> : null}
            <div className="flex justify-end gap-2">
              <Button onClick={() => setDialogOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
              <Button type="submit">บันทึก</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
