"use client";

import { useMemo, useState } from "react";
import { Search } from "lucide-react";

import { AuditTimeline, SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Button, EmptyState, Input, Pagination, Select } from "@/components/ui/primitives";

type Option = Record<string, unknown>;

const PAGE_SIZE_DEFAULT = 20;

/**
 * ประวัติระบบ — the system-wide activity log, a top-level menu under เมนู: ระบบ
 * (business-flow.md). Admins see every branch; other roles see only their own
 * branch, which the backend enforces on the query.
 *
 * The filter bar submits to the server (GET /audit which re-queries the API);
 * paging is client-side over what came back, since GET /audit-logs takes no
 * page params and caps its own result set.
 */
export function AuditConsole({
  branches,
  items,
  filters
}: {
  branches: Option[];
  items: Option[];
  filters: Record<string, string>;
}) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_DEFAULT);

  const { rows, totalPages, safePage } = useMemo(() => {
    const pages = Math.max(1, Math.ceil(items.length / pageSize));
    const current = Math.min(Math.max(1, page), pages);
    return {
      rows: items.slice((current - 1) * pageSize, current * pageSize),
      totalPages: pages,
      safePage: current
    };
  }, [items, page, pageSize]);

  return (
    <SectionCard description="กรองตามสาขา ประเภทข้อมูล การกระทำ และช่วงวันที่ — เรียงจากรายการล่าสุด" title="ประวัติการทำงาน">
      <form action="/audit" className="mb-4 flex flex-wrap items-end gap-3" method="GET">
        <Field className="w-48" label="สาขา">
          <Select aria-label="กรองตามสาขา" defaultValue={filters.branch_id} name="branch_id">
            <option value="">ทุกสาขา</option>
            {branches.map((branch) => (
              <option key={String(branch.id)} value={String(branch.id)}>
                {String(branch.name)}
              </option>
            ))}
          </Select>
        </Field>
        <Field className="w-44" label="ประเภทข้อมูล">
          <Input aria-label="ประเภทข้อมูล" defaultValue={filters.entity_type} name="entity_type" placeholder="เช่น invoice" />
        </Field>
        <Field className="w-44" label="การกระทำ">
          <Input aria-label="การกระทำ" defaultValue={filters.action} name="action" placeholder="เช่น pos.checkout" />
        </Field>
        <Field className="w-40" label="ตั้งแต่วันที่">
          <Input aria-label="ตั้งแต่วันที่" defaultValue={filters.date_from} name="date_from" type="date" />
        </Field>
        <Field className="w-40" label="ถึงวันที่">
          <Input aria-label="ถึงวันที่" defaultValue={filters.date_to} name="date_to" type="date" />
        </Field>
        <Button type="submit">
          <Search className="h-4 w-4" />
          ค้นหา
        </Button>
      </form>

      {items.length === 0 ? <EmptyState description="ลองปรับตัวกรองแล้วค้นหาใหม่" /> : <AuditTimeline items={rows} />}

      <Pagination
        className="mt-4"
        onPageChange={setPage}
        onPageSizeChange={(size) => { setPageSize(size); setPage(1); }}
        page={safePage}
        pageSize={pageSize}
        total={items.length}
        totalPages={totalPages}
      />
    </SectionCard>
  );
}
