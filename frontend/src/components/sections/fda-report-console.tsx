"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Printer, Search } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Button, Checkbox, EmptyState, Input, Pagination, Select, usePagedRows } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

// date_from/date_to are plain YYYY-MM-DD (a report period, not an instant),
// so this formats the date only — dateTime()'s time-of-day would be
// meaningless noise on the printed document's header.
function dateOnly(value: string) {
  if (!value) return "-";
  const [year, month, day] = value.split("-").map(Number);
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeZone: "Asia/Bangkok" }).format(new Date(Date.UTC(year, (month || 1) - 1, day || 1, 12)));
}

// Part B, Rule 1 — the อย. submission generator: filter → pick products →
// print. Screen and print share the same fetched summary; there's no
// separate "generate" call, mirroring how the PO print view (D4) reads off
// already-fetched detail data.
export function FdaReportConsole({
  summary,
  branches,
  categories,
  defaultDateFrom,
  defaultDateTo,
  defaultBranchId,
  defaultCategoryId
}: {
  summary: {
    company_name: string;
    company_address: string;
    company_tax_id: string;
    fda_license_no: string;
    date_from: string;
    date_to: string;
    items: Option[];
  };
  branches: Option[];
  categories: Option[];
  defaultDateFrom: string;
  defaultDateTo: string;
  defaultBranchId: string;
  defaultCategoryId: string;
}) {
  const router = useRouter();
  const [selected, setSelected] = useState<Set<string>>(new Set(summary.items.map((item) => String(item.product_id))));
  // Date/branch/category filter the query server-side; this one narrows what
  // came back, so you can find a product without re-running the report.
  const [search, setSearch] = useState("");
  const visibleItems = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase("th");
    if (!keyword) return summary.items;
    return summary.items.filter((item) =>
      [item.sku, item.name, item.fda_registration_no].some((value) =>
        String(value || "").toLocaleLowerCase("th").includes(keyword)
      )
    );
  }, [search, summary.items]);
  const { pageRows, pager, resetPage } = usePagedRows(visibleItems);

  const selectedItems = useMemo(
    () => summary.items.filter((item) => selected.has(String(item.product_id))),
    [summary.items, selected]
  );

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function updateQuery(next: Partial<{ date_from: string; date_to: string; branch_id: string; category_id: string }>) {
    const params = new URLSearchParams();
    const merged = {
      date_from: next.date_from ?? defaultDateFrom,
      date_to: next.date_to ?? defaultDateTo,
      branch_id: next.branch_id ?? defaultBranchId,
      category_id: next.category_id ?? defaultCategoryId
    };
    for (const [key, value] of Object.entries(merged)) {
      if (value) params.set(key, value);
    }
    router.push(`/fda-reports${params.size ? `?${params.toString()}` : ""}`);
  }

  return (
    <div className="space-y-6">
      <div className="print:hidden">
        <SectionCard description="สินค้าที่ตั้งค่า &quot;ต้องรายงานต่อ อย.&quot; ไว้ในหน้ารายการสินค้าจะแสดงที่นี่โดยอัตโนมัติ ปรับช่วงวันที่แล้วเลือกเฉพาะรายการที่ต้องการก่อนพิมพ์" title="เลือกสินค้าและช่วงเวลา">
          <div className="mb-4 flex flex-wrap items-end gap-3">
            <Field label="วันที่เริ่มต้น">
              <Input defaultValue={defaultDateFrom} onChange={(event) => updateQuery({ date_from: event.target.value })} type="date" />
            </Field>
            <Field label="วันที่สิ้นสุด">
              <Input defaultValue={defaultDateTo} onChange={(event) => updateQuery({ date_to: event.target.value })} type="date" />
            </Field>
            <Field className="w-52" label="สาขา">
              <Select aria-label="กรองตามสาขา" onChange={(event) => updateQuery({ branch_id: event.target.value })} value={defaultBranchId}>
                <option value="">ทุกสาขา</option>
                {branches.map((branch) => (
                  <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
                ))}
              </Select>
            </Field>
            <Field className="w-52" label="หมวดสินค้า">
              <Select aria-label="กรองตามหมวดสินค้า" onChange={(event) => updateQuery({ category_id: event.target.value })} value={defaultCategoryId}>
                <option value="">ทุกหมวด</option>
                {categories.map((category) => (
                  <option key={String(category.id)} value={String(category.id)}>{String(category.name)}</option>
                ))}
              </Select>
            </Field>
            <Field className="w-64" label="ค้นหาสินค้า">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                <Input
                  aria-label="ค้นหาสินค้าในรายงาน อย."
                  className="pl-9"
                  onChange={(event) => { setSearch(event.target.value); resetPage(); }}
                  placeholder="SKU, ชื่อสินค้า, เลขทะเบียน"
                  value={search}
                />
              </div>
            </Field>
            <Button className="ml-auto print:hidden" disabled={selectedItems.length === 0} onClick={() => window.print()} type="button">
              <Printer className="h-4 w-4" />
              พิมพ์เอกสารนำส่ง อย.
            </Button>
          </div>

          {summary.items.length === 0 ? (
            <EmptyState description="ตั้งค่า &quot;ต้องรายงานต่อ อย.&quot; ให้สินค้าที่หน้ารายการสินค้าก่อน" />
          ) : (
            <div className="overflow-x-auto rounded-2xl border">
              <table className="w-full text-sm">
                <thead className="bg-muted text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="w-10 px-3 py-2"></th>
                    <th className="px-3 py-2 text-left">SKU</th>
                    <th className="px-3 py-2 text-left">ชื่อสินค้า</th>
                    <th className="px-3 py-2 text-left">เลขทะเบียน อย.</th>
                    <th className="px-3 py-2 text-right">จำนวนขาย</th>
                    <th className="px-3 py-2 text-right">จำนวนรับเข้า</th>
                  </tr>
                </thead>
                <tbody>
                  {pageRows.map((item) => {
                    const id = String(item.product_id);
                    return (
                      <tr className="border-t" key={id}>
                        <td className="px-3 py-2"><Checkbox checked={selected.has(id)} onChange={() => toggle(id)} /></td>
                        <td className="px-3 py-2">{String(item.sku)}</td>
                        <td className="px-3 py-2">{String(item.name)}</td>
                        <td className="px-3 py-2">{String(item.fda_registration_no) || <span className="text-muted-foreground">ยังไม่ระบุ</span>}</td>
                        <td className="px-3 py-2 text-right tabular-nums">{Number(item.quantity_sold).toLocaleString("th-TH")}</td>
                        <td className="px-3 py-2 text-right tabular-nums">{Number(item.quantity_received).toLocaleString("th-TH")}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          {summary.items.length ? <Pagination className="mt-4 print:hidden" {...pager} /> : null}
        </SectionCard>
      </div>

      <FdaSubmissionDocument dateFrom={summary.date_from} dateTo={summary.date_to} items={selectedItems} meta={summary} />
    </div>
  );
}

function FdaSubmissionDocument({
  meta,
  items,
  dateFrom,
  dateTo
}: {
  meta: { company_name: string; company_address: string; company_tax_id: string; fda_license_no: string };
  items: Option[];
  dateFrom: string;
  dateTo: string;
}) {
  return (
    <div className="hidden print:block">
      <div className="mb-6 text-center">
        <p className="text-lg font-bold">เอกสารนำส่งสำนักงานคณะกรรมการอาหารและยา (อย.)</p>
        <p className="text-sm">รายงานการเคลื่อนไหวสินค้าที่ต้องรายงานต่อ อย.</p>
      </div>
      <div className="mb-4 grid grid-cols-2 gap-2 text-sm">
        <p><strong>สถานประกอบการ:</strong> {meta.company_name}</p>
        <p><strong>เลขที่ใบอนุญาต อย.:</strong> {meta.fda_license_no || "-"}</p>
        <p><strong>ที่อยู่:</strong> {meta.company_address}</p>
        <p><strong>เลขประจำตัวผู้เสียภาษี:</strong> {meta.company_tax_id}</p>
        <p className="col-span-2"><strong>ช่วงเวลารายงาน:</strong> {dateOnly(dateFrom)} ถึง {dateOnly(dateTo)}</p>
      </div>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b-2 border-black">
            <th className="p-2 text-left">ลำดับ</th>
            <th className="p-2 text-left">SKU</th>
            <th className="p-2 text-left">ชื่อสินค้า</th>
            <th className="p-2 text-left">เลขทะเบียน อย.</th>
            <th className="p-2 text-right">จำนวนขาย</th>
            <th className="p-2 text-right">จำนวนรับเข้า</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr className="border-b" key={String(item.product_id)}>
              <td className="p-2">{index + 1}</td>
              <td className="p-2">{String(item.sku)}</td>
              <td className="p-2">{String(item.name)}</td>
              <td className="p-2">{String(item.fda_registration_no) || "-"}</td>
              <td className="p-2 text-right tabular-nums">{Number(item.quantity_sold).toLocaleString("th-TH")}</td>
              <td className="p-2 text-right tabular-nums">{Number(item.quantity_received).toLocaleString("th-TH")}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="mt-16 grid grid-cols-2 gap-8 text-sm">
        <div className="text-center">
          <p className="mb-12">ลงชื่อ .......................................................</p>
          <p>ผู้จัดทำรายงาน</p>
        </div>
        <div className="text-center">
          <p className="mb-12">ลงชื่อ .......................................................</p>
          <p>ผู้มีอำนาจลงนาม</p>
        </div>
      </div>
    </div>
  );
}
