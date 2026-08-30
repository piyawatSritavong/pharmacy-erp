import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

export function Field({
  label,
  hint,
  error,
  children,
  className,
  labelClassName
}: {
  label: string;
  hint?: string;
  /** Shared validation message (see @/lib/validation) — same wording everywhere a field is reused. */
  error?: string;
  children: ReactNode;
  className?: string;
  /** Override the label's default text-foreground — e.g. a light color when Field sits on a dark card (see D4's summary panel). */
  labelClassName?: string;
}) {
  return (
    <label className={cn("block space-y-2", className)}>
      <span className={cn("flex items-baseline justify-between gap-3 text-sm font-bold text-foreground", labelClassName)}>
        {label}
        {hint ? <span className="text-xs font-normal text-muted-foreground">{hint}</span> : null}
      </span>
      <span className={cn("block", error && "[&_input]:border-error [&_select]:border-error [&_textarea]:border-error [&_button]:border-error")}>
        {children}
      </span>
      {error ? (
        <span className="block text-xs font-semibold text-error" role="alert">
          {error}
        </span>
      ) : null}
    </label>
  );
}
