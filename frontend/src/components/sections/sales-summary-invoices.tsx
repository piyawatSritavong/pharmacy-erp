"use client";

import { useMemo, useState } from "react";
import { ChevronDown, ChevronRight, FileClock } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/primitives";
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
export function SalesSummaryInvoices({ invoices, everyone = false }: { invoices: Row[]; everyone?: boolean }) {
  const groups = useMemo<Group[]>(() => {
    const buckets = new Map<string, Group>();
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
          total: 0
        });
      }
      const group = buckets.get(key)!;
      group.rows.push(invoice);
      group.total += Number(invoice.total_amount || 0);
    }
    return [...buckets.values()].sort((a, b) => {
      if (a.key === OPEN_KEY) return -1;
      if (b.key === OPEN_KEY) return 1;
      return (b.periodStart || "").localeCompare(a.periodStart || "");
    });
  }, [invoices]);

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
        <p className="py-10 text-center text-sm text-muted-foreground">ไม่มีใบขายในช่วงวันที่เลือก</p>
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
                    <span className="block text-xs text-muted-foreground">{group.rows.length.toLocaleString("th-TH")} ใบ</span>
                  </span>
                </button>
                {expanded ? (
                  <TableContainer>
                    <Table>
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
                        {group.rows.map((invoice) => (
                          <TableRow key={text(invoice.id)}>
                            <TableCell className="whitespace-nowrap font-medium">{text(invoice.invoice_number)}</TableCell>
                            <TableCell>{text(invoice.customer_name)}</TableCell>
                            {everyone ? <TableCell className="whitespace-nowrap">{text(invoice.branch_name)}</TableCell> : null}
                            <TableCell>{statusLabels[text(invoice.payment_status)] || text(invoice.payment_status)}</TableCell>
                            <TableCell className="whitespace-nowrap text-right tabular-nums">{currency(Number(invoice.total_amount || 0))}</TableCell>
                            <TableCell className="whitespace-nowrap">{dateTime(invoice.issued_at)}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </SectionCard>
  );
}
