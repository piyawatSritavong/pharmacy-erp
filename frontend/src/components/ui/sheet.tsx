"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import type { PropsWithChildren, ReactNode } from "react";

import { cn } from "@/lib/utils";

export function Sheet({
  open,
  children
}: PropsWithChildren<{ open: boolean }>) {
  return <DialogPrimitive.Root open={open}>{children}</DialogPrimitive.Root>;
}

export function SheetContent({
  className,
  children
}: PropsWithChildren<{ className?: string }>) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/30" />
      <DialogPrimitive.Content
        className={cn(
          "fixed inset-y-0 right-0 z-50 h-full w-full max-w-xl border-l border-black/10 bg-white p-6 shadow-2xl",
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
    <div className="mb-4 flex items-start justify-between gap-4">
      <div>
        <DialogPrimitive.Title className="text-lg font-semibold text-black">{title}</DialogPrimitive.Title>
        {description ? (
          <DialogPrimitive.Description className="mt-1 text-sm text-black/55">
            {description}
          </DialogPrimitive.Description>
        ) : null}
      </div>
      {actions}
    </div>
  );
}
