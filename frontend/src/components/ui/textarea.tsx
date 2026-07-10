import type { TextareaHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={cn(
        "flex min-h-24 w-full rounded-md border border-black/10 bg-white px-3 py-2 text-sm outline-none transition placeholder:text-black/35 focus-visible:ring-2 focus-visible:ring-black/10",
        props.className
      )}
    />
  );
}
