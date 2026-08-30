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
  children,
  side = "bottom",
  align = "center",
  sideOffset = 8
}: PropsWithChildren<{
  className?: string;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  sideOffset?: number;
}>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        align={align}
        className={cn(
          "z-50 min-w-40 rounded-xl border border-black/10 bg-white p-2 shadow-xl",
          className
        )}
        side={side}
        sideOffset={sideOffset}
      >
        {children}
      </DropdownMenuPrimitive.Content>
    </DropdownMenuPrimitive.Portal>
  );
}

export function DropdownMenuItem({
  className,
  children,
  asChild
}: PropsWithChildren<{ className?: string; asChild?: boolean }>) {
  return (
    <DropdownMenuPrimitive.Item
      asChild={asChild}
      className={cn(
        "rounded-lg px-3 py-2 text-sm text-black outline-none transition hover:bg-black/[0.04] focus:bg-black/[0.04]",
        className
      )}
    >
      {children}
    </DropdownMenuPrimitive.Item>
  );
}
