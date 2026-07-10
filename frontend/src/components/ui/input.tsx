import type { InputHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={cn(
        "flex h-11 w-full rounded-md border border-black/10 bg-white px-3 py-2 text-sm outline-none transition placeholder:text-black/35 focus-visible:ring-2 focus-visible:ring-black/10",
        props.className
      )}
    />
  );
}
