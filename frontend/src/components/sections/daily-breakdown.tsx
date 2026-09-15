"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowRight, Receipt } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { currency } from "@/lib/utils";
import type { BreakdownGroup, DailyBreakdown, TenderSplit } from "@/services/erp";

type GroupKey =
  | "transfer_abbreviated"
  | "transfer_full_tax"
  | "cash_full_tax"
  | "mixed_abbreviated"
  | "mixed_full_tax"
  | "cash_ghost_hidden"
  | "cash_repriced"
  | "cash_mixed"
  | "unpaid";

type GroupSpec = {
  key: GroupKey;
  title: string;
  note: string;
  paymentType: "cash" | "bank_transfer" | "mixed";
  closeStatus: "active" | "adjusted" | "hidden";
  paid: boolean;
  /** Which tax invoice this group is, where the group is defined by it. */
  taxInvoice?: "full" | "abbreviated";
};

// Money that settles as rung up: the close leaves all of these alone.
const REAL_GROUPS: GroupSpec[] = [
  {
    key: "transfer_abbreviated",
    taxInvoice: "abbreviated",
    title: "ยอดโอน",
    note: "ใบกำกับภาษีอย่างย่อ",
    paymentType: "bank_transfer",
    closeStatus: "active",
    paid: true
  },
  {
    key: "transfer_full_tax",
    taxInvoice: "full",
    title: "ยอดโอน + ใบกำกับเต็มรูป",
    note: "ออกใบกำกับภาษีเต็มรูปแล้ว",
    paymentType: "bank_transfer",
    closeStatus: "active",
    paid: true
  },
  {
    key: "cash_full_tax",
    taxInvoice: "full",
    title: "เงินสด + ใบกำกับเต็มรูป",
    note: "ออกใบกำกับภาษีเต็มรูปแล้ว",
    paymentType: "cash",
    closeStatus: "active",
    paid: true
  },
  {
    key: "mixed_abbreviated",
    taxInvoice: "abbreviated",
    title: "เงินสด + โอน ผสม",
    note: "ใบกำกับภาษีอย่างย่อ · จ่ายสองทาง จึงไม่เข้าเงื่อนไข",
    paymentType: "mixed",
    closeStatus: "active",
    paid: true
  },
  {
    key: "mixed_full_tax",
    taxInvoice: "full",
    title: "เงินสด + โอน ผสม + ใบกำกับเต็มรูป",
    note: "จ่ายสองทาง จึงไม่เข้าเงื่อนไข",
    paymentType: "mixed",
    closeStatus: "active",
    paid: true
  }
];

// Cash without a full tax invoice — the pool the close acts on, split by what
// Ghost Stock at the warehouse covers.
const CLOSE_GROUPS: GroupSpec[] = [
  {
    key: "cash_ghost_hidden",
    taxInvoice: "abbreviated",
    title: "มีในสต๊อกผี — ต้องหายไป",
    note: "ใบกำกับภาษีอย่างย่อ · ทุกรายการมีผีคุ้ม",
    paymentType: "cash",
    closeStatus: "hidden",
    paid: true
  },
  {
    key: "cash_repriced",
    taxInvoice: "abbreviated",
    title: "ไม่มีในสต๊อกผี — ต้องปรับราคา",
    note: "ใบกำกับภาษีอย่างย่อ · ไม่มีผีคุ้มสักรายการ",
    paymentType: "cash",
    closeStatus: "adjusted",
    paid: true
  },
  {
    key: "cash_mixed",
    taxInvoice: "abbreviated",
    title: "บิลผสม — หายบางรายการ ปรับบางรายการ",
    note: "ใบกำกับภาษีอย่างย่อ · ผีคุ้มบางรายการ",
    paymentType: "cash",
    closeStatus: "adjusted",
    paid: true
  }
];

const UNPAID_GROUP: GroupSpec = {
  key: "unpaid",
  title: "ค้างชำระ",
  note: "ออกบิลแล้วแต่ยังไม่ได้รับเงิน",
  paymentType: "cash",
  closeStatus: "active",
  paid: false
};

export type BreakdownFilters = {
  closeStatus: string;
  paymentType: string;
  paymentStatus: string;
};

function keep(spec: GroupSpec, filters: BreakdownFilters) {
  if (filters.closeStatus && spec.closeStatus !== filters.closeStatus) return false;
  if (filters.paymentType && spec.paymentType !== filters.paymentType) return false;
  if (filters.paymentStatus === "paid" && !spec.paid) return false;
  if (filters.paymentStatus === "unpaid" && spec.paid) return false;
  return true;
}

const EMPTY: BreakdownGroup = {
  amount: 0,
  invoice_count: 0,
  after_amount: 0,
  after_count: 0,
  difference: 0,
  difference_count: 0
};

function readGroup(source: Record<string, unknown>, key: string): BreakdownGroup {
  return (source?.[key] as BreakdownGroup | undefined) || EMPTY;
}

function bills(count: number) {
  return `${count.toLocaleString("th-TH")} บิล`;
}

/**
 * A tile states a total; this is how the reader gets to the bills that make it
 * up. Every filter the group is defined by travels in the link, so the history
 * page opens already narrowed to exactly this set instead of to everything.
 */
function historyHref(spec: GroupSpec, data: { date_from: string; date_to: string }, branchName?: string) {
  const query = new URLSearchParams({ date_from: data.date_from, date_to: data.date_to });
  if (spec.key !== "unpaid") query.set("payment_method", spec.paymentType);
  if (spec.taxInvoice) query.set("tax_invoice_type", spec.taxInvoice);
  if (spec.key === "unpaid") query.set("payment_status", "unpaid");
  else query.set("close_status", spec.closeStatus);
  // Both repriced groups read as "adjusted"; what separates them is whether the
  // close also struck lines off the bill.
  if (spec.key === "cash_mixed") query.set("removed_lines", "1");
  if (spec.key === "cash_repriced") query.set("removed_lines", "0");
  if (branchName) query.set("branch", branchName);
  return `/sales-history?${query.toString()}`;
}

function ViewBillsLink({ href, count }: { href: string; count: number }) {
  if (count === 0) return null;
  return (
    <Link
      className="mt-2 inline-flex min-h-11 items-center sm:min-h-0 gap-1.5 text-xs font-medium text-primary transition hover:underline"
      href={href}
    >
      <Receipt className="h-3.5 w-3.5" />
      ดูบิล
    </Link>
  );
}

/**
 * One measured figure, in the two readings the screen has to support.
 *
 * Before the round is closed the second number is what the close would leave;
 * afterwards it is what it left. Either way the difference is stated in money
 * and in bills, because a bill that disappears takes its count with it and a
 * total alone does not show that.
 */
function GroupTile({
  spec,
  group,
  closed,
  compact,
  href
}: {
  spec: GroupSpec;
  group: BreakdownGroup;
  closed: boolean;
  compact?: boolean;
  href: string;
}) {
  // Once the round is closed every figure states both sides, even where they
  // match: "this money was not touched" is an audit answer, and a tile that
  // simply omits the second number does not give it. While the round is open
  // there is nothing to say unless the close would move something.
  const moved = closed || group.difference !== 0 || group.difference_count !== 0;
  const lead = closed ? group.after_amount : group.amount;
  const leadCount = closed ? group.after_count : group.invoice_count;
  return (
    <div className={`rounded-xl border bg-card px-3 sm:px-4 ${compact ? "py-2 sm:py-3" : "py-2.5 sm:py-4"} `}>
      <p className={`font-medium ${compact ? "text-xs" : "text-sm"}`}>{spec.title}</p>
      {compact ? null : <p className="mt-0.5 text-xs text-muted-foreground">{spec.note}</p>}

      <div className="mt-2 flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span className={`font-semibold tabular-nums ${compact ? "text-base sm:text-lg" : "text-xl sm:text-2xl"}`}>{currency(lead)}</span>
        <span className="text-xs text-muted-foreground">{bills(leadCount)}</span>
      </div>

      {moved ? (
        <div className="mt-2 space-y-1 border-t pt-2 text-xs">
          <p className="flex flex-wrap items-center gap-1.5 text-muted-foreground">
            <ArrowRight className="h-3 w-3 shrink-0" />
            <span>{closed ? "ก่อนปรับ" : "ถ้าปิดรอบ"}</span>
            <span className="font-semibold tabular-nums text-foreground">
              {currency(closed ? group.amount : group.after_amount)}
            </span>
            <span>{bills(closed ? group.invoice_count : group.after_count)}</span>
          </p>
          <p className="text-muted-foreground">
            ส่วนต่าง{" "}
            {group.difference === 0 && group.difference_count === 0 ? (
              <span className="font-semibold tabular-nums text-foreground">ไม่มี</span>
            ) : (
              <>
                <span className="font-semibold tabular-nums text-error">−{currency(group.difference)}</span>
                {group.difference_count > 0 ? <span> · หายไป {bills(group.difference_count)}</span> : null}
              </>
            )}
          </p>
        </div>
      ) : null}
      <ViewBillsLink count={group.invoice_count} href={href} />
    </div>
  );
}

/** A titled group of tiles with its own running total, stated the same way. */
function Panel({
  title,
  note,
  specs,
  source,
  filters,
  totalKey,
  closed,
  compact,
  provisional,
  window,
  branchName
}: {
  title: string;
  note: string;
  specs: GroupSpec[];
  source: Record<string, unknown>;
  filters: BreakdownFilters;
  totalKey: string;
  closed: boolean;
  compact?: boolean;
  provisional?: boolean;
  window: { date_from: string; date_to: string };
  branchName?: string;
}) {
  const visible = specs.filter((spec) => keep(spec, filters));
  if (visible.length === 0) return null;
  // Filtering hides groups, so the heading adds up what is on screen rather
  // than quoting a server total for rows the operator cannot see.
  const showServerTotal = visible.length === specs.length;
  const total = showServerTotal
    ? readGroup(source, totalKey)
    : visible.reduce(
        (sum, spec) => {
          const group = readGroup(source, spec.key);
          return {
            ...sum,
            amount: sum.amount + group.amount,
            invoice_count: sum.invoice_count + group.invoice_count,
            after_amount: sum.after_amount + group.after_amount,
            after_count: sum.after_count + group.after_count,
            difference: sum.difference + group.difference,
            difference_count: sum.difference_count + group.difference_count
          };
        },
        { ...EMPTY }
      );

  return (
    // A dashed edge while the round is open: those figures are what the close
    // WOULD do, not money that has settled.
    <div className={`rounded-xl border p-2 sm:rounded-2xl sm:p-4 ${provisional && !closed ? "border-dashed bg-muted/40" : "bg-muted/20"}`}>
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <p className={`font-semibold ${compact ? "text-sm" : "text-base"}`}>{title}</p>
          {compact ? null : <p className="text-xs text-muted-foreground">{note}</p>}
        </div>
        <div className="text-right">
          <p className={`font-semibold tabular-nums ${compact ? "text-base" : "text-xl"}`}>
            {currency(closed ? total.after_amount : total.amount)}
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              {bills(closed ? total.after_count : total.invoice_count)}
            </span>
          </p>
          {closed || total.difference !== 0 || total.difference_count !== 0 ? (
            <p className="text-xs text-muted-foreground">
              {closed ? "ก่อนปรับ" : "ถ้าปิดรอบ"}{" "}
              <span className="font-semibold tabular-nums text-foreground">
                {currency(closed ? total.amount : total.after_amount)}
              </span>{" "}
              · {bills(closed ? total.invoice_count : total.after_count)} · ส่วนต่าง{" "}
              {total.difference === 0 && total.difference_count === 0 ? (
                <span className="font-semibold tabular-nums text-foreground">ไม่มี</span>
              ) : (
                <span className="font-semibold tabular-nums text-error">−{currency(total.difference)}</span>
              )}
            </p>
          ) : null}
        </div>
      </div>
      <div className={`grid gap-3 ${compact ? "sm:grid-cols-2 lg:grid-cols-3" : "md:grid-cols-3"}`}>
        {visible.map((spec) => (
          <GroupTile
            closed={closed}
            compact={compact}
            group={readGroup(source, spec.key)}
            href={historyHref(spec, window, branchName)}
            key={spec.key}
            spec={spec}
          />
        ))}
      </div>
    </div>
  );
}

function Breakdown({
  source,
  filters,
  markup,
  closed,
  compact,
  window,
  branchName
}: {
  source: Record<string, unknown>;
  filters: BreakdownFilters;
  markup: number;
  closed: boolean;
  compact?: boolean;
  window: { date_from: string; date_to: string };
  branchName?: string;
}) {
  const unpaid = readGroup(source, "unpaid");
  const day = readGroup(source, "day_total");
  // The headline is the figure the books currently hold, which is exactly what
  // central_admin's "ยอดรวม" shows — so the two screens can be opened side by
  // side and read against each other without adding two panels up by hand. It
  // is hidden while a filter is narrowing the page, because then it would be a
  // total for rows that are not on screen.
  const unfiltered = !filters.closeStatus && !filters.paymentType && !filters.paymentStatus;
  return (
    <div className="space-y-3">
      {unfiltered && day.invoice_count > 0 ? (
        <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1 rounded-2xl border bg-card px-4 py-3">
          <div>
            <p className={`font-semibold ${compact ? "text-sm" : "text-base"}`}>ยอดรวมทั้งวัน</p>
            {compact ? null : (
              <p className="text-xs text-muted-foreground">
                {closed ? "ตัวเลขนี้ตรงกับ “ยอดรวม” ที่ admin.central เห็น" : "ยอดที่ขายไปแล้ว ยังไม่ได้ปิดรอบ"}
              </p>
            )}
          </div>
          <div className="text-right">
            <p className={`font-semibold tabular-nums ${compact ? "text-xl" : "text-3xl"}`}>
              {currency(closed ? day.after_amount : day.amount)}
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {bills(closed ? day.after_count : day.invoice_count)}
              </span>
            </p>
            {closed ? (
              <p className="text-xs text-muted-foreground">
                ก่อนปรับ <span className="font-semibold tabular-nums text-foreground">{currency(day.amount)}</span> ·{" "}
                {bills(day.invoice_count)} · ส่วนต่าง{" "}
                {day.difference === 0 && day.difference_count === 0 ? (
                  <span className="font-semibold tabular-nums text-foreground">ไม่มี</span>
                ) : (
                  <span className="font-semibold tabular-nums text-error">−{currency(day.difference)}</span>
                )}
              </p>
            ) : null}
            <Link
              className="mt-1 inline-flex items-center gap-1.5 text-xs font-medium text-primary transition hover:underline"
              href={`/sales-history?date_from=${window.date_from}&date_to=${window.date_to}${branchName ? `&branch=${encodeURIComponent(branchName)}` : ""}`}
            >
              <Receipt className="h-3.5 w-3.5" />
              ดูบิลทั้งหมด
            </Link>
          </div>
        </div>
      ) : null}
      <Panel
        closed={closed}
        compact={compact}
        filters={filters}
        note={closed ? "เงินที่เข้าแล้วและรอบสิ้นเดือนไม่ได้แตะ" : "เงินที่เข้าแล้วและรอบสิ้นเดือนจะไม่แตะ"}
        source={source}
        specs={REAL_GROUPS}
        title="ยอดรวมจริง"
        totalKey="real_total"
        window={window}
        branchName={branchName}
      />
      <Panel
        closed={closed}
        compact={compact}
        filters={filters}
        note={closed ? `ปิดรอบแล้วที่ ต้นทุน + ${markup}%` : `ถ้าปิดรอบตอนนี้ — ต้นทุน + ${markup}%`}
        provisional
        source={source}
        specs={CLOSE_GROUPS}
        title={`ยอดเข้าเงื่อนไข ต้นทุน + ${markup}%`}
        totalKey="close_total"
        window={window}
        branchName={branchName}
      />
      {unpaid.invoice_count > 0 && keep(UNPAID_GROUP, filters) ? (
        <div className="rounded-xl border border-dashed px-4 py-3 text-sm">
          <span className="font-medium">{UNPAID_GROUP.title}</span>
          <span className="ml-2 text-muted-foreground">{UNPAID_GROUP.note}</span>
          <span className="ml-3 font-semibold tabular-nums">{currency(unpaid.amount)}</span>
          <span className="ml-2 text-xs text-muted-foreground">{bills(unpaid.invoice_count)}</span>
          <span className="ml-3 inline-block">
            <ViewBillsLink count={unpaid.invoice_count} href={historyHref(UNPAID_GROUP, window, branchName)} />
          </span>
        </div>
      ) : null}
    </div>
  );
}

/**
 * What central_admin's dashboard is: the money that arrived, split by how it
 * arrived. A bill settled part in cash and part by transfer lands on both
 * sides — the split follows the tender, not the bill.
 */
function TenderBoard({ tender, compact }: { tender: TenderSplit; compact?: boolean }) {
  const rows = [
    { label: "ยอดโอน", amount: tender.transfer_amount },
    { label: "ยอดเงินสด", amount: tender.cash_amount }
  ];
  return (
    <div className="rounded-2xl border bg-muted/20 p-4">
      {/* Sized to match the Superadmin day total, so the two screens can sit
          side by side and the eye lands on the same figure in both. */}
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
        <p className={`font-semibold ${compact ? "text-sm" : "text-base"}`}>ยอดรวม</p>
        <p className={`font-semibold tabular-nums ${compact ? "text-xl" : "text-3xl"}`}>
          {currency(tender.total_amount)}
          <span className="ml-2 text-xs font-normal text-muted-foreground">{bills(tender.invoice_count)}</span>
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {rows.map((row) => (
          <div className={`rounded-xl border bg-card px-3 sm:px-4 ${compact ? "py-2 sm:py-3" : "py-2.5 sm:py-4"}`} key={row.label}>
            <p className={`font-medium ${compact ? "text-xs" : "text-sm"}`}>{row.label}</p>
            <p className={`mt-2 font-semibold tabular-nums ${compact ? "text-base sm:text-lg" : "text-xl sm:text-2xl"}`}>{currency(row.amount)}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * Box 2 and Box 3: the day's takings for the whole company, then the same
 * breakdown for each branch.
 *
 * While the window is today the page refreshes itself, because the number is
 * being watched as it moves. On a past day nothing can change, so nothing polls.
 */
export function DailyBreakdownBoards({
  data,
  filters,
  live
}: {
  data: DailyBreakdown;
  filters: BreakdownFilters;
  live: boolean;
}) {
  const router = useRouter();
  const [refreshedAt, setRefreshedAt] = useState<string>("");

  useEffect(() => {
    if (!live) return;
    const timer = setInterval(() => {
      router.refresh();
      setRefreshedAt(new Intl.DateTimeFormat("th-TH", { timeZone: "Asia/Bangkok", timeStyle: "medium" }).format(new Date()));
    }, 20_000);
    return () => clearInterval(timer);
  }, [live, router]);

  const singleDay = data.date_from === data.date_to;
  const heading = useMemo(
    () => (singleDay ? "ยอดขายรวมวันนี้ทุกสาขา" : "ยอดขายรวมทุกสาขาในช่วงที่เลือก"),
    [singleDay]
  );
  const window = singleDay ? data.date_from : `${data.date_from} ถึง ${data.date_to}`;
  const description = data.closed && data.reconciliation_number
    ? `${window} · สรุปสิ้นเดือนแล้วในรอบ ${data.reconciliation_number} — ตัวเลขก่อนปรับมาจากบันทึกของรอบนั้น`
    : window;

  return (
    <>
      <SectionCard
        actions={
          live ? (
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className="h-2 w-2 rounded-full bg-primary" />
              อัปเดตสดทุก 20 วินาที{refreshedAt ? ` · ล่าสุด ${refreshedAt}` : ""}
            </span>
          ) : null
        }
        description={description}
        title={heading}
      >
        {data.shows_close ? (
          <Breakdown
            closed={data.closed}
            filters={filters}
            markup={data.markup_percent}
            source={data.overall as Record<string, unknown>}
            window={data}
          />
        ) : (
          <TenderBoard tender={data.overall.tender as TenderSplit} />
        )}
      </SectionCard>

      <SectionCard description="แยกตามสาขา เรียงจากยอดมากไปน้อย" title="ยอดขายแต่ละสาขา">
        <div className="space-y-4">
          {data.branches.map((branch) => (
            <div className="rounded-xl border bg-card p-2 sm:rounded-2xl sm:p-4" key={String(branch.branch_id)}>
              <p className="mb-3 font-semibold">
                {String(branch.branch_name)}
                <span className="ml-2 text-xs font-normal text-muted-foreground">{String(branch.branch_code)}</span>
              </p>
              {data.shows_close ? (
                <Breakdown
                  branchName={String(branch.branch_name)}
                  closed={data.closed}
                  compact
                  filters={filters}
                  markup={data.markup_percent}
                  source={branch as Record<string, unknown>}
                  window={data}
                />
              ) : (
                <TenderBoard compact tender={branch.tender as TenderSplit} />
              )}
            </div>
          ))}
        </div>
      </SectionCard>
    </>
  );
}
