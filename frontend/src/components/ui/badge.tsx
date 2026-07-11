import type { PropsWithChildren } from "react";

import { cn } from "@/lib/utils";

export function Badge({ className, children }: PropsWithChildren<{ className?: string }>) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md border border-transparent bg-secondary px-2.5 py-0.5 text-xs font-medium text-secondary-foreground",
        className
      )}
    >
      {children}
    </span>
  );
}
