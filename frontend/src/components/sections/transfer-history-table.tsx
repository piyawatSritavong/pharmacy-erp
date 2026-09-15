"use client";

import { Fragment, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Search } from "lucide-react";

import { SectionCard, statusLabel } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Input, Pagination, Select, usePagedRows } from "@/components/ui/primitives";
import { cn, dateTime } from "@/lib/utils";

type Option = Record<string, unknown>;
type TransferLine = {
  id?: unknown;
  product_name?: unknown;
  sku?: unknown;
  quantity?: unknown;
  received_quantity?: unknown;
  stock_bucket?: unknown;
  discrepancy_note?: unknown;
};

function text(value: unknown) {
  return value == null ? "" : String(value);
}
function count(value: unknown) {
  return Number(value || 0).toLocaleString("th-TH");
}

/**
 * ประวัติใบโอน — filterable, paged transfer history. A client component
 * because GET /transfers returns every transfer in one response and takes no
 * page or search params, so the narrowing happens here.
 *
 * Each row opens to show the lines that moved, the way a bill opens in the
 * month-end report — the header alone never said *what* was transferred.
 */
export function TransferHistoryTable({
  branches,
  transfers,
  canUseGhost = false
}: {
  branches: Option[];
  transfers: Option[];
  /** Superadmin only: shows the stock bucket and the discrepancy column. */
  canUseGhost?: boolean;
}) {
  const [search, setSearch] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

  const visible = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase("th");
    return transfers.filter((row) => {
      if (statusFilter && String(row.status) !== statusFilter) return false;
      // A branch matches on either leg — you look for "everything involving
      // MES", not "everything MES sent".
      if (
        branchFilter &&
        String(row.source_branch_name) !== branchFilter &&
        String(row.destination_branch_name) !== branchFilter
      ) {
        return false;
      }
      if (
        keyword &&
        ![row.transfer_code, row.source_branch_name, row.destination_branch_name].some((value) =>
          String(value || "").toLocaleLowerCase("th").includes(keyword)
        )
      ) {
        return false;
      }
      return true;
    });
  }, [branchFilter, search, statusFilter, transfers]);

  const { pageRows, pager, resetPage } = usePagedRows(visible);
  const columnCount = canUseGhost ? 8 : 7;

  return (
    <SectionCard description="สถานะการโอนสินค้าระหว่างทุกสาขา · กดแถวเพื่อดูรายการในใบโอน" title="ประวัติใบโอน">
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <Field className="w-full sm:w-64" label="ค้นหา">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              aria-label="ค้นหาใบโอน"
              className="pl-9"
              onChange={(event) => { setSearch(event.target.value); resetPage(); }}
              placeholder="รหัสโอน หรือชื่อสาขา"
              value={search}
            />
          </div>
        </Field>
        <Field className="w-52" label="สาขา (ต้นทางหรือปลายทาง)">
          <Select
            aria-label="กรองใบโอนตามสาขา"
            onChange={(event) => { setBranchFilter(event.target.value); resetPage(); }}
            value={branchFilter}
          >
            <option value="">ทุกสาขา</option>
            {branches.map((branch) => (
              <option key={String(branch.id)} value={String(branch.name)}>{String(branch.name)}</option>
            ))}
          </Select>
        </Field>
        <Field className="w-44" label="สถานะ">
          <Select
            aria-label="กรองใบโอนตามสถานะ"
            onChange={(event) => { setStatusFilter(event.target.value); resetPage(); }}
            value={statusFilter}
          >
            <option value="">ทุกสถานะ</option>
            <option value="requested">รอส่ง</option>
            <option value="in_transit">กำลังส่ง</option>
            <option value="completed">รับแล้ว</option>
          </Select>
        </Field>
      </div>

      <div className="overflow-x-auto rounded-xl border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-xs text-muted-foreground">
            <tr>
              <th className="w-10 px-3 py-3" />
              <th className="whitespace-nowrap px-3 py-3 text-left font-medium">รหัสโอน</th>
              <th className="whitespace-nowrap px-3 py-3 text-left font-medium">ต้นทาง</th>
              <th className="whitespace-nowrap px-3 py-3 text-left font-medium">ปลายทาง</th>
              <th className="px-3 py-3 text-left font-medium">จำนวนรายการ</th>
              <th className="whitespace-nowrap px-3 py-3 text-left font-medium">สถานะ</th>
              {canUseGhost ? <th className="px-3 py-3 text-left font-medium">พบส่วนต่าง</th> : null}
              <th className="whitespace-nowrap px-3 py-3 text-left font-medium">วันที่สร้าง</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {pageRows.length === 0 ? (
              <tr>
                <td className="px-3 py-10 text-center text-muted-foreground" colSpan={columnCount}>
                  ไม่มีใบโอน · ลองปรับคำค้นหาหรือตัวกรอง
                </td>
              </tr>
            ) : (
              pageRows.map((row) => {
                const id = text(row.id) || text(row.transfer_code);
                const open = expanded === id;
                const lines = (row.items as TransferLine[] | undefined) || [];
                return (
                  <Fragment key={id}>
                    <tr
                      className={cn("cursor-pointer transition hover:bg-muted/50", open && "bg-muted/40")}
                      onClick={() => setExpanded(open ? null : id)}
                    >
                      <td className="px-3 py-3">
                        <button
                          aria-expanded={open}
                          aria-label={`ดูรายการในใบโอน ${text(row.transfer_code)}`}
                          className="grid h-6 w-6 place-items-center rounded text-muted-foreground"
                          onClick={(event) => { event.stopPropagation(); setExpanded(open ? null : id); }}
                          type="button"
                        >
                          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                        </button>
                      </td>
                      <td className="whitespace-nowrap px-3 py-3 font-medium">{text(row.transfer_code)}</td>
                      <td className="whitespace-nowrap px-3 py-3">{text(row.source_branch_name)}</td>
                      <td className="whitespace-nowrap px-3 py-3">{text(row.destination_branch_name)}</td>
                      <td className="px-3 py-3">{count(row.item_count)}</td>
                      <td className="whitespace-nowrap px-3 py-3">{statusLabel(row.status)}</td>
                      {canUseGhost ? <td className="px-3 py-3">{row.has_discrepancy ? "ใช่" : "ไม่"}</td> : null}
                      <td className="whitespace-nowrap px-3 py-3">{row.requested_at ? dateTime(text(row.requested_at)) : "-"}</td>
                    </tr>
                    {open ? (
                      <tr>
                        <td className="bg-muted/20 px-3 pb-4 pt-0" colSpan={columnCount}>
                          <div className="overflow-hidden rounded-lg border bg-white">
                            <table className="w-full text-sm">
                              <thead className="bg-muted/60 text-xs text-muted-foreground">
                                <tr>
                                  <th className="px-3 py-2 text-left font-medium">สินค้า</th>
                                  <th className="px-3 py-2 text-left font-medium">SKU</th>
                                  <th className="px-3 py-2 text-right font-medium">จำนวนที่ส่ง</th>
                                  <th className="px-3 py-2 text-right font-medium">จำนวนที่รับ</th>
                                  {canUseGhost ? <th className="px-3 py-2 text-left font-medium">ประเภทสต๊อก</th> : null}
                                  <th className="px-3 py-2 text-left font-medium">หมายเหตุส่วนต่าง</th>
                                </tr>
                              </thead>
                              <tbody className="divide-y">
                                {lines.length === 0 ? (
                                  <tr>
                                    <td className="px-3 py-6 text-center text-muted-foreground" colSpan={canUseGhost ? 6 : 5}>
                                      ไม่มีรายการในใบโอนนี้
                                    </td>
                                  </tr>
                                ) : (
                                  lines.map((line, index) => (
                                    <tr key={text(line.id) || index}>
                                      <td className="px-3 py-2 font-medium">{text(line.product_name)}</td>
                                      <td className="px-3 py-2 text-muted-foreground">{text(line.sku)}</td>
                                      <td className="px-3 py-2 text-right tabular-nums">{count(line.quantity)}</td>
                                      <td className="px-3 py-2 text-right tabular-nums">
                                        {line.received_quantity == null ? "-" : count(line.received_quantity)}
                                      </td>
                                      {canUseGhost ? (
                                        <td className="px-3 py-2">{text(line.stock_bucket) === "ghost" ? "สต๊อกผี" : "สต๊อกจริง"}</td>
                                      ) : null}
                                      <td className="px-3 py-2 text-muted-foreground">{text(line.discrepancy_note) || "-"}</td>
                                    </tr>
                                  ))
                                )}
                              </tbody>
                            </table>
                          </div>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>
      <Pagination className="mt-4" {...pager} />
    </SectionCard>
  );
}
