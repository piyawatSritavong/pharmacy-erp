"use client";

import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { Check } from "lucide-react";
import React, { forwardRef, useMemo, useState, type ChangeEvent } from "react";

import { cn } from "@/lib/utils";

type CheckboxProps = Omit<
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>,
  "checked" | "defaultChecked" | "onCheckedChange" | "onChange"
> & {
  checked?: boolean;
  defaultChecked?: boolean;
  onChange?: React.ChangeEventHandler<HTMLInputElement>;
  name?: string;
  value?: string | number | readonly string[];
};

export const Checkbox = forwardRef<HTMLButtonElement, CheckboxProps>(function Checkbox(
  {
    checked,
    defaultChecked,
    onChange,
    name,
    value,
    className,
    disabled,
    ...props
  },
  ref
) {
  const controlled = checked !== undefined;
  const [internalChecked, setInternalChecked] = useState(Boolean(defaultChecked));
  const resolvedChecked = controlled ? Boolean(checked) : internalChecked;
  const normalizedValue = useMemo(() => {
    if (typeof value === "string") {
      return value;
    }
    if (typeof value === "number") {
      return String(value);
    }
    return "on";
  }, [value]);

  function emitChange(next: boolean) {
    if (!controlled) {
      setInternalChecked(next);
    }
    if (!onChange) {
      return;
    }
    const syntheticEvent = {
      target: { checked: next, name, value: normalizedValue },
      currentTarget: { checked: next, name, value: normalizedValue }
    } as unknown as ChangeEvent<HTMLInputElement>;
    onChange(syntheticEvent);
  }

  return (
    <>
      <CheckboxPrimitive.Root
        ref={ref}
        checked={resolvedChecked}
        className={cn(
          "peer h-4 w-4 shrink-0 rounded-sm border border-black/20 bg-white shadow-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-black data-[state=checked]:bg-black data-[state=checked]:text-white",
          className
        )}
        disabled={disabled}
        onCheckedChange={(state) => emitChange(state === true)}
        {...props}
      >
        <CheckboxPrimitive.Indicator className="flex items-center justify-center text-current">
          <Check className="h-3.5 w-3.5" />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      <input
        aria-hidden="true"
        checked={resolvedChecked}
        className="sr-only"
        disabled={disabled}
        name={name}
        onChange={() => {}}
        readOnly
        tabIndex={-1}
        type="checkbox"
        value={normalizedValue}
      />
    </>
  );
});
