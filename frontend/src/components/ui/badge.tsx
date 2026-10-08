import type { PropsWithChildren } from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

/**
 * One status pill for the whole app. Tones sit on the semantic tokens, whose
 * tints follow the theme on their own — no per-file colour maps.
 */
export const badgeVariants = cva(
  "inline-flex items-center rounded-md border border-transparent px-2.5 py-0.5 text-xs font-medium",
  {
    variants: {
      tone: {
        default: "bg-secondary text-secondary-foreground",
        neutral: "bg-muted text-muted-foreground",
        success: "bg-success-50 text-success-800",
        warning: "bg-warning-50 text-warning-800",
        error: "bg-error-50 text-error-800",
        info: "bg-info-50 text-info-800",
        primary: "bg-primary/10 text-primary",
        // Unread/pending counts on nav items and the bell.
        count: "min-w-5 justify-center rounded-full bg-destructive px-1 text-2xs font-bold leading-5 text-destructive-foreground"
      }
    },
    defaultVariants: { tone: "default" }
  }
);

export type BadgeTone = NonNullable<VariantProps<typeof badgeVariants>["tone"]>;

export function Badge({ className, children, tone }: PropsWithChildren<{ className?: string; tone?: BadgeTone }>) {
  return <span className={cn(badgeVariants({ tone }), className)}>{children}</span>;
}

/** A count capped at 99+, or nothing at all when there is nothing to count. */
export function CountBadge({ count, className }: { count: number; className?: string }) {
  if (count <= 0) return null;
  return <Badge className={className} tone="count">{count > 99 ? "99+" : count}</Badge>;
}
