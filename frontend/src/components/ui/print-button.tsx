"use client";

import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function PrintButton() {
  return (
    <button
      className={cn(buttonVariants(), "print:hidden")}
      onClick={() => window.print()}
      type="button"
    >
      พิมพ์เอกสาร
    </button>
  );
}
