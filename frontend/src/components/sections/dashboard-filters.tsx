"use client";

import { useRouter, useSearchParams } from "next/navigation";

import { Field } from "@/components/ui/field";
import { Button, Input, Select } from "@/components/ui/primitives";

/**
 * The scope every figure on the dashboard is computed over. It exists because
 * two screens quietly disagreed: the dashboard counted every issued bill for all
 * time, while สรุปสิ้นเดือน counted only bills that were actually paid, inside a
 * period — so the same branch read ฿230,909.44 on one page and ฿225,748.62 on
 * the other. Rather than pick one definition and hide the other, the operator
 * chooses, and the choice applies to every number on the page at once.
 */
export function DashboardFilters() {
  const router = useRouter();
  const params = useSearchParams();
  const dateFrom = params.get("date_from") || "";
  const dateTo = params.get("date_to") || "";
  const paymentStatus = params.get("payment_status") || "";

  function apply(next: Record<string, string>) {
    const query = new URLSearchParams(params.toString());
    for (const [key, value] of Object.entries(next)) {
      if (value) query.set(key, value);
      else query.delete(key);
    }
    router.push(query.size ? `/dashboard?${query.toString()}` : "/dashboard");
  }

  const narrowed = Boolean(dateFrom || dateTo || paymentStatus);

  return (
    <div className="flex flex-wrap items-end gap-3 rounded-2xl border bg-card p-4 shadow-card">
      <Field className="w-44" label="ตั้งแต่วันที่">
        <Input aria-label="ยอดขายตั้งแต่วันที่" onChange={(event) => apply({ date_from: event.target.value })} type="date" value={dateFrom} />
      </Field>
      <Field className="w-44" label="ถึงวันที่">
        <Input aria-label="ยอดขายถึงวันที่" onChange={(event) => apply({ date_to: event.target.value })} type="date" value={dateTo} />
      </Field>
      <Field
        className="w-56"
        hint={paymentStatus === "paid" ? "ตรงกับนิยามของหน้าสรุปสิ้นเดือน" : "รวมบิลที่ยังเก็บเงินไม่ได้"}
        label="สถานะชำระเงิน"
      >
        <Select aria-label="กรองตามสถานะชำระเงิน" onChange={(event) => apply({ payment_status: event.target.value })} value={paymentStatus}>
          <option value="">ทุกใบที่ออกบิล</option>
          <option value="paid">เฉพาะที่ชำระแล้ว</option>
          <option value="unpaid">เฉพาะที่ค้างชำระ</option>
        </Select>
      </Field>
      {narrowed ? (
        <Button className="mb-0.5" onClick={() => router.push("/dashboard")} type="button" variant="secondary">
          ล้างตัวกรอง
        </Button>
      ) : null}
    </div>
  );
}
