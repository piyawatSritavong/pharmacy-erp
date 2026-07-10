"use client";

import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu";
import type { PropsWithChildren } from "react";

import { cn } from "@/lib/utils";

export function DropdownMenu({ children }: PropsWithChildren) {
  return <DropdownMenuPrimitive.Root>{children}</DropdownMenuPrimitive.Root>;
}

export function DropdownMenuTrigger({ children }: PropsWithChildren) {
  return <DropdownMenuPrimitive.Trigger asChild>{children}</DropdownMenuPrimitive.Trigger>;
}

export function DropdownMenuContent({
  className,
  children
}: PropsWithChildren<{ className?: string }>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        className={cn(
          "z-50 min-w-40 rounded-xl border border-black/10 bg-white p-2 shadow-xl",
          className
        )}
        sideOffset={8}
      >
        {children}
      </DropdownMenuPrimitive.Content>
    </DropdownMenuPrimitive.Portal>
  );
}

export function DropdownMenuItem({ className, children }: PropsWithChildren<{ className?: string }>) {
  return (
    <DropdownMenuPrimitive.Item
      className={cn(
        "rounded-lg px-3 py-2 text-sm text-black outline-none transition hover:bg-black/[0.04] focus:bg-black/[0.04]",
        className
      )}
    >
      {children}
    </DropdownMenuPrimitive.Item>
  );
}
