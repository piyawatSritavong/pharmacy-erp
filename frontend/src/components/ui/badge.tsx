import type { PropsWithChildren } from "react";

import { cn } from "@/lib/utils";

export function Badge({ className, children }: PropsWithChildren<{ className?: string }>) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border border-black/10 bg-[#f5f5f4] px-3 py-1 text-[11px] font-medium uppercase tracking-[0.18em] text-black/70",
        className
      )}
    >
      {children}
    </span>
  );
}
