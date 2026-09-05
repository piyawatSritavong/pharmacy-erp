"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard } from "@/components/sections/common";
import { currency } from "@/lib/utils";
import type { BreakdownGroup, DailyBreakdown } from "@/services/erp";

type GroupKey =
  | "transfer_abbreviated"
  | "transfer_full_tax"
  | "cash_full_tax"
  | "cash_ghost_hidden"
  | "cash_repriced"
  | "cash_mixed"
  | "unpaid";

type GroupSpec = {
  key: GroupKey;
  title: string;
  note: string;
  paymentType: "cash" | "bank_transfer";
  closeStatus: "active" | "adjusted" | "hidden";
  paid: boolean;
  showsAdjusted?: boolean;
};

// Money that settles as rung up: the close leaves all three alone.
const REAL_GROUPS: GroupSpec[] = [
  {
    key: "transfer_abbreviated",
    title: "ยอดโอน",
    note: "ใบกำกับภาษีอย่างย่อ",
    paymentType: "bank_transfer",
    closeStatus: "active",
    paid: true
  },
  {
    key: "transfer_full_tax",
    title: "ยอดโอน + ใบกำกับเต็มรูป",
    note: "ออกใบกำกับภาษีเต็มรูปแล้ว",
    paymentType: "bank_transfer",
    closeStatus: "active",
    paid: true
  },
  {
    key: "cash_full_tax",
    title: "เงินสด + ใบกำกับเต็มรูป",
    note: "ออกใบกำกับภาษีเต็มรูปแล้ว",
    paymentType: "cash",
    closeStatus: "active",
    paid: true
  }
];

// Cash without a full tax invoice — the pool the close acts on, split by what
// Ghost Stock at the warehouse can cover right now.
const CLOSE_GROUPS: GroupSpec[] = [
  {
    key: "cash_ghost_hidden",
    title: "มีในสต๊อกผี — ต้องหายไป",
    note: "ใบกำกับภาษีอย่างย่อ · ทุกรายการมีผีคุ้ม",
    paymentType: "cash",
    closeStatus: "hidden",
    paid: true
  },
  {
    key: "cash_repriced",
    title: "ไม่มีในสต๊อกผี — ต้องปรับราคา",
    note: "ใบกำกับภาษีอย่างย่อ · ไม่มีผีคุ้มสักรายการ",
    paymentType: "cash",
    closeStatus: "adjusted",
    paid: true,
    showsAdjusted: true
  },
  {
    key: "cash_mixed",
    title: "บิลผสม — หายบางรายการ ปรับบางรายการ",
    note: "ใบกำกับภาษีอย่างย่อ · ผีคุ้มบางรายการ",
    paymentType: "cash",
    closeStatus: "adjusted",
    paid: true,
    showsAdjusted: true
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

function readGroup(source: Record<string, unknown>, key: string): BreakdownGroup {
  const value = source?.[key] as BreakdownGroup | undefined;
  return value || { amount: 0, invoice_count: 0 };
}

function bills(count: number) {
  return `${count.toLocaleString("th-TH")} บิล`;
}

/** One measured figure. Amounts are tabular so columns of them line up. */
function GroupTile({ spec, group, compact }: { spec: GroupSpec; group: BreakdownGroup; compact?: boolean }) {
  const empty = group.invoice_count === 0;
  return (
    <div
      className={`rounded-xl border bg-card px-4 ${compact ? "py-3" : "py-4"} ${empty ? "opacity-55" : ""}`}
    >
      <p className={`font-medium ${compact ? "text-xs" : "text-sm"}`}>{spec.title}</p>
      {compact ? null : <p className="mt-0.5 text-xs text-muted-foreground">{spec.note}</p>}
      <p className={`mt-2 font-semibold tabular-nums ${compact ? "text-lg" : "text-2xl"}`}>{currency(group.amount)}</p>
      <div className="mt-1 flex flex-wrap items-baseline gap-x-3 text-xs text-muted-foreground">
        <span>{bills(group.invoice_count)}</span>
        {spec.showsAdjusted && group.adjusted_amount !== undefined ? (
          <span>
            ปรับเหลือ <span className="font-semibold tabular-nums text-foreground">{currency(group.adjusted_amount)}</span>
          </span>
        ) : null}
      </div>
    </div>
  );
}

/** A titled group of tiles with its own running total. */
function Panel({
  title,
  note,
  specs,
  source,
  filters,
  compact,
  provisional
}: {
  title: string;
  note: string;
  specs: GroupSpec[];
  source: Record<string, unknown>;
  filters: BreakdownFilters;
  compact?: boolean;
  provisional?: boolean;
}) {
  const visible = specs.filter((spec) => keep(spec, filters));
  if (visible.length === 0) return null;
  const total = visible.reduce((sum, spec) => sum + readGroup(source, spec.key).amount, 0);
  const totalBills = visible.reduce((sum, spec) => sum + readGroup(source, spec.key).invoice_count, 0);

  return (
    // A dashed edge on the provisional side: these figures are what the close
    // WOULD do if it ran now, not money that has settled.
    <div className={`rounded-2xl border p-4 ${provisional ? "border-dashed bg-muted/40" : "bg-muted/20"}`}>
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <p className={`font-semibold ${compact ? "text-sm" : "text-base"}`}>{title}</p>
          {compact ? null : <p className="text-xs text-muted-foreground">{note}</p>}
        </div>
        <p className={`font-semibold tabular-nums ${compact ? "text-base" : "text-xl"}`}>
          {currency(total)}
          <span className="ml-2 text-xs font-normal text-muted-foreground">{bills(totalBills)}</span>
        </p>
      </div>
      <div className={`grid gap-3 ${compact ? "sm:grid-cols-3" : "md:grid-cols-3"}`}>
        {visible.map((spec) => (
          <GroupTile compact={compact} group={readGroup(source, spec.key)} key={spec.key} spec={spec} />
        ))}
      </div>
    </div>
  );
}

function Breakdown({
  source,
  filters,
  showsClose,
  markup,
  compact
}: {
  source: Record<string, unknown>;
  filters: BreakdownFilters;
  showsClose: boolean;
  markup: number;
  compact?: boolean;
}) {
  const unpaid = readGroup(source, "unpaid");
  return (
    <div className="space-y-3">
      <Panel
        compact={compact}
        filters={filters}
        note="เงินที่เข้าแล้วและรอบสิ้นเดือนจะไม่แตะ"
        source={source}
        specs={REAL_GROUPS}
        title="ยอดรวมจริง"
      />
      {showsClose ? (
        <Panel
          compact={compact}
          filters={filters}
          note={`ถ้าปิดรอบตอนนี้ — ต้นทุน + ${markup}%`}
          provisional
          source={source}
          specs={CLOSE_GROUPS}
          title={`ยอดเข้าเงื่อนไข ต้นทุน + ${markup}%`}
        />
      ) : null}
      {unpaid.invoice_count > 0 && keep(UNPAID_GROUP, filters) ? (
        <div className="rounded-xl border border-dashed px-4 py-3 text-sm">
          <span className="font-medium">{UNPAID_GROUP.title}</span>
          <span className="ml-2 text-muted-foreground">{UNPAID_GROUP.note}</span>
          <span className="ml-3 font-semibold tabular-nums">{currency(unpaid.amount)}</span>
          <span className="ml-2 text-xs text-muted-foreground">{bills(unpaid.invoice_count)}</span>
        </div>
      ) : null}
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

  const heading = useMemo(
    () => (data.date_from === data.date_to ? "ยอดขายรวมวันนี้ทุกสาขา" : "ยอดขายรวมทุกสาขาในช่วงที่เลือก"),
    [data.date_from, data.date_to]
  );

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
        description={data.date_from === data.date_to ? data.date_from : `${data.date_from} ถึง ${data.date_to}`}
        title={heading}
      >
        <Breakdown
          filters={filters}
          markup={data.markup_percent}
          showsClose={data.shows_close}
          source={data.overall}
        />
      </SectionCard>

      <SectionCard description="แยกตามสาขา เรียงจากยอดมากไปน้อย" title="ยอดขายแต่ละสาขา">
        <div className="space-y-4">
          {data.branches.map((branch) => (
            <div className="rounded-2xl border bg-card p-4" key={String(branch.branch_id)}>
              <p className="mb-3 font-semibold">
                {String(branch.branch_name)}
                <span className="ml-2 text-xs font-normal text-muted-foreground">{String(branch.branch_code)}</span>
              </p>
              <Breakdown
                compact
                filters={filters}
                markup={data.markup_percent}
                showsClose={data.shows_close}
                source={branch as Record<string, unknown>}
              />
            </div>
          ))}
        </div>
      </SectionCard>
    </>
  );
}
