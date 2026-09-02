import { BarChart } from "@/components/ui/bar-chart";
import { currency } from "@/lib/utils";

type TodaySales = {
  date: string;
  total_amount: number;
  invoice_count: number;
  branches: Array<{ branch_code: string; branch_name: string; invoice_count: number; total_amount: number }>;
};

function buddhistDate(iso: string) {
  const parsed = new Date(`${iso}T00:00:00+07:00`);
  if (Number.isNaN(parsed.getTime())) return iso;
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "long", timeZone: "Asia/Bangkok" }).format(parsed);
}

/**
 * Today's takings, company-wide, at the top of the dashboard: one big number
 * for the day and a bar per branch so the shape of the day is legible before
 * the per-branch cards below spell it out in full.
 */
export function SalesTodayChart({ data }: { data: TodaySales }) {
  const busiest = [...data.branches].sort((a, b) => b.total_amount - a.total_amount)[0];

  return (
    <section aria-label="สรุปยอดขายรวมวันนี้" className="rounded-2xl border bg-card p-5 shadow-card sm:p-6">
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div>
          <p className="text-xs font-medium text-muted-foreground">ยอดขายรวมวันนี้ · {buddhistDate(data.date)}</p>
          <p className="mt-1 text-4xl font-semibold tracking-tight tabular-nums">{currency(data.total_amount)}</p>
          <p className="mt-1 text-sm text-muted-foreground">{data.invoice_count.toLocaleString("th-TH")} ใบขาย จาก {data.branches.length.toLocaleString("th-TH")} สาขา</p>
        </div>
        {busiest && busiest.total_amount > 0 ? (
          <div className="rounded-xl bg-primary/5 px-4 py-2.5 text-right">
            <p className="text-xs text-muted-foreground">สาขาที่ขายมากที่สุดวันนี้</p>
            <p className="mt-0.5 font-semibold">{busiest.branch_name}</p>
            <p className="text-sm tabular-nums text-primary">{currency(busiest.total_amount)}</p>
          </div>
        ) : null}
      </div>
      <div className="mt-5">
        {data.total_amount > 0 ? (
          <BarChart
            ariaLabel="กราฟยอดขายวันนี้แยกตามสาขา"
            format="currency"
            height="h-40"
            points={data.branches.map((branch) => ({
              label: branch.branch_code,
              value: branch.total_amount,
              hint: `${branch.branch_name} · ${branch.invoice_count.toLocaleString("th-TH")} ใบ`
            }))}
          />
        ) : (
          <p className="rounded-xl border border-dashed py-10 text-center text-sm text-muted-foreground">ยังไม่มีการขายในวันนี้</p>
        )}
      </div>
    </section>
  );
}
