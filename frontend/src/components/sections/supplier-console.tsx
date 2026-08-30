"use client";

import {
  FormEvent,
  startTransition,
  useEffect,
  useMemo,
  useState,
} from "react";
import { Building2, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";

import { Field } from "@/components/ui/field";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  Input,
  Notice,
  Pagination,
  Select,
  Textarea,
  usePagedRows,
} from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Item = Record<string, unknown>;

const blankSupplier: Item = {
  supplier_code: "",
  legal_name: "",
  tax_id: "",
  company_branch_type: "head_office",
  company_branch_number: "",
  address_line: "",
  subdistrict: "",
  district: "",
  province: "",
  postal_code: "",
  contact_name: "",
  phone: "",
  email: "",
  payment_terms_days: 0,
  notes: "",
  active: true,
};

export function SupplierConsole({ initialItems }: { initialItems: Item[] }) {
  const router = useRouter();
  const [items, setItems] = useState(initialItems);
  const [query, setQuery] = useState("");
  const [creditFilter, setCreditFilter] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Item>(blankSupplier);
  const [message, setMessage] = useState("");
  const [deleteState, setDeleteState] = useState<{
    id: string;
    label: string;
    confirmation: string;
    counts: Record<string, number>;
  } | null>(null);
  const [deleteText, setDeleteText] = useState("");
  useEffect(() => {
    setItems(initialItems);
  }, [initialItems]);
  const visible = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return items.filter((item) => {
      if (
        keyword &&
        ![item.legal_name, item.supplier_code, item.tax_id, item.phone].some((value) =>
          String(value || "").toLowerCase().includes(keyword),
        )
      ) {
        return false;
      }
      const terms = Number(item.payment_terms_days || 0);
      if (creditFilter === "cash" && terms !== 0) return false;
      if (creditFilter === "credit" && terms === 0) return false;
      return true;
    });
  }, [creditFilter, items, query]);
  const { pageRows, pager, resetPage } = usePagedRows(visible);

  function openCreate() {
    setEditing({ ...blankSupplier });
    setMessage("");
    setDialogOpen(true);
  }
  function openEdit(item: Item) {
    setEditing({ ...item });
    setMessage("");
    setDialogOpen(true);
  }
  function update(key: string, value: unknown) {
    setEditing((current) => ({ ...current, [key]: value }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    try {
      const id = String(editing.id || "");
      await proxyClient(id ? `/suppliers/${id}` : "/suppliers", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(editing),
      });
      setDialogOpen(false);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "บันทึกบริษัทคู่ค้าไม่สำเร็จ",
      );
    }
  }

  // Real delete, not an archive (business-flow.md Global Rules) — same
  // impact-preview + typed-confirmation pattern as branches/users/products.
  // Past POs keep their own supplier snapshot, so history is unaffected.
  async function requestDelete(item: Item) {
    try {
      const impact = await proxyClient<{
        confirmation: string;
        counts: Record<string, number>;
      }>(`/suppliers/${String(item.id)}/deletion-impact`);
      setDeleteText("");
      setDeleteState({
        id: String(item.id),
        label: String(item.legal_name),
        confirmation: impact.confirmation,
        counts: impact.counts,
      });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ตรวจสอบผลกระทบไม่สำเร็จ");
    }
  }

  async function confirmDelete() {
    if (!deleteState) return;
    try {
      const response = await proxyClient<{ message: string }>(`/suppliers/${deleteState.id}`, {
        method: "DELETE",
        body: JSON.stringify({ confirmation: deleteText }),
      });
      setDeleteState(null);
      setDeleteText("");
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ลบบริษัทคู่ค้าไม่สำเร็จ");
    }
  }


  return (
    <>
      <section className="overflow-hidden rounded-3xl border bg-white shadow-card">
        <div className="flex flex-col gap-3 border-b bg-surface-warm p-5 md:flex-row md:items-center md:justify-between">
          <div className="flex flex-1 flex-wrap items-center gap-3">
            <div className="relative min-w-0 flex-1 md:max-w-md">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาบริษัทคู่ค้า"
                className="pl-9"
                onChange={(event) => { setQuery(event.target.value); resetPage(); }}
                placeholder="ชื่อบริษัท รหัสคู่ค้า เลขผู้เสียภาษี หรือโทรศัพท์"
                value={query}
              />
            </div>
            <Select
              aria-label="กรองตามเงื่อนไขเครดิต"
              className="w-48 shrink-0"
              onChange={(event) => { setCreditFilter(event.target.value); resetPage(); }}
              value={creditFilter}
            >
              <option value="">ทุกเงื่อนไขชำระ</option>
              <option value="cash">เงินสด (0 วัน)</option>
              <option value="credit">มีเครดิต</option>
            </Select>
          </div>
          <Button onClick={openCreate} type="button">
            <Plus className="h-4 w-4" />
            เพิ่มบริษัทคู่ค้า
          </Button>
        </div>
        {message ? (
          <Notice className="rounded-none border-b" tone="error">
            {message}
          </Notice>
        ) : null}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[980px] text-sm">
            <thead className="bg-muted text-left">
              <tr>
                <th className="p-3">บริษัท</th>
                <th className="p-3">เลขผู้เสียภาษี</th>
                <th className="p-3">ผู้ติดต่อ</th>
                <th className="p-3">ที่อยู่</th>
                <th className="p-3">เครดิต</th>
                <th className="p-3">PO</th>
                <th className="p-3 text-right">จัดการ</th>
              </tr>
            </thead>
            <tbody>
              {pageRows.map((item) => (
                <tr className="border-t" key={String(item.id)}>
                  <td className="p-3">
                    <strong className="block">{String(item.legal_name)}</strong>
                    <span className="text-xs text-muted-foreground">
                      {String(item.supplier_code)}
                    </span>
                  </td>
                  <td className="p-3">{String(item.tax_id || "-")}</td>
                  <td className="p-3">
                    {String(item.contact_name || "-")}
                    <span className="block text-xs text-muted-foreground">
                      {String(item.phone || item.email || "")}
                    </span>
                  </td>
                  <td className="max-w-sm p-3">
                    {[
                      item.address_line,
                      item.subdistrict,
                      item.district,
                      item.province,
                      item.postal_code,
                    ]
                      .filter(Boolean)
                      .join(" ") || "-"}
                  </td>
                  <td className="p-3">
                    {Number(item.payment_terms_days || 0)} วัน
                  </td>
                  <td className="p-3">
                    {Number(item.purchase_order_count || 0)}
                  </td>
                  <td className="p-3">
                    <div className="flex justify-end gap-2">
                      <Button
                        className="h-9 px-3"
                        onClick={() => openEdit(item)}
                        type="button"
                        variant="secondary"
                      >
                        <Pencil className="h-4 w-4" />
                        แก้ไข
                      </Button>
                      <Button
                        className="h-9 px-3"
                        onClick={() => void requestDelete(item)}
                        type="button"
                        variant="destructive"
                      >
                        <Trash2 className="h-4 w-4" />
                        ลบ
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {visible.length === 0 ? <EmptyState className="p-12" description="ไม่พบบริษัทคู่ค้าตามเงื่อนไข" icon={Building2} /> : null}
        <Pagination className="border-t p-4" {...pager} />
      </section>

      <Dialog onOpenChange={(open) => !open && setDeleteState(null)} open={Boolean(deleteState)}>
        <DialogContent className="max-w-lg">
          <DialogHeader
            description="การลบนี้ถาวร — ใบสั่งซื้อเดิมยังคงเก็บชื่อและที่อยู่คู่ค้าไว้ในเอกสารตามเดิม แต่คู่ค้ารายนี้จะหายจากรายการเลือก"
            title={`ลบบริษัทคู่ค้า “${deleteState?.label || ""}”`}
          />
          <div className="space-y-4">
            <div className="rounded-2xl bg-muted p-4 text-sm">
              <p className="mb-2 font-semibold">ข้อมูลที่อ้างถึงคู่ค้ารายนี้</p>
              <ul className="space-y-1 text-muted-foreground">
                {Object.entries(deleteState?.counts || {}).map(([label, count]) => (
                  <li key={label}>
                    {label}: <strong className="text-foreground">{count.toLocaleString("th-TH")}</strong> รายการ
                  </li>
                ))}
              </ul>
            </div>
            <Field hint={`พิมพ์ “${deleteState?.confirmation || ""}” เพื่อยืนยัน`} label="ยืนยันการลบ">
              <Input
                aria-label="ข้อความยืนยันการลบคู่ค้า"
                onChange={(event) => setDeleteText(event.target.value)}
                value={deleteText}
              />
            </Field>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setDeleteState(null)} type="button" variant="secondary">
                ยกเลิก
              </Button>
              <Button
                disabled={deleteText !== deleteState?.confirmation}
                onClick={() => void confirmDelete()}
                type="button"
                variant="destructive"
              >
                ลบถาวร
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setDialogOpen} open={dialogOpen}>
        <DialogContent className="max-w-4xl">
          <DialogHeader
            description="ข้อมูลนี้ใช้ร่วมกันทุกสาขา และ PO จะเก็บ snapshot ไว้ตามวันที่ซื้อ"
            title={editing.id ? "แก้ไขบริษัทคู่ค้า" : "เพิ่มบริษัทคู่ค้า"}
          />
          <form className="grid gap-4 md:grid-cols-2" onSubmit={submit}>
            <Field label="รหัสคู่ค้า">
              <Input
                onChange={(e) => update("supplier_code", e.target.value)}
                placeholder="เว้นว่างเพื่อสร้างอัตโนมัติ"
                value={String(editing.supplier_code || "")}
              />
            </Field>
            <Field label="ชื่อบริษัท *">
              <Input
                required
                onChange={(e) => update("legal_name", e.target.value)}
                value={String(editing.legal_name || "")}
              />
            </Field>
            <Field label="เลขผู้เสียภาษี">
              <Input
                onChange={(e) => update("tax_id", e.target.value)}
                value={String(editing.tax_id || "")}
              />
            </Field>
            <div className="grid grid-cols-2 gap-2">
              <Field label="ประเภทสำนักงาน">
                <Select
                  onChange={(e) =>
                    update("company_branch_type", e.target.value)
                  }
                  value={String(editing.company_branch_type || "head_office")}
                >
                  <option value="head_office">สำนักงานใหญ่</option>
                  <option value="branch">สาขา</option>
                </Select>
              </Field>
              <Field label="เลขที่สาขา">
                <Input
                  onChange={(e) =>
                    update("company_branch_number", e.target.value)
                  }
                  value={String(editing.company_branch_number || "")}
                />
              </Field>
            </div>
            <Field className="md:col-span-2" label="ที่อยู่">
              <Input
                onChange={(e) => update("address_line", e.target.value)}
                value={String(editing.address_line || "")}
              />
            </Field>
            {[
              ["subdistrict", "แขวง/ตำบล"],
              ["district", "เขต/อำเภอ"],
              ["province", "จังหวัด"],
              ["postal_code", "รหัสไปรษณีย์"],
            ].map(([key, label]) => (
              <Field key={key} label={label}>
                <Input
                  onChange={(e) => update(key, e.target.value)}
                  value={String(editing[key] || "")}
                />
              </Field>
            ))}
            <Field label="ผู้ติดต่อ">
              <Input
                onChange={(e) => update("contact_name", e.target.value)}
                value={String(editing.contact_name || "")}
              />
            </Field>
            <Field label="โทรศัพท์">
              <Input
                onChange={(e) => update("phone", e.target.value)}
                value={String(editing.phone || "")}
              />
            </Field>
            <Field label="Email">
              <Input
                onChange={(e) => update("email", e.target.value)}
                type="email"
                value={String(editing.email || "")}
              />
            </Field>
            <Field label="เครดิต (วัน)">
              <Input
                min={0}
                onChange={(e) =>
                  update("payment_terms_days", Number(e.target.value))
                }
                type="number"
                value={Number(editing.payment_terms_days || 0)}
              />
            </Field>
            <Field className="md:col-span-2" label="หมายเหตุ">
              <Textarea
                onChange={(e) => update("notes", e.target.value)}
                value={String(editing.notes || "")}
              />
            </Field>
            {editing.id ? (
              <label className="flex items-center gap-2 text-sm">
                <input
                  checked={Boolean(editing.active)}
                  onChange={(e) => update("active", e.target.checked)}
                  type="checkbox"
                />
                เปิดใช้งาน
              </label>
            ) : null}
            {message ? (
              <p className="md:col-span-2 text-sm text-primary">{message}</p>
            ) : null}
            <div className="flex justify-end gap-2 md:col-span-2">
              <Button
                onClick={() => setDialogOpen(false)}
                type="button"
                variant="secondary"
              >
                ยกเลิก
              </Button>
              <Button type="submit">บันทึก</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
