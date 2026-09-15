"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, ChevronUp, Search } from "lucide-react";

import { usePopoverPosition } from "@/components/ui/use-popover-position";
import { cn } from "@/lib/utils";

export type MultiSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

/**
 * Shared replacement for "a row of checkboxes" filters: the trigger shows the
 * picked options as chips (capped at `maxVisibleChips`, the rest collapse into
 * a "… N" counter) and the panel holds a search box over a two-column option
 * grid. Panel is absolutely positioned inside a relative wrapper rather than
 * portaled, so it stays inside the card's own stacking/scroll context.
 */
export function MultiSelect({
  options,
  value,
  onChange,
  placeholder = "เลือกตัวเลือก",
  searchPlaceholder = "ค้นหา",
  emptyText = "ไม่พบตัวเลือก",
  selectAllLabel = "เลือกทั้งหมด",
  clearLabel = "ล้างทั้งหมด",
  maxVisibleChips = 3,
  disabled,
  className,
  label
}: {
  options: MultiSelectOption[];
  value: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  selectAllLabel?: string;
  clearLabel?: string;
  /** Chips rendered before the overflow counter takes over. */
  maxVisibleChips?: number;
  disabled?: boolean;
  className?: string;
  /** Accessible name for the trigger — the visible <Field> label. */
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const wrapperRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const listID = useId();
  const panelPosition = usePopoverPosition(open, wrapperRef, 384);

  // Close on click-away / Escape. Bound only while open so the page keeps no
  // idle listeners when every picker on it is closed.
  useEffect(() => {
    if (!open) {
      return;
    }
    function onPointerDown(event: MouseEvent | TouchEvent) {
      if (!wrapperRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("touchstart", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("touchstart", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  useEffect(() => {
    if (open) {
      searchRef.current?.focus();
    } else {
      setQuery("");
    }
  }, [open]);

  const selected = useMemo(
    () => options.filter((option) => value.includes(option.value)),
    [options, value]
  );
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle === ""
      ? options
      : options.filter((option) => option.label.toLowerCase().includes(needle));
  }, [options, query]);

  const visibleChips = selected.slice(0, maxVisibleChips);
  const hiddenCount = selected.length - visibleChips.length;
  const selectableValues = options.filter((option) => !option.disabled).map((option) => option.value);
  const allSelected = selectableValues.length > 0 && selectableValues.every((item) => value.includes(item));

  function toggle(option: MultiSelectOption) {
    if (option.disabled) {
      return;
    }
    onChange(
      value.includes(option.value)
        ? value.filter((item) => item !== option.value)
        : [...value, option.value]
    );
  }

  return (
    <div className={cn("relative", className)} ref={wrapperRef}>
      <button
        aria-controls={open ? listID : undefined}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-label={label}
        className="flex min-h-10 w-full items-center gap-2 rounded-md border border-input bg-card px-2 py-1.5 text-left text-sm text-foreground shadow-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50"
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        type="button"
      >
        <span className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
          {selected.length === 0 ? (
            <span className="px-1 text-muted-foreground">{placeholder}</span>
          ) : (
            <>
              {visibleChips.map((option) => (
                <span
                  className="inline-flex max-w-[12rem] items-center truncate rounded-full bg-primary px-2.5 py-0.5 text-xs font-medium text-primary-foreground"
                  key={option.value}
                >
                  {option.label}
                </span>
              ))}
              {hiddenCount > 0 ? (
                <>
                  <span aria-hidden className="px-0.5 text-xs text-muted-foreground">
                    ...
                  </span>
                  <span className="inline-flex h-6 min-w-6 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-semibold tabular-nums text-primary-foreground">
                    {hiddenCount}
                  </span>
                </>
              ) : null}
            </>
          )}
        </span>
        {open ? (
          <ChevronUp className="h-4 w-4 shrink-0 text-muted-foreground" />
        ) : (
          <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
      </button>

      {open ? (
        <div className="absolute left-0 right-0 z-40 overflow-y-auto overscroll-contain rounded-md border border-border bg-card p-3 shadow-md" style={{ maxHeight: panelPosition?.maxHeight, ...(panelPosition?.above ? { bottom: "calc(100% + 0.375rem)" } : { top: "calc(100% + 0.375rem)" }) }}>
          <div className="relative">
            <input
              className="h-9 w-full rounded-md border border-input bg-card pl-3 pr-9 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/40"
              onChange={(event) => setQuery(event.target.value)}
              placeholder={searchPlaceholder}
              ref={searchRef}
              // text, not search: WebKit's native clear "✕" would collide
              // with the magnifier sitting in the same right-hand slot.
              type="text"
              value={query}
            />
            <Search className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          </div>

          <div className="mt-2 flex items-center justify-between gap-3 text-xs">
            <button
              className="font-medium text-primary disabled:opacity-40"
              disabled={allSelected}
              onClick={() => onChange(selectableValues)}
              type="button"
            >
              {selectAllLabel}
            </button>
            <button
              className="font-medium text-muted-foreground disabled:opacity-40"
              disabled={selected.length === 0}
              onClick={() => onChange([])}
              type="button"
            >
              {clearLabel}
            </button>
          </div>

          <div
            aria-multiselectable
            className="mt-2 grid max-h-64 grid-cols-1 gap-x-4 gap-y-1 overflow-y-auto overscroll-contain sm:grid-cols-2"
            id={listID}
            role="listbox"
          >
            {filtered.length === 0 ? (
              <p className="col-span-full py-4 text-center text-sm text-muted-foreground">{emptyText}</p>
            ) : (
              filtered.map((option) => {
                const checked = value.includes(option.value);
                return (
                  <button
                    aria-selected={checked}
                    className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-foreground outline-none transition-colors hover:bg-muted focus-visible:bg-muted disabled:cursor-not-allowed disabled:opacity-40"
                    disabled={option.disabled}
                    key={option.value}
                    onClick={() => toggle(option)}
                    role="option"
                    type="button"
                  >
                    <span
                      className={cn(
                        "grid h-4 w-4 shrink-0 place-items-center rounded-sm border transition-colors",
                        checked ? "border-primary bg-primary text-primary-foreground" : "border-input bg-card"
                      )}
                    >
                      {checked ? <Check className="h-3 w-3" /> : null}
                    </span>
                    <span className="truncate">{option.label}</span>
                  </button>
                );
              })
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
