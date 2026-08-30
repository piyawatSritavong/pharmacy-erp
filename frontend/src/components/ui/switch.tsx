"use client";

import { useState } from "react";

import { cn } from "@/lib/utils";

/**
 * Toggle switch (D10: replaces the plain "เปิดใช้งาน" checkbox). Uncontrolled
 * like the existing Checkbox usage across settings-console.tsx (defaultChecked,
 * self-managed) — a hidden native checkbox mirrors the visual state so it
 * still submits through the surrounding <form>'s FormData the same way
 * `data.get("active") === "on"` already expects.
 */
export function Switch({
  defaultChecked = false,
  disabled,
  name,
  className,
  "aria-label": ariaLabel
}: {
  defaultChecked?: boolean;
  disabled?: boolean;
  name?: string;
  className?: string;
  "aria-label"?: string;
}) {
  const [checked, setChecked] = useState(defaultChecked);
  return (
    <span className="inline-flex items-center">
      <button
        aria-checked={checked}
        aria-label={ariaLabel}
        className={cn(
          "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50",
          checked ? "bg-primary" : "bg-neutral-300",
          className
        )}
        disabled={disabled}
        onClick={() => setChecked((current) => !current)}
        role="switch"
        type="button"
      >
        <span
          aria-hidden="true"
          className={cn(
            "inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform",
            checked ? "translate-x-5" : "translate-x-0"
          )}
        />
      </button>
      {name ? <input checked={checked} className="sr-only" name={name} onChange={() => {}} readOnly tabIndex={-1} type="checkbox" /> : null}
    </span>
  );
}
