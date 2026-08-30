import { Banknote, Landmark, Clock3 } from "lucide-react";

import { BarChart } from "@/components/ui/bar-chart";
import { EmptyState } from "@/components/ui/primitives";
import { currency, dateTime } from "@/lib/utils";

type BranchSales = {
  branch_id: string;
  branch_code: string;
  branch_name: string;
  invoice_count: number;
  total_amount: number;
  cash_amount: number;
  transfer_amount: number;
  last_sale_at: string | null;
};

/**
 * Fixed 3-column grid of per-branch sales cards at the top of the dashboard.
 * Unlike everything below it, this block is intentionally hardcoded rather
 * than pinned-report-driven: it is the one at-a-glance view of every selling
 * branch — ยอดรวม, เงินสด, เงินโอน, and when that branch last sold.
 */
export function BranchSalesGrid({ items }: { items: BranchSales[] }) {
  if (!items.length) {
    return <EmptyState description="ยังไม่มีสาขาที่เปิดขาย" />;
  }

  return (
    <section aria-label="ยอดขายแต่ละสาขา" className="space-y-3">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 className="text-xl font-semibold tracking-tight">ยอดขายแต่ละสาขา</h2>
        <span className="text-xs text-muted-foreground">ยอดสะสมทั้งหมด · แยกตามวิธีรับชำระ</span>
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {items.map((item) => (
          <BranchCard item={item} key={item.branch_id} />
        ))}
      </div>
    </section>
  );
}

function BranchCard({ item }: { item: BranchSales }) {
  const cash = Number(item.cash_amount) || 0;
  const transfer = Number(item.transfer_amount) || 0;
  const total = Number(item.total_amount) || 0;
  // Share of collected money, not of total_amount — an unpaid invoice would
  // otherwise make the two bars look wrong against the header figure.
  const collected = cash + transfer;
  const cashShare = collected > 0 ? Math.round((cash / collected) * 100) : 0;

  return (
    <article className="flex flex-col gap-4 rounded-2xl border bg-card p-5 shadow-card">
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-base font-semibold" title={item.branch_name}>
            {item.branch_name}
          </h3>
          <p className="text-xs text-muted-foreground">
            {item.branch_code} · {item.invoice_count.toLocaleString("th-TH")} ใบขาย
          </p>
        </div>
        <span className="shrink-0 rounded-full bg-primary/10 px-3 py-1 text-xs font-semibold text-primary">
          {cashShare}% เงินสด
        </span>
      </header>

      <div>
        <p className="text-xs font-medium text-muted-foreground">ยอดรวม</p>
        <p className="text-2xl font-semibold tracking-tight tabular-nums">{currency(total)}</p>
      </div>

      <BarChart
        ariaLabel={`กราฟยอดเงินสดและเงินโอนของ ${item.branch_name}`}
        format="currency"
        height="h-20"
        points={[
          { label: "เงินสด", value: cash },
          { label: "เงินโอน", value: transfer }
        ]}
      />

      <dl className="grid grid-cols-2 gap-3 border-t pt-3 text-sm">
        <div>
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Banknote className="h-3.5 w-3.5" />
            เงินสด
          </dt>
          <dd className="mt-0.5 font-semibold tabular-nums">{currency(cash)}</dd>
        </div>
        <div>
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Landmark className="h-3.5 w-3.5" />
            เงินโอน
          </dt>
          <dd className="mt-0.5 font-semibold tabular-nums">{currency(transfer)}</dd>
        </div>
        <div className="col-span-2">
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Clock3 className="h-3.5 w-3.5" />
            เวลาการทำรายการล่าสุด
          </dt>
          <dd className="mt-0.5 font-medium">
            {item.last_sale_at ? dateTime(item.last_sale_at) : <span className="text-muted-foreground">ยังไม่มีการขาย</span>}
          </dd>
        </div>
      </dl>
    </article>
  );
}
