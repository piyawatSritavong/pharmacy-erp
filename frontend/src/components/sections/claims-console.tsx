"use client";

import { startTransition, useMemo, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { Ban, Search, Send, Undo2 } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { ProductSearchPicker } from "@/components/sections/product-search-picker";
import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  Input,
  Pagination,
  Select,
  Textarea,
  usePagedRows
} from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

const STATUS_LABEL: Record<string, string> = {
  pending_claim: "รอส่งเคลม",
  sent_to_supplier: "ส่งเคลมแล้ว",
  resolved_case_a: "ปิดเคลม — รับรุ่นเดิม",
  resolved_case_b: "ปิดเคลม — รุ่นทดแทน",
  rejected: "คู่ค้าปฏิเสธ"
};

const STATUS_TONE: Record<string, string> = {
  pending_claim: "bg-warning-50 text-warning-800",
  sent_to_supplier: "bg-info-50 text-info-800",
  resolved_case_a: "bg-success-50 text-success-800",
  resolved_case_b: "bg-success-50 text-success-800",
  rejected: "bg-error-50 text-error"
};

// Part B, Rule 4 — the back-office half: work the claim queue POS return
// initiations land in, send batches to the supplier, then close each one
// out as Case A (same model restocked) or Case B (different model —
// original stays written off, the replacement is received in separately).
export function ClaimsConsole({ initialItems, suppliers }: { initialItems: Option[]; suppliers: Option[] }) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [search, setSearch] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [sendDialog, setSendDialog] = useState<Option | null>(null);
  const [supplierId, setSupplierId] = useState("");
  // One resolve dialog per claim: the user explicitly picks Case A (same
  // model back → restock) or Case B (discontinued → sell out original +
  // purchase in the replacement) before anything happens — the two paths and
  // their stock consequences are never implicit (business-flow.md).
  const [resolveDialog, setResolveDialog] = useState<Option | null>(null);
  const [resolveCase, setResolveCase] = useState<"" | "a" | "b">("");
  const [replacementProductId, setReplacementProductId] = useState("");
  const [rejectDialog, setRejectDialog] = useState<Option | null>(null);
  const [rejectNote, setRejectNote] = useState("");

  // Branch list comes from the claims themselves — the page passes no branch
  // catalogue, and a filter that offers branches with no claims is noise.
  const branchNames = useMemo(
    () => Array.from(new Set(initialItems.map((item) => String(item.branch_name || "")).filter(Boolean))).sort(),
    [initialItems]
  );

  const groups = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase("th");
    const buckets: Record<string, Option[]> = { pending_claim: [], sent_to_supplier: [], closed: [] };
    for (const item of initialItems) {
      if (branchFilter && String(item.branch_name) !== branchFilter) continue;
      if (
        keyword &&
        ![item.product_name, item.sku, item.reason, item.supplier_name].some((value) =>
          String(value || "").toLocaleLowerCase("th").includes(keyword)
        )
      ) {
        continue;
      }
      const status = String(item.status);
      if (status === "pending_claim") buckets.pending_claim.push(item);
      else if (status === "sent_to_supplier") buckets.sent_to_supplier.push(item);
      else buckets.closed.push(item);
    }
    return buckets;
  }, [branchFilter, initialItems, search]);

  // Each queue pages on its own — they are three separate work lists, and a
  // shared pager would hide a waiting claim behind an archive page.
  const pending = usePagedRows(groups.pending_claim);
  const waiting = usePagedRows(groups.sent_to_supplier);
  const closed = usePagedRows(groups.closed);

  function onFilterChange(apply: () => void) {
    apply();
    pending.resetPage();
    waiting.resetPage();
    closed.resetPage();
  }

  async function run(path: string, body?: Record<string, unknown>) {
    try {
      const result = await proxyClient<{ message: string }>(path, { method: "POST", body: JSON.stringify(body || {}) });
      setMessage(result.message);
      setSendDialog(null);
      setResolveDialog(null);
      setRejectDialog(null);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ดำเนินการไม่สำเร็จ");
    }
  }

  function submitResolve() {
    if (!resolveDialog || !resolveCase) return;
    if (resolveCase === "a") {
      void run(`/product-returns/${String(resolveDialog.id)}/resolve-case-a`);
    } else {
      void run(`/product-returns/${String(resolveDialog.id)}/resolve-case-b`, { replacement_product_id: replacementProductId });
    }
  }

  function ReturnRow({ item, actions }: { item: Option; actions?: ReactNode }) {
    return (
      <div className="flex flex-wrap items-center gap-3 rounded-2xl border bg-card p-4" key={String(item.id)}>
        <div className="min-w-[220px] flex-1">
          <p className="font-semibold">{String(item.product_name)} <span className="font-normal text-muted-foreground">({String(item.sku)})</span></p>
          <p className="text-sm text-muted-foreground">{String(item.branch_name)} · จำนวน {String(item.quantity)} · เหตุผล: {String(item.reason)}</p>
          {item.supplier_name ? <p className="text-sm text-muted-foreground">คู่ค้า: {String(item.supplier_name)}</p> : null}
          {item.replacement_product_name ? <p className="text-sm text-muted-foreground">รุ่นทดแทน: {String(item.replacement_product_name)}</p> : null}
        </div>
        <span className={`rounded-full px-3 py-1 text-xs font-semibold ${STATUS_TONE[String(item.status)] || "bg-muted"}`}>
          {STATUS_LABEL[String(item.status)] || String(item.status)}
        </span>
        {actions}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {message ? <p className="rounded-2xl border bg-card px-4 py-3 text-sm shadow-card">{message}</p> : null}

      <SectionCard description="ค้นหาด้วยชื่อสินค้า SKU เหตุผล หรือคู่ค้า และกรองตามสาขา" title="ค้นหาเคลม">
        <div className="flex flex-wrap items-end gap-3">
          <Field className="w-72" label="ค้นหา">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาเคลม"
                className="pl-9"
                onChange={(event) => onFilterChange(() => setSearch(event.target.value))}
                placeholder="ชื่อสินค้า, SKU, เหตุผล, คู่ค้า"
                value={search}
              />
            </div>
          </Field>
          <Field className="w-56" label="สาขา">
            <Select
              aria-label="กรองเคลมตามสาขา"
              onChange={(event) => onFilterChange(() => setBranchFilter(event.target.value))}
              value={branchFilter}
            >
              <option value="">ทุกสาขา</option>
              {branchNames.map((name) => <option key={name} value={name}>{name}</option>)}
            </Select>
          </Field>
        </div>
      </SectionCard>

      <SectionCard description="รายการที่ POS ออกสินค้าทดแทนให้ลูกค้าแล้ว รอส่งเคลมให้คู่ค้า" title="รอส่งเคลม">
        {groups.pending_claim.length === 0 ? <EmptyState /> : (
          <div className="space-y-3">
            {pending.pageRows.map((item) => (
              <ReturnRow
                actions={
                  <Button onClick={() => { setSendDialog(item); setSupplierId(""); }} type="button">
                    <Send className="h-4 w-4" />
                    ส่งเคลม
                  </Button>
                }
                item={item}
                key={String(item.id)}
              />
            ))}
          </div>
        )}
        <Pagination className="mt-4" {...pending.pager} />
      </SectionCard>

      <SectionCard description="ส่งเคลมให้คู่ค้าแล้ว รอผลตอบกลับ" title="ระหว่างรอคู่ค้า">
        {groups.sent_to_supplier.length === 0 ? <EmptyState /> : (
          <div className="space-y-3">
            {waiting.pageRows.map((item) => (
              <ReturnRow
                actions={
                  <div className="flex flex-wrap gap-2">
                    <Button onClick={() => { setResolveDialog(item); setResolveCase(""); setReplacementProductId(""); }} type="button">
                      <Undo2 className="h-4 w-4" />
                      ปิดเคลม
                    </Button>
                    <Button onClick={() => { setRejectDialog(item); setRejectNote(""); }} type="button" variant="destructive">
                      <Ban className="h-4 w-4" />
                      ปฏิเสธ
                    </Button>
                  </div>
                }
                item={item}
                key={String(item.id)}
              />
            ))}
          </div>
        )}
        <Pagination className="mt-4" {...waiting.pager} />
      </SectionCard>

      <SectionCard description="เคลมที่ปิดแล้ว — รับรุ่นเดิม, รับรุ่นทดแทน, หรือถูกปฏิเสธ" title="ปิดเคลมแล้ว">
        {groups.closed.length === 0 ? <EmptyState /> : (
          <div className="space-y-3">
            {closed.pageRows.map((item) => <ReturnRow item={item} key={String(item.id)} />)}
          </div>
        )}
        <Pagination className="mt-4" {...closed.pager} />
      </SectionCard>

      <Dialog onOpenChange={(open) => !open && setSendDialog(null)} open={Boolean(sendDialog)}>
        <DialogContent>
          <DialogHeader description="เลือกคู่ค้าที่จะส่งเคลมนี้ไปให้" title="ส่งเคลมให้คู่ค้า" />
          <div className="space-y-4">
            <Field label="คู่ค้า">
              <Select aria-label="เลือกคู่ค้า" onChange={(event) => setSupplierId(event.target.value)} value={supplierId}>
                <option value="">เลือกคู่ค้า</option>
                {suppliers.map((supplier) => (
                  <option key={String(supplier.id)} value={String(supplier.id)}>{String(supplier.legal_name)}</option>
                ))}
              </Select>
            </Field>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setSendDialog(null)} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={!supplierId} onClick={() => void run(`/product-returns/${String(sendDialog?.id)}/send-to-supplier`, { supplier_id: supplierId })} type="button">ส่งเคลม</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => !open && setResolveDialog(null)} open={Boolean(resolveDialog)}>
        <DialogContent>
          <DialogHeader
            description={`${String(resolveDialog?.product_name || "")} · จำนวน ${String(resolveDialog?.quantity || "")} — เลือกว่าคู่ค้าตอบกลับแบบใด ระบบจะจัดการสต๊อกให้ตรงตามกรณีนั้น`}
            title="ปิดเคลม — คู่ค้าส่งสินค้ากลับมาแบบไหน?"
          />
          <div className="space-y-4">
            {/* The two supplier-return cases, side by side and mutually
                exclusive — each states its stock consequence up front so the
                choice is unambiguous before confirming. */}
            <div className="grid gap-3">
              <label
                className={`flex cursor-pointer items-start gap-3 rounded-2xl border p-4 transition-colors ${resolveCase === "a" ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
              >
                <input
                  checked={resolveCase === "a"}
                  className="mt-1"
                  name="resolve-case"
                  onChange={() => setResolveCase("a")}
                  type="radio"
                />
                <span>
                  <strong className="block">Case A — ได้สินค้ารุ่นเดิมกลับมา</strong>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    คู่ค้าส่งสินค้ารุ่นเดิม (ชิ้นใหม่) กลับมา → ระบบรับกลับเข้าสต๊อกโดยตรง ไม่มีการซื้อใหม่
                  </span>
                </span>
              </label>
              <label
                className={`flex cursor-pointer items-start gap-3 rounded-2xl border p-4 transition-colors ${resolveCase === "b" ? "border-primary bg-primary/5" : "hover:bg-muted"}`}
              >
                <input
                  checked={resolveCase === "b"}
                  className="mt-1"
                  name="resolve-case"
                  onChange={() => setResolveCase("b")}
                  type="radio"
                />
                <span>
                  <strong className="block">Case B — รุ่นเดิมเลิกผลิต ได้รุ่นอื่นมาแทน</strong>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    คู่ค้าส่งสินค้ารุ่นใหม่/รุ่นอื่นราคาเดียวกันมาแทน → ระบบบันทึกขายออกรุ่นเดิม (ไม่คืนสต๊อก) และรับซื้อรุ่นทดแทนเข้าสต๊อกแยกต่างหาก
                  </span>
                </span>
              </label>
            </div>
            {resolveCase === "b" ? (
              <Field label="สินค้ารุ่นทดแทนที่ได้รับ">
                <ProductSearchPicker ariaLabel="เลือกสินค้ารุ่นทดแทน" onChange={(value) => setReplacementProductId(value)} value={replacementProductId} />
              </Field>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button onClick={() => setResolveDialog(null)} type="button" variant="secondary">ยกเลิก</Button>
              <Button
                disabled={!resolveCase || (resolveCase === "b" && !replacementProductId)}
                onClick={submitResolve}
                type="button"
              >
                {resolveCase === "b" ? "ยืนยัน — รับรุ่นทดแทนเข้าสต๊อก" : resolveCase === "a" ? "ยืนยัน — รับรุ่นเดิมคืนสต๊อก" : "เลือกกรณีก่อนยืนยัน"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => !open && setRejectDialog(null)} open={Boolean(rejectDialog)}>
        <DialogContent>
          <DialogHeader description="บันทึกเหตุผลที่คู่ค้าปฏิเสธเคลมนี้" title="ปฏิเสธเคลม" />
          <div className="space-y-4">
            <Field label="หมายเหตุ" hint="ไม่บังคับ">
              <Textarea onChange={(event) => setRejectNote(event.target.value)} value={rejectNote} />
            </Field>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setRejectDialog(null)} type="button" variant="secondary">ยกเลิก</Button>
              <Button onClick={() => void run(`/product-returns/${String(rejectDialog?.id)}/reject`, { note: rejectNote })} type="button" variant="destructive">ยืนยันปฏิเสธ</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
