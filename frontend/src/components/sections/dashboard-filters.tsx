"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";
import { useRouter, useSearchParams } from "next/navigation";
import { ChevronLeft, ChevronRight, SlidersHorizontal } from "lucide-react";

import { ReconciliationRoundPicker } from "@/components/sections/reconciliation-round-picker";
import { Field } from "@/components/ui/field";
import { Button, Input, Select } from "@/components/ui/primitives";

const BANGKOK = "Asia/Bangkok";

/** Today where the shops are, not where the browser is. */
export function bangkokToday() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: BANGKOK }).format(new Date());
}

/** 2026-09-06 → 6 กันยายน 2569. */
export function thaiFullDate(iso: string) {
  const parsed = new Date(`${iso}T00:00:00+07:00`);
  if (Number.isNaN(parsed.getTime())) return iso;
  return new Intl.DateTimeFormat("th-TH", {
    timeZone: BANGKOK,
    day: "numeric",
    month: "long",
    year: "numeric"
  }).format(parsed);
}

function shiftDay(iso: string, days: number) {
  const parsed = new Date(`${iso}T00:00:00+07:00`);
  parsed.setUTCDate(parsed.getUTCDate() + days);
  return new Intl.DateTimeFormat("en-CA", { timeZone: BANGKOK }).format(parsed);
}

/**
 * The scope every figure on the dashboard is computed over. Default is today:
 * the page answers "how is trading going right now", and only widens when the
 * operator asks it to.
 *
 * The right-hand pair steps a day at a time, because that is how the day's
 * takings are actually reviewed — yesterday against today — and typing two
 * dates to move one day back is friction the screen does not need.
 */
export function DashboardFilters({ canSeeClose }: { canSeeClose: boolean }) {
  const [filtersOpen, setFiltersOpen] = useState(false);
  const router = useRouter();
  const params = useSearchParams();
  const today = bangkokToday();
  const dateFrom = params.get("date_from") || today;
  const dateTo = params.get("date_to") || today;
  const closeStatus = params.get("close_status") || "";
  const paymentType = params.get("payment_type") || "";
  const paymentStatus = params.get("payment_status") || "";
  const roundId = params.get("round_id") || "";
  const roundLabel = params.get("round_label") || "";

  function apply(next: Record<string, string>) {
    const query = new URLSearchParams(params.toString());
    for (const [key, value] of Object.entries(next)) {
      if (value) query.set(key, value);
      else query.delete(key);
    }
    router.push(query.size ? `/dashboard?${query.toString()}` : "/dashboard");
  }

  // Stepping moves the whole window, so a week stays a week as it walks back.
  function step(days: number) {
    const spanDays = Math.round(
      (new Date(`${dateTo}T00:00:00+07:00`).getTime() - new Date(`${dateFrom}T00:00:00+07:00`).getTime()) / 86_400_000
    );
    const nextFrom = shiftDay(dateFrom, days * (spanDays + 1 || 1));
    apply({ date_from: nextFrom, date_to: shiftDay(nextFrom, spanDays) });
  }

  const singleDay = dateFrom === dateTo;
  const isToday = singleDay && dateFrom === today;

  return (
    <div className="rounded-xl border bg-card p-3 shadow-card sm:rounded-2xl sm:p-4">
      <div className="flex flex-col gap-3 sm:gap-5 xl:flex-row xl:items-start xl:justify-between">
        <Button aria-controls="dashboard-filter-fields" aria-expanded={filtersOpen} className="w-full justify-between sm:hidden" onClick={() => setFiltersOpen((value) => !value)} type="button" variant="secondary">
          <span className="flex items-center gap-2"><SlidersHorizontal className="h-4 w-4" />ตัวกรองและช่วงวันที่</span>
          <span className="text-xs text-muted-foreground">{[closeStatus, paymentType, paymentStatus, roundId].filter(Boolean).length || "ทั้งหมด"}</span>
        </Button>
        <div className={cn("min-w-0 flex-wrap items-end gap-3 sm:flex", filtersOpen ? "flex" : "hidden")} id="dashboard-filter-fields">
          <Field className="w-full min-[390px]:w-[calc(50%-0.375rem)] sm:w-40" label="วันเริ่มต้น">
            <Input
              aria-label="ยอดขายตั้งแต่วันที่"
              onChange={(event) => apply({ date_from: event.target.value })}
              type="date"
              value={dateFrom}
            />
          </Field>
          <Field className="w-full min-[390px]:w-[calc(50%-0.375rem)] sm:w-40" label="วันสิ้นสุด">
            <Input
              aria-label="ยอดขายถึงวันที่"
              onChange={(event) => apply({ date_to: event.target.value })}
              type="date"
              value={dateTo}
            />
          </Field>
          {canSeeClose ? (
            <Field className="w-full sm:w-44" label="สถานะ">
              <Select
                aria-label="กรองตามสถานะการปิดรอบ"
                onChange={(event) => apply({ close_status: event.target.value })}
                value={closeStatus}
              >
                <option value="">ทุกสถานะ</option>
                <option value="active">Active — ไม่ถูกแตะ</option>
                <option value="adjusted">Adjusted — ปรับราคา</option>
                <option value="hidden">Hidden — ถูกซ่อน</option>
              </Select>
            </Field>
          ) : null}
          <Field className="w-full sm:w-40" label="ประเภทชำระเงิน">
            <Select
              aria-label="กรองตามประเภทชำระเงิน"
              onChange={(event) => apply({ payment_type: event.target.value })}
              value={paymentType}
            >
              <option value="">ทุกประเภท</option>
              <option value="cash">เงินสด</option>
              <option value="bank_transfer">เงินโอน</option>
              <option value="mixed">เงินสด + โอน ผสม</option>
            </Select>
          </Field>
          <Field className="w-full sm:w-40" label="สถานะชำระเงิน">
            <Select
              aria-label="กรองตามสถานะชำระเงิน"
              onChange={(event) => apply({ payment_status: event.target.value })}
              value={paymentStatus}
            >
              <option value="">ทุกใบที่ออกบิล</option>
              <option value="paid">ชำระแล้ว</option>
              <option value="unpaid">ค้างชำระ</option>
            </Select>
          </Field>
          {canSeeClose ? (
            <Field className="w-full sm:w-64" label="รอบสรุปสิ้นเดือน">
              <ReconciliationRoundPicker
                label={roundLabel}
                onChange={(id, round) =>
                  apply(
                    round
                      ? {
                          round_id: id,
                          round_label: round.reconciliation_number,
                          date_from: round.period_start,
                          date_to: round.period_end
                        }
                      : { round_id: "", round_label: "" }
                  )
                }
                value={roundId}
              />
            </Field>
          ) : null}
        </div>

        <div className="flex min-w-0 shrink-0 flex-wrap items-center justify-end gap-2">
          <Button aria-label="วันก่อนหน้า" onClick={() => step(-1)} type="button" variant="secondary">
            <ChevronLeft className="h-4 w-4" />
            <span className="hidden sm:inline">วันก่อนหน้า</span>
          </Button>
          <div className="min-w-0 flex-1 rounded-xl border bg-muted/50 px-2 sm:px-4 py-2 text-center sm:min-w-[13rem] sm:flex-none">
            <p className="text-sm font-semibold leading-snug sm:text-base">
              {singleDay ? thaiFullDate(dateFrom) : `${thaiFullDate(dateFrom)} – ${thaiFullDate(dateTo)}`}
            </p>
            <p className="text-xs text-muted-foreground">{isToday ? "วันนี้ · อัปเดตสด" : "ย้อนหลัง"}</p>
          </div>
          <Button aria-label="วันถัดไป" onClick={() => step(1)} type="button" variant="secondary">
            <span className="hidden sm:inline">วันถัดไป</span>
            <ChevronRight className="h-4 w-4" />
          </Button>
          {!isToday ? (
            <Button onClick={() => apply({ date_from: today, date_to: today, round_id: "", round_label: "" })} type="button">
              กลับมาวันนี้
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
