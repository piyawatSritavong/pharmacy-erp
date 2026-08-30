"use client";

/**
 * FEATURE: chart visualisation.
 * Owns the timeline only — bucket granularity and the time range are chosen in
 * config-steps.tsx and arrive here already applied to `result`.
 */

import { CircleAlert } from "lucide-react";

import type { ReportDataset, ReportDefinition, ReportResult } from "@/types/report-builder";
import { cn } from "@/lib/utils";
import { CURRENCY_FORMAT, DATE_TIME_FORMAT, NUMBER_FORMAT, fieldLabel, timelineTickLabel } from "./shared";

export function Timeline({ result, definition, dataset, compact = false }: {
  result?: ReportResult | null;
  definition?: ReportDefinition;
  dataset?: ReportDataset;
  compact?: boolean;
}) {
  const points = result?.timeline || [];
  const maximum = Math.max(...points.map((point) => point.count), 1);
  const bucket = result?.meta.time_bucket;
  const bucketLabels: Record<string, string> = { hour: "รายชั่วโมง", day: "รายวัน", week: "รายสัปดาห์", month: "รายเดือน" };
  const timeField = definition?.time_config?.field;
  const xAxisLabel = timeField ? `${fieldLabel(dataset, timeField)}${bucket ? ` (${bucketLabels[bucket] || bucket})` : ""}` : "เวลา";
  const tickIndexes = Array.from(new Set([0, Math.floor((points.length - 1) / 2), points.length - 1])).filter((index) => index >= 0);
  return (
    <div
      aria-label={`กราฟเปรียบเทียบ ${xAxisLabel} กับ จำนวนรายการ`}
      className={cn("relative overflow-hidden border-y bg-[#fffaf3]", compact ? "h-32" : "h-52")}
      role="img"
    >
      {points.length ? (
        <>
          <div className={cn("absolute font-semibold text-muted-foreground", compact ? "bottom-10 left-14 right-3 top-7" : "bottom-14 left-[4.5rem] right-6 top-10")}>
            {[0, 0.5, 1].map((ratio) => (
              <div className="absolute left-0 right-0 border-t border-dashed border-black/10" key={ratio} style={{ bottom: `${ratio * 100}%` }}>
                <span className="absolute right-full top-0 -translate-y-1/2 pr-2 text-[9px] tabular-nums">{NUMBER_FORMAT.format(Math.round(maximum * ratio))}</span>
              </div>
            ))}
            <div className={cn("absolute bottom-0 flex items-end gap-px", compact ? "left-1 right-1 top-1" : "left-3 right-3 top-3")}>
              {points.map((point) => (
                <div
                  className="group relative min-w-[4px] flex-1 rounded-t-sm bg-primary/75 transition hover:bg-primary"
                  key={point.bucket}
                  style={{ height: `${Math.max(4, (point.count / maximum) * 100)}%` }}
                  title={`${DATE_TIME_FORMAT.format(new Date(point.bucket))}: ${NUMBER_FORMAT.format(point.count)} รายการ`}
                >
                  <span className="sr-only">{point.bucket}: {point.count}</span>
                </div>
              ))}
              {tickIndexes.map((index) => (
                <span
                  className={cn("absolute top-full mt-1 whitespace-nowrap text-[9px] font-medium text-muted-foreground", points.length === 1 ? "-translate-x-1/2" : index === 0 ? "translate-x-0" : index === points.length - 1 ? "-translate-x-full" : "-translate-x-1/2")}
                  key={points[index].bucket}
                  style={{ left: points.length === 1 ? "50%" : `${(index / (points.length - 1)) * 100}%` }}
                >
                  {timelineTickLabel(points[index].bucket, bucket)}
                </span>
              ))}
            </div>
          </div>
          <span className={cn("absolute flex items-center justify-center text-[9px] font-bold text-muted-foreground", compact ? "bottom-10 left-1 top-7 pb-2" : "bottom-14 left-2 top-10 pb-4")}><span className="rotate-180 whitespace-nowrap [writing-mode:vertical-rl]">แกน Y: จำนวนรายการ</span></span>
          <span className={cn("absolute bottom-2 left-1/2 -translate-x-1/2 whitespace-nowrap text-[9px] font-bold text-muted-foreground", compact && "bottom-1 text-[8px]")}>แกน X: {xAxisLabel}</span>
        </>
      ) : (
        <div className="grid h-full place-items-center text-sm text-muted-foreground">
          {result ? "ไม่มีเหตุการณ์ในช่วงเวลาที่เลือก" : "ระบบกำลังโหลด Timeline โดยอัตโนมัติ"}
        </div>
      )}
      {!compact && result ? (
        <div className="absolute left-3 top-2 flex gap-3 text-[11px] font-semibold text-muted-foreground">
          <span>{result.meta.time_bucket ? `Bucket: ${result.meta.time_bucket}` : "ไม่ใช้ Timeline"}</span>
          <span>{NUMBER_FORMAT.format(result.pagination.total)} แถว</span>
        </div>
      ) : null}
    </div>
  );
}

