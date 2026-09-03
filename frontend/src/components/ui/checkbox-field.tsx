"use client";

import { useId, type ChangeEventHandler, type ReactNode } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";

/**
 * The one labelled checkbox in the app. Every "tick this on" control — in a
 * dialog, a settings form, or at the till — renders through here, so they all
 * share the same height, border, spacing and check mark. Before this each
 * screen rolled its own row (and several still used a bare
 * <input type="checkbox">, which drew the browser's blue box next to our black
 * one), so the same control looked different from page to page.
 *
 * Use the bare <Checkbox /> only where there is no label to show — a selection
 * column in a table, for instance.
 *
 * The whole row is clickable: Radix renders the control as a <button>, which is
 * a labelable element, so htmlFor reaches it.
 */
export function CheckboxField({
  label,
  checked,
  defaultChecked,
  onChange,
  name,
  disabled = false,
  className,
  "aria-label": ariaLabel
}: {
  label: ReactNode;
  checked?: boolean;
  defaultChecked?: boolean;
  onChange?: ChangeEventHandler<HTMLInputElement>;
  name?: string;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}) {
  const id = useId();

  return (
    <label
      className={cn(
        "flex min-h-11 cursor-pointer select-none items-center gap-3 rounded-xl border bg-white px-4 py-2.5 text-sm font-medium transition hover:bg-muted",
        disabled && "cursor-not-allowed opacity-50 hover:bg-white",
        className
      )}
      htmlFor={id}
    >
      <Checkbox
        aria-label={ariaLabel}
        checked={checked}
        defaultChecked={defaultChecked}
        disabled={disabled}
        id={id}
        name={name}
        onChange={onChange}
      />
      <span>{label}</span>
    </label>
  );
}
