"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import { useEffect, useRef, useState, type PropsWithChildren } from "react";

/** One cart, inline on wide tills and focus-trapped above the product list on
 * smaller screens. The complete cart scrolls on short/keyboard-sized screens. */
export function PosCart({ children, open, onOpenChange }: PropsWithChildren<{
  open: boolean;
  onOpenChange: (open: boolean) => void;
}>) {
  const [inline, setInline] = useState(false);
  const returnFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    const query = window.matchMedia("(min-width: 1280px)");
    const update = () => setInline(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  if (inline) return <aside className="pos-cart flex h-full min-h-0 flex-col rounded-2xl border bg-white p-4 shadow-card">{children}</aside>;
  return <DialogPrimitive.Root onOpenChange={onOpenChange} open={open}>
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="fixed inset-0 z-40 bg-black/40" />
      <DialogPrimitive.Content
        aria-describedby={undefined}
        className="pos-cart fixed inset-x-2 top-2 z-40 mx-auto flex max-h-[calc(100dvh-1rem)] w-auto max-w-lg flex-col overflow-y-auto overscroll-contain rounded-2xl border bg-card p-3 shadow-2xl sm:inset-x-4 sm:top-4 sm:max-h-[calc(100dvh-2rem)] sm:p-4"
        onOpenAutoFocus={() => { returnFocus.current = document.activeElement as HTMLElement; }}
        onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus.current?.focus(); }}
      >
        <DialogPrimitive.Title asChild><span className="sr-only">รายการขายปัจจุบัน</span></DialogPrimitive.Title>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  </DialogPrimitive.Root>;
}
