"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import type { PropsWithChildren, ReactNode } from "react";

import { cn } from "@/lib/utils";

export function Sheet({
  open,
  onOpenChange,
  children
}: PropsWithChildren<{ open: boolean; onOpenChange?: (open: boolean) => void }>) {
  return <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>{children}</DialogPrimitive.Root>;
}

export function SheetTrigger({ children }: PropsWithChildren) {
  return <DialogPrimitive.Trigger asChild>{children}</DialogPrimitive.Trigger>;
}

export function SheetContent({
  className,
  side = "right",
  children
}: PropsWithChildren<{ className?: string; side?: "left" | "right" }>) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/30" />
      <DialogPrimitive.Content
        className={cn(
          "app-sheet fixed inset-y-0 z-50 flex h-dvh max-h-dvh w-[calc(100%-1rem)] max-w-xl flex-col overflow-y-auto overscroll-contain border bg-card p-4 shadow-2xl sm:w-full sm:p-6",
          side === "left" ? "left-0" : "right-0",
          className
        )}
      >
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}

export function SheetHeader({
  title,
  description,
  actions
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="sticky -top-4 z-10 mb-4 flex shrink-0 items-start justify-between gap-2 bg-card pb-3 sm:-top-6">
      <div className="min-w-0">
        <DialogPrimitive.Title className="text-lg font-semibold text-black">{title}</DialogPrimitive.Title>
        {description ? (
          <DialogPrimitive.Description className="mt-1 text-sm text-muted-foreground">
            {description}
          </DialogPrimitive.Description>
        ) : null}
      </div>
      {actions}
      <DialogPrimitive.Close aria-label="ปิดเมนู" className="grid h-11 w-11 shrink-0 place-items-center rounded-lg hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" type="button">
        <X className="h-5 w-5" />
      </DialogPrimitive.Close>
    </div>
  );
}
