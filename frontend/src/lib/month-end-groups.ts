/** Grouping helpers for month-end rounds.
 *
 *  A round is chosen by date range, so one month can hold several rounds and a
 *  list spans years. Screens group by round and label the group with its Thai
 *  month and Buddhist-era year.
 */
const MONTHS = [
  "มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน",
  "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"
];

/** "2026-09-03" → "2026-09"; anything unparseable groups on its own. */
export function periodKey(isoDate: string) {
  return (isoDate || "").slice(0, 7);
}

/** "2026-09" or "2026-09-03" → "กันยายน 2569" */
export function periodLabel(isoDate: string) {
  const [year, month] = (isoDate || "").split("-");
  const index = Number(month) - 1;
  if (!year || Number.isNaN(index) || index < 0 || index > 11) return isoDate || "ไม่ระบุช่วงเวลา";
  return `${MONTHS[index]} ${Number(year) + 543}`;
}

/** "2026-09-03" → "3 ก.ย. 2569" */
export function shortDate(isoDate: string) {
  if (!isoDate) return "-";
  const parsed = new Date(`${isoDate}T00:00:00+07:00`);
  if (Number.isNaN(parsed.getTime())) return isoDate;
  return new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short", year: "numeric", timeZone: "Asia/Bangkok" }).format(parsed);
}

/** Rounds newest first, so the freshest close sits at the top of a long list. */
export function byPeriodDesc<T extends { period_start: string; period_end: string }>(items: T[]) {
  return [...items].sort((a, b) => (b.period_start || "").localeCompare(a.period_start || "") || (b.period_end || "").localeCompare(a.period_end || ""));
}

/** Groups rounds into one bucket per calendar month, newest month first. */
export function groupByPeriod<T extends { period_start: string; period_end: string }>(items: T[]) {
  const buckets = new Map<string, T[]>();
  for (const item of byPeriodDesc(items)) {
    const key = periodKey(item.period_start);
    if (!buckets.has(key)) buckets.set(key, []);
    buckets.get(key)!.push(item);
  }
  return [...buckets.entries()].map(([key, rounds]) => ({ key, label: periodLabel(key), rounds }));
}
