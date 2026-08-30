import type { InputHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={cn(
        // text-foreground is explicit, not inherited: otherwise this input
        // silently takes on whatever text color its parent wrapper set (e.g.
        // white-on-white inside a dark summary card — see D4's "invisible
        // text" bug) instead of staying readable against its own bg-card.
        "flex h-10 w-full rounded-md border border-input bg-card px-3 py-2 text-sm text-foreground shadow-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50",
        props.className
      )}
    />
  );
}
