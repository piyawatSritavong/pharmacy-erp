"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { createContext, useContext, useEffect, useRef, type PropsWithChildren, type ReactNode, type RefObject } from "react";

import { cn } from "@/lib/utils";

const ReturnFocusContext = createContext<RefObject<HTMLElement | null> | null>(null);

export function Dialog({
  open,
  onOpenChange,
  children
}: PropsWithChildren<{ open: boolean; onOpenChange?: (open: boolean) => void }>) {
  const wasOpen = useRef(false);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (open) return;
    // Safari does not focus a button when it is tapped. Remember the actual
    // opener without changing native focus behavior for other controls.
    const rememberPointer = (event: PointerEvent) => {
      const target = event.target instanceof Element ? event.target.closest<HTMLElement>("button, a[href], [role='button']") : null;
      if (target) opener.current = target;
    };
    document.addEventListener("pointerdown", rememberPointer, true);
    return () => document.removeEventListener("pointerdown", rememberPointer, true);
  }, [open]);
  // Capture before mounting children: a form's autoFocus input may take focus
  // before Radix dispatches onOpenAutoFocus. It is not the return destination.
  if (open && !wasOpen.current && typeof document !== "undefined") {
    if (document.activeElement !== document.body) opener.current = document.activeElement as HTMLElement;
  }
  wasOpen.current = open;
  return <ReturnFocusContext.Provider value={opener}><DialogPrimitive.Root onOpenChange={onOpenChange} open={open}>{children}</DialogPrimitive.Root></ReturnFocusContext.Provider>;
}

export function DialogContent({
  className,
  children
}: PropsWithChildren<{ className?: string }>) {
  const returnFocus = useRef<HTMLElement | null>(null);
  const opener = useContext(ReturnFocusContext);
  return (
    <DialogPrimitive.Portal>
      {/* print:hidden — a modal backdrop should never appear in printed/exported output (see D4's PO print layout). */}
      <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/40 print:hidden" />
      {/* Centred on both axes and capped to the viewport, so a tall form
          scrolls inside the dialog instead of running off-screen (the header
          and action buttons stayed unreachable before). Every dialog in the
          app inherits this — don't re-declare max-h/overflow at call sites. */}
      <DialogPrimitive.Content
        onOpenAutoFocus={() => { returnFocus.current = opener?.current || document.activeElement as HTMLElement; }}
        onCloseAutoFocus={(event) => {
          // autoFocus can skip Radix's mount event entirely, so the root's
          // captured opener is also read directly on close.
          const target = opener?.current || returnFocus.current;
          if (target?.isConnected) {
            event.preventDefault();
            target.focus();
          }
        }}
        className={cn(
          "app-dialog fixed left-1/2 top-1/2 z-50 flex max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 flex-col overflow-y-auto overscroll-contain rounded-2xl border border-black/10 bg-white p-4 shadow-2xl sm:p-6",
          className
        )}
      >
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
}

export function DialogHeader({
  title,
  description,
  actions,
  closeLabel = "ปิดหน้าต่าง",
  closeDisabled = false,
  closeOnDesktop = false
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  closeLabel?: string;
  closeDisabled?: boolean;
  closeOnDesktop?: boolean;
}) {
  return (
    <div className="dialog-header sticky -top-4 z-10 mb-4 flex shrink-0 items-start justify-between gap-2 bg-card pb-2 pt-1 sm:static sm:gap-4">
      <div className="min-w-0 flex-1">
        <DialogPrimitive.Title className="text-lg font-semibold text-black">{title}</DialogPrimitive.Title>
        {description ? (
          <DialogPrimitive.Description className="mt-1 text-sm text-muted-foreground">
            {description}
          </DialogPrimitive.Description>
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
        {actions}
        <DialogPrimitive.Close aria-label={closeLabel} disabled={closeDisabled} className={cn("grid h-11 w-11 shrink-0 place-items-center rounded-lg hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", !closeOnDesktop && "sm:hidden")} type="button">
          <X className="h-5 w-5" />
        </DialogPrimitive.Close>
      </div>
    </div>
  );
}
