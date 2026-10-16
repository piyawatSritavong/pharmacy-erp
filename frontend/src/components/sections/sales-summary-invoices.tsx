"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, FileClock } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Button, EmptyState, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/primitives";
import { currency } from "@/lib/utils";
import { periodLabel, shortDate } from "@/lib/month-end-groups";

type Row = Record<string, unknown>;

type Group = {
  key: string;
  number: string;
  periodStart: string;
  periodEnd: string;
  rows: Row[];
  total: number;
  count: number;
};

const OPEN_KEY = "__open__";

function text(value: unknown) {
  return value == null ? "" : String(value);
}

function dateTime(value: unknown) {
  const raw = text(value);
  if (!raw) return "-";
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) return raw;
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(parsed);
}

const statusLabels: Record<string, string> = { paid: "ชำระแล้ว", unpaid: "ยังไม่ชำระ", partial: "ชำระบางส่วน" };

/** Bills grouped by the month-end round that closed them.
 *
 *  A flat list grows without bound across years, and a month can hold several
 *  rounds because the round is chosen by date range. Bills not yet in any round
 *  sit in their own group at the top.
 */
const ROWS_STEP = 30;

export function SalesSummaryInvoices({
  invoices,
  groups: serverGroups = [],
  total = invoices.length,
  dateFrom = "",
  dateTo = "",
  everyone = false
}: {
  /** The newest bills only; the server caps how many it sends. */
  invoices: Row[];
  /** Every round's count and total, over all bills in the range. */
  groups?: Row[];
  total?: number;
  dateFrom?: string;
  dateTo?: string;
  everyone?: boolean;
}) {
  const [shown, setShown] = useState<Record<string, number>>({});
  const groups = useMemo<Group[]>(() => {
    const buckets = new Map<string, Group>();
    for (const group of serverGroups) {
      const number = text(group.reconciliation_number);
      const key = number || OPEN_KEY;
      buckets.set(key, {
        key,
        number,
        periodStart: text(group.reconciliation_period_start),
        periodEnd: text(group.reconciliation_period_end),
        rows: [],
        total: Number(group.total || 0),
        count: Number(group.count || 0)
      });
    }
    for (const invoice of invoices) {
      const number = text(invoice.reconciliation_number);
      const key = number || OPEN_KEY;
      if (!buckets.has(key)) {
        buckets.set(key, {
          key,
          number,
          periodStart: text(invoice.reconciliation_period_start),
          periodEnd: text(invoice.reconciliation_period_end),
          rows: [],
          total: 0,
          count: 0
        });
      }
      const group = buckets.get(key)!;
      group.rows.push(invoice);
      if (!serverGroups.length) {
        group.total += Number(invoice.total_amount || 0);
        group.count += 1;
      }
    }
    return [...buckets.values()].sort((a, b) => {
      if (a.key === OPEN_KEY) return -1;
      if (b.key === OPEN_KEY) return 1;
      return (b.periodStart || "").localeCompare(a.periodStart || "");
    });
  }, [invoices, serverGroups]);

  // The newest group opens; older rounds stay folded until asked for.
  const [open, setOpen] = useState<Record<string, boolean>>({});
  function isOpen(group: Group, index: number) {
    return open[group.key] ?? index === 0;
  }

  return (
    <SectionCard
      title={everyone ? "ใบขายทุกสาขา" : "ใบขายของคุณ"}
      description={`${everyone ? "ใบขายทุกสาขาในช่วงวันที่เลือก" : "แสดงเฉพาะใบขายที่พนักงานคนปัจจุบันสร้างในช่วงวันที่เลือก"} · ยุบเป็นกลุ่มตามรอบสรุปสิ้นเดือน`}
    >
      {groups.length === 0 ? (
        <EmptyState description="ไม่มีใบขายในช่วงวันที่เลือก" />
      ) : (
        <div className="space-y-3">
          {groups.map((group, index) => {
            const expanded = isOpen(group, index);
            return (
              <div className="overflow-hidden rounded-lg border" key={group.key}>
                <button
                  aria-expanded={expanded}
                  className="flex w-full items-center gap-3 bg-muted/40 px-4 py-3 text-left transition hover:bg-muted"
                  onClick={() => setOpen((current) => ({ ...current, [group.key]: !expanded }))}
                  type="button"
                >
                  {expanded ? <ChevronDown className="h-4 w-4 shrink-0" /> : <ChevronRight className="h-4 w-4 shrink-0" />}
                  <FileClock className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="block font-semibold">
                      {group.key === OPEN_KEY ? "ยังไม่อยู่ในรอบสรุปสิ้นเดือน" : group.number}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {group.key === OPEN_KEY
                        ? "ใบขายที่ยังไม่ถูกปิดรอบ"
                        : `${periodLabel(group.periodStart)} · ${shortDate(group.periodStart)} ถึง ${shortDate(group.periodEnd)}`}
                    </span>
                  </span>
                  <span className="shrink-0 text-right text-sm">
                    <span className="block font-semibold tabular-nums">{currency(group.total)}</span>
                    <span className="block text-xs text-muted-foreground">{group.count.toLocaleString("th-TH")} ใบ</span>
                  </span>
                </button>
                {expanded ? (
                  <div className="p-2 sm:p-0">
                    <TableContainer>
                      <Table className="mobile-card-table">
                        <TableHeader>
                          <TableRow>
                            <TableHead>เลขที่ใบขาย</TableHead>
                            <TableHead>ลูกค้า</TableHead>
                            {everyone ? <TableHead>สาขา</TableHead> : null}
                            <TableHead>สถานะ</TableHead>
                            <TableHead className="text-right">ยอดรวม</TableHead>
                            <TableHead>วันที่ออก</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {group.rows.slice(0, shown[group.key] || ROWS_STEP).map((invoice) => (
                            <TableRow key={text(invoice.id)}>
                              <TableCell className="whitespace-nowrap font-medium" data-label="เลขที่ใบขาย" data-primary="true">{text(invoice.invoice_number)}</TableCell>
                              <TableCell data-label="ลูกค้า">{text(invoice.customer_name) || "-"}</TableCell>
                              {everyone ? <TableCell className="whitespace-nowrap" data-label="สาขา">{text(invoice.branch_name)}</TableCell> : null}
                              <TableCell data-label="สถานะ">{statusLabels[text(invoice.payment_status)] || text(invoice.payment_status)}</TableCell>
                              <TableCell className="whitespace-nowrap text-right tabular-nums" data-label="ยอดรวม">{currency(Number(invoice.total_amount || 0))}</TableCell>
                              <TableCell className="whitespace-nowrap" data-label="วันที่ออก">{dateTime(invoice.issued_at)}</TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                    {group.rows.length > (shown[group.key] || ROWS_STEP) ? (
                      <div className="p-3 text-center">
                        <Button onClick={() => setShown((current) => ({ ...current, [group.key]: (current[group.key] || ROWS_STEP) + ROWS_STEP }))} type="button" variant="secondary">
                          แสดงเพิ่ม
                        </Button>
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          })}
          {total > invoices.length ? (
            <p className="text-sm text-muted-foreground">
              แสดง {invoices.length.toLocaleString("th-TH")} บิลล่าสุดจากทั้งหมด {total.toLocaleString("th-TH")} บิล ·{" "}
              <Link className="font-semibold text-primary underline-offset-4 hover:underline" href={`/sales-history?date_from=${dateFrom}&date_to=${dateTo}`}>
                ดูทั้งหมดในประวัติการขาย
              </Link>
            </p>
          ) : null}
        </div>
      )}
    </SectionCard>
  );
}
