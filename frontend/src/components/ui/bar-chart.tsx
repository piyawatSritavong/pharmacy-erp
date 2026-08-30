"use client";

import { cn } from "@/lib/utils";

const NUMBER_FORMAT = new Intl.NumberFormat("th-TH", { maximumFractionDigits: 2 });

/**
 * Hand-rolled instead of Intl's `notation: "compact"`, which is NOT stable
 * across ICU builds — Node in the container renders "฿25.0K" where Chromium
 * renders "฿25K", and that difference hydration-mismatches (React #418) on
 * any server-rendered chart.
 */
function compact(value: number) {
  const sign = value < 0 ? "-" : "";
  const size = Math.abs(value);
  if (size >= 1_000_000) return `${sign}${trim(size / 1_000_000)}M`;
  if (size >= 1_000) return `${sign}${trim(size / 1_000)}K`;
  return `${sign}${trim(size)}`;
}

/** One decimal, but only when it carries information ("25.4K", not "25.0K"). */
function trim(value: number) {
  const rounded = Math.round(value * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

export type BarChartPoint = { label: string; value: number; hint?: string };

/** Serializable rather than a formatter function, so server components can
 *  render this chart too (a function prop can't cross the RSC boundary). */
export type BarChartFormat = "compact" | "number" | "currency";

const FORMATTERS: Record<BarChartFormat, (value: number) => string> = {
  compact,
  number: (value) => NUMBER_FORMAT.format(value),
  currency: (value) => `฿${compact(value)}`
};

/**
 * Compact categorical bar chart shared by the pinned-report panels and the
 * dashboard's branch cards. Purely presentational: hand it labelled values
 * and it scales them against the largest one.
 */
export function BarChart({
  points,
  className,
  ariaLabel,
  height = "h-28",
  format = "compact"
}: {
  points: BarChartPoint[];
  className?: string;
  ariaLabel: string;
  height?: string;
  format?: BarChartFormat;
}) {
  const maximum = Math.max(...points.map((point) => point.value), 1);
  const render = FORMATTERS[format] || FORMATTERS.compact;
  if (!points.length) return null;

  return (
    <div aria-label={ariaLabel} className={cn("w-full", className)} role="img">
      {/* Columns stretch to the full track height (not items-end) so each
          bar's percentage height has something to resolve against. */}
      <div className={cn("flex items-stretch gap-1.5", height)}>
        {points.map((point) => {
          // Floor at 2% so a zero/near-zero series still reads as a bar.
          const ratio = Math.max(point.value / maximum, 0.02);
          return (
            <div className="group flex min-w-0 flex-1 flex-col justify-end" key={point.label}>
              <span className="mb-1 truncate text-center text-[10px] font-semibold tabular-nums text-muted-foreground">
                {render(point.value)}
              </span>
              <div
                className="w-full rounded-t-sm bg-primary/75 transition-colors group-hover:bg-primary"
                style={{ height: `${ratio * 100}%` }}
                title={point.hint || `${point.label}: ${render(point.value)}`}
              />
            </div>
          );
        })}
      </div>
      <div className="mt-1.5 flex gap-1.5 border-t pt-1.5">
        {points.map((point) => (
          <span className="min-w-0 flex-1 truncate text-center text-[10px] text-muted-foreground" key={point.label} title={point.label}>
            {point.label}
          </span>
        ))}
      </div>
    </div>
  );
}

export type BarChartSeries = { key: string; label: string };
export type SeriesPoint = { label: string; values: number[]; hints?: string[] };

const SERIES_COLORS = ["bg-primary/75 group-hover:bg-primary", "bg-amber-400/80 group-hover:bg-amber-500"];
const SERIES_DOTS = ["bg-primary", "bg-amber-400"];

/**
 * Grouped bar chart for a report: one cluster per X value, one bar per measure.
 *
 * Separate from BarChart above rather than an extra prop on it, because the
 * dashboard's single-measure cards depend on that component's exact layout and
 * this one needs a legend plus per-cluster grouping.
 */
export function SeriesBarChart({
  points,
  series,
  ariaLabel,
  className,
  height = "h-36",
  format = "number",
  xAxisLabel
}: {
  points: SeriesPoint[];
  series: BarChartSeries[];
  ariaLabel: string;
  className?: string;
  height?: string;
  format?: BarChartFormat;
  xAxisLabel?: string;
}) {
  if (!points.length || !series.length) return null;
  const render = FORMATTERS[format] || FORMATTERS.number;
  const maximum = Math.max(...points.flatMap((point) => point.values), 1);

  return (
    <div className={cn("w-full", className)}>
      <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1">
        {series.map((entry, index) => (
          <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground" key={entry.key}>
            <span aria-hidden className={cn("h-2 w-2 rounded-sm", SERIES_DOTS[index % SERIES_DOTS.length])} />
            {entry.label}
          </span>
        ))}
      </div>
      <div aria-label={ariaLabel} className="w-full" role="img">
        <div className={cn("flex items-stretch gap-2", height)}>
          {points.map((point) => (
            <div className="group flex min-w-0 flex-1 flex-col justify-end gap-1" key={point.label}>
              <div className="flex flex-1 items-end justify-center gap-0.5">
                {point.values.map((value, index) => (
                  <div
                    className={cn("w-full rounded-t-sm transition-colors", SERIES_COLORS[index % SERIES_COLORS.length])}
                    key={series[index]?.key || index}
                    style={{ height: `${Math.max((value / maximum) * 100, value > 0 ? 2 : 0.5)}%` }}
                    title={point.hints?.[index] || `${point.label} · ${series[index]?.label}: ${render(value)}`}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
        <div className="mt-1.5 flex gap-2 border-t pt-1.5">
          {points.map((point) => (
            <span className="min-w-0 flex-1 truncate text-center text-[10px] text-muted-foreground" key={point.label} title={point.label}>
              {point.label}
            </span>
          ))}
        </div>
      </div>
      {xAxisLabel ? <p className="mt-1 text-center text-[10px] text-muted-foreground">{xAxisLabel}</p> : null}
    </div>
  );
}
