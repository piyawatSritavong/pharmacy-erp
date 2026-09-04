import Link from "next/link";
import { ArrowRight, EyeOff, Tag } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { currency } from "@/lib/utils";

type Branch = Record<string, unknown>;
type Comparison = {
  before_invoice_count: number;
  before_amount: number;
  after_invoice_count: number;
  after_amount: number;
  hidden_invoice_count: number;
  hidden_amount: number;
  repriced_reduction: number;
  branches: Branch[];
};

function num(value: unknown) {
  const parsed = Number(value || 0);
  return Number.isFinite(parsed) ? parsed : 0;
}
function count(value: unknown) {
  return num(value).toLocaleString("th-TH");
}

/**
 * Superadmin only: the period as it was before the month-end close, beside what
 * it is now. A close is destructive — it soft-deletes the bills Ghost Stock
 * covered and rewrites the totals of the ones it reprices — so without this the
 * dashboard silently shows the adjusted books and nothing says so.
 *
 * admin.central never sees this block; they work from the adjusted books alone.
 */
export function CloseComparison({ data, reportHref }: { data: Comparison; reportHref: string }) {
  const difference = num(data.before_amount) - num(data.after_amount);
  const untouched = num(data.before_amount) - num(data.hidden_amount) - num(data.repriced_reduction);

  return (
    <SectionCard
      actions={
        <Link className="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition hover:bg-muted" href={reportHref}>
          ดูรายละเอียดรายบิล
          <ArrowRight className="h-3.5 w-3.5" />
        </Link>
      }
      description="ยอดก่อนปิดรอบเทียบกับยอดปัจจุบัน · เห็นเฉพาะผู้ดูแลระบบสูงสุด"
      title="ยอดก่อนปรับ / หลังปรับ"
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
        <div className="rounded-2xl border bg-muted/40 p-5">
          <p className="text-xs font-medium text-muted-foreground">ยอดก่อนปรับ</p>
          <p className="mt-1 text-3xl font-bold tabular-nums">{currency(num(data.before_amount))}</p>
          <p className="mt-1 text-xs text-muted-foreground">{count(data.before_invoice_count)} ใบ · รวมบิลที่ถูกซ่อนไปแล้ว</p>
        </div>
        <div className="hidden place-items-center lg:grid">
          <ArrowRight className="h-6 w-6 text-muted-foreground" />
        </div>
        <div className="rounded-2xl border border-primary/30 bg-primary/5 p-5">
          <p className="text-xs font-medium text-primary">ยอดหลังปรับ (ที่ทุกคนเห็น)</p>
          <p className="mt-1 text-3xl font-bold tabular-nums">{currency(num(data.after_amount))}</p>
          <p className="mt-1 text-xs text-muted-foreground">{count(data.after_invoice_count)} ใบ · ยอดที่ admin.central เห็น</p>
        </div>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <div className="rounded-xl border p-4">
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><EyeOff className="h-3.5 w-3.5" />บิลที่ถูกซ่อน</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{currency(num(data.hidden_amount))}</p>
          <p className="text-xs text-muted-foreground">{count(data.hidden_invoice_count)} ใบ · มีในสต๊อกผี ส่งคืน WH แล้วตัด Ghost</p>
        </div>
        <div className="rounded-xl border p-4">
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><Tag className="h-3.5 w-3.5" />ส่วนที่ปรับราคาลง</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{currency(num(data.repriced_reduction))}</p>
          <p className="text-xs text-muted-foreground">บิลที่ยังอยู่ แต่บันทึกใหม่ที่ต้นทุน + %</p>
        </div>
        <div className="rounded-xl border p-4">
          <p className="text-xs text-muted-foreground">ผลต่างรวม</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{currency(difference)}</p>
          <p className="text-xs text-muted-foreground">บิลที่ไม่ถูกแตะ {currency(untouched)}</p>
        </div>
      </div>

      <div className="mt-5 overflow-x-auto rounded-xl border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2.5 text-left font-medium">สาขา</th>
              <th className="px-3 py-2.5 text-right font-medium">ใบก่อน</th>
              <th className="px-3 py-2.5 text-right font-medium">ยอดก่อนปรับ</th>
              <th className="px-3 py-2.5 text-right font-medium">ใบหลัง</th>
              <th className="px-3 py-2.5 text-right font-medium">ยอดหลังปรับ</th>
              <th className="px-3 py-2.5 text-right font-medium">ซ่อน</th>
              <th className="px-3 py-2.5 text-right font-medium">ปรับราคาลง</th>
              <th className="px-3 py-2.5 text-right font-medium" />
            </tr>
          </thead>
          <tbody className="divide-y">
            {data.branches.map((branch) => {
              const changed = num(branch.hidden_amount) > 0 || num(branch.repriced_reduction) > 0;
              return (
                <tr className="hover:bg-muted/40" key={String(branch.branch_id)}>
                  <td className="px-3 py-2.5 font-medium">{String(branch.branch_name)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums">{count(branch.before_invoice_count)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums">{currency(num(branch.before_amount))}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums">{count(branch.after_invoice_count)}</td>
                  <td className="px-3 py-2.5 text-right font-semibold tabular-nums">{currency(num(branch.after_amount))}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-red-700">
                    {num(branch.hidden_amount) > 0 ? currency(num(branch.hidden_amount)) : "—"}
                  </td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-amber-700">
                    {num(branch.repriced_reduction) > 0 ? currency(num(branch.repriced_reduction)) : "—"}
                  </td>
                  <td className="px-3 py-2.5 text-right">
                    {changed ? (
                      <Link className="text-xs font-semibold text-primary underline underline-offset-2" href={reportHref}>
                        ดูบิล
                      </Link>
                    ) : null}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </SectionCard>
  );
}
