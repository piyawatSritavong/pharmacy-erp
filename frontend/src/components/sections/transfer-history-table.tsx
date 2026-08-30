"use client";

import { useMemo, useState } from "react";
import { Search } from "lucide-react";

import { DataTable, SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Input, Pagination, Select, usePagedRows } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

/**
 * ประวัติใบโอน — filterable, paged transfer history. A client component
 * because GET /transfers returns every transfer in one response and takes no
 * page or search params, so the narrowing happens here.
 */
export function TransferHistoryTable({ branches, transfers }: { branches: Option[]; transfers: Option[] }) {
  const [search, setSearch] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

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

  return (
    <SectionCard description="สถานะการโอนสินค้าระหว่างทุกสาขา" title="ประวัติใบโอน">
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <Field className="w-64" label="ค้นหา">
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

      <DataTable
        columns={[
          { key: "transfer_code", label: "รหัสโอน", className: "whitespace-nowrap font-medium" },
          { key: "source_branch_name", label: "ต้นทาง", className: "whitespace-nowrap" },
          { key: "destination_branch_name", label: "ปลายทาง", className: "whitespace-nowrap" },
          { key: "item_count", label: "จำนวนรายการ" },
          { key: "status", label: "สถานะ", className: "whitespace-nowrap" },
          { key: "has_discrepancy", label: "พบส่วนต่าง" },
          { key: "requested_at", label: "วันที่สร้าง", type: "datetime", className: "whitespace-nowrap" }
        ]}
        emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
        rows={pageRows}
      />
      <Pagination className="mt-4" {...pager} />
    </SectionCard>
  );
}
