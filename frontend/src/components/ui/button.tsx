"use client";

import type { ButtonHTMLAttributes } from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

/** Exported so non-<button> elements (e.g. a Next <Link>) can wear the same
 *  styling instead of hand-copying the classes. */
export const buttonVariants = cva(
  "ui-button inline-flex h-10 max-w-full items-center justify-center gap-2 whitespace-normal rounded-md px-3 text-center text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-50 sm:whitespace-nowrap sm:px-4 [&>svg]:shrink-0",
  {
    variants: {
      // Hover = one shade darker, active = two shades darker, per the
      // centralized theme scale (tailwind.config.ts `primary`/`neutral`/
      // semantic 50-900 scales) — one definition, used everywhere.
      variant: {
        default: "bg-primary text-primary-foreground shadow-sm hover:bg-primary-600 active:bg-primary-700",
        secondary: "border border-input bg-card text-foreground shadow-sm hover:bg-neutral-100 active:bg-neutral-200",
        ghost: "bg-transparent text-foreground hover:bg-neutral-100 active:bg-neutral-200",
        destructive: "bg-destructive text-destructive-foreground shadow-sm hover:bg-error-700 active:bg-error-800",
        success: "bg-success text-success-foreground shadow-sm hover:bg-success-700 active:bg-success-800",
        warning: "bg-warning text-warning-foreground shadow-sm hover:bg-warning-700 active:bg-warning-800",
        info: "bg-info text-info-foreground shadow-sm hover:bg-info-700 active:bg-info-800"
      }
    },
    defaultVariants: {
      variant: "default"
    }
  }
);

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonVariants>;

export function Button({ className, variant, ...props }: ButtonProps) {
  return (
    <button
      className={cn(buttonVariants({ variant }), className)}
      {...props}
    />
  );
}
