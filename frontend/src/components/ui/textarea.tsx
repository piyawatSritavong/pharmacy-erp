import type { TextareaHTMLAttributes } from "react";

import { cn } from "@/lib/utils";

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={cn(
        // text-foreground explicit for the same reason as Input/Select.
        "flex min-h-24 w-full rounded-md border border-input bg-card px-3 py-2 text-sm text-foreground shadow-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50",
        props.className
      )}
    />
  );
}

/** Grows with its content instead of scrolling (D10's address field). */
export function AutoResizeTextarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  function resize(element: HTMLTextAreaElement) {
    element.style.height = "auto";
    element.style.height = `${element.scrollHeight}px`;
  }
  return (
    <textarea
      {...props}
      className={cn(
        "flex min-h-10 w-full resize-none overflow-hidden rounded-md border border-input bg-card px-3 py-2 text-sm text-foreground shadow-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50",
        props.className
      )}
      onInput={(event) => {
        resize(event.currentTarget);
        props.onInput?.(event);
      }}
      ref={(node) => {
        if (node) resize(node);
      }}
      rows={1}
    />
  );
}
