// No "use client": Button has no hooks, and keeping it a shared module lets
// server components (not-found pages, daily-sales) call buttonVariants().
import type { ButtonHTMLAttributes } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";

import { cn } from "@/lib/utils";

/** Exported so non-<button> elements (e.g. a Next <Link>) can wear the same
 *  styling instead of hand-copying the classes. */
export const buttonVariants = cva(
  "ui-button inline-flex h-10 max-w-full items-center justify-center gap-2 whitespace-normal rounded-md px-3 text-center text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-50 sm:whitespace-nowrap sm:px-4 [&>svg]:shrink-0",
  {
    variants: {
      // Hover/active darken by opacity, not by stepping to a fixed -600/-700
      // shade: the fill is a theme token (lighter in dark mode) and a fixed
      // shade would put its dark foreground on a dark fill.
      variant: {
        default: "bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 active:bg-primary/80",
        secondary: "border border-input bg-card text-foreground shadow-sm hover:bg-muted active:bg-muted/70",
        ghost: "bg-transparent text-foreground hover:bg-muted active:bg-muted/70",
        destructive: "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 active:bg-destructive/80",
        success: "bg-success text-success-foreground shadow-sm hover:bg-success/90 active:bg-success/80",
        warning: "bg-warning text-warning-foreground shadow-sm hover:bg-warning/90 active:bg-warning/80",
        info: "bg-info text-info-foreground shadow-sm hover:bg-info/90 active:bg-info/80"
      },
      size: {
        default: "",
        sm: "h-9 px-3",
        // The till's large touch controls.
        lg: "h-12 px-5 text-base",
        icon: "h-10 w-10 px-0 sm:px-0"
      },
      shape: {
        default: "",
        pill: "rounded-full"
      }
    },
    defaultVariants: {
      variant: "default",
      size: "default",
      shape: "default"
    }
  }
);

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonVariants> & {
  /** While an action is in flight: disables the button (no double submit),
   *  shows a spinner and, if given, swaps the label for loadingText. */
  loading?: boolean;
  loadingText?: string;
};

export function Button({ className, variant, size, shape, loading = false, loadingText, disabled, children, ...props }: ButtonProps) {
  return (
    <button
      aria-busy={loading || undefined}
      className={cn(buttonVariants({ variant, size, shape }), className)}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <Loader2 aria-hidden className="h-4 w-4 animate-spin" /> : null}
      {loading && loadingText ? loadingText : children}
    </button>
  );
}
