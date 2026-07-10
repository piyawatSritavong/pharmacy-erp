"use client";

import * as React from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown } from "lucide-react";

import { cn } from "@/lib/utils";

const EMPTY_VALUE = "__empty__";

type NativeSelectProps = Omit<
  React.SelectHTMLAttributes<HTMLSelectElement>,
  "children" | "onChange" | "value" | "defaultValue"
> & {
  children: React.ReactNode;
  value?: string;
  defaultValue?: string;
  onChange?: (event: React.ChangeEvent<HTMLSelectElement>) => void;
  placeholder?: string;
};

type ParsedOption = {
  key: string;
  nativeValue: string;
  radixValue: string;
  label: string;
  disabled: boolean;
};

function flattenText(node: React.ReactNode): string {
  return React.Children.toArray(node)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number") {
        return String(child);
      }
      if (React.isValidElement(child)) {
        return flattenText(child.props.children);
      }
      return "";
    })
    .join("")
    .trim();
}

function collectOptions(children: React.ReactNode, bucket: ParsedOption[]) {
  React.Children.forEach(children, (child) => {
    if (!React.isValidElement(child)) {
      return;
    }
    if (child.type === React.Fragment) {
      collectOptions(child.props.children, bucket);
      return;
    }
    if (child.type !== "option") {
      return;
    }

    const nativeValue = String(child.props.value ?? "");
    bucket.push({
      key: String(child.key ?? `${nativeValue}-${bucket.length}`),
      nativeValue,
      radixValue: nativeValue === "" ? EMPTY_VALUE : nativeValue,
      label: flattenText(child.props.children),
      disabled: Boolean(child.props.disabled)
    });
  });
}

function parseOptions(children: React.ReactNode) {
  const bucket: ParsedOption[] = [];
  collectOptions(children, bucket);
  return bucket;
}

export function Select({
  children,
  value,
  defaultValue,
  onChange,
  name,
  disabled,
  className,
  placeholder,
  ...props
}: NativeSelectProps) {
  const controlled = value !== undefined;
  const [internalValue, setInternalValue] = React.useState(defaultValue ?? "");
  const resolvedValue = controlled ? value ?? "" : internalValue;
  const items = React.useMemo(() => parseOptions(children), [children]);
  const selected = items.find((item) => item.nativeValue === resolvedValue);
  const displayLabel = selected?.label || placeholder || "Select option";

  function emitChange(nextValue: string) {
    if (!controlled) {
      setInternalValue(nextValue);
    }
    if (!onChange) {
      return;
    }
    const syntheticEvent = {
      target: { value: nextValue, name },
      currentTarget: { value: nextValue, name }
    } as unknown as React.ChangeEvent<HTMLSelectElement>;
    onChange(syntheticEvent);
  }

  return (
    <>
      <SelectPrimitive.Root
        disabled={disabled}
        value={resolvedValue === "" ? EMPTY_VALUE : resolvedValue}
        onValueChange={(nextValue) => emitChange(nextValue === EMPTY_VALUE ? "" : nextValue)}
      >
        <SelectPrimitive.Trigger
          className={cn(
            "flex h-11 w-full items-center justify-between rounded-md border border-black/10 bg-white px-3 py-2 text-left text-sm outline-none transition placeholder:text-black/35 focus-visible:ring-2 focus-visible:ring-black/10 disabled:cursor-not-allowed disabled:opacity-50",
            className
          )}
          {...(props as React.ComponentPropsWithoutRef<typeof SelectPrimitive.Trigger>)}
        >
          <SelectPrimitive.Value aria-label={displayLabel} placeholder={displayLabel} />
          <SelectPrimitive.Icon asChild>
            <ChevronDown className="h-4 w-4 text-black/45" />
          </SelectPrimitive.Icon>
        </SelectPrimitive.Trigger>
        <SelectPrimitive.Portal>
          <SelectPrimitive.Content
            className="z-50 min-w-[8rem] overflow-hidden rounded-xl border border-black/10 bg-white shadow-2xl"
            position="popper"
            sideOffset={6}
          >
            <SelectPrimitive.Viewport className="p-1">
              {items.map((item) => (
                <SelectPrimitive.Item
                  key={item.key}
                  value={item.radixValue}
                  className="relative flex cursor-default select-none items-center rounded-lg py-2 pl-8 pr-3 text-sm text-black outline-none data-[disabled]:pointer-events-none data-[disabled]:opacity-40 data-[highlighted]:bg-black data-[highlighted]:text-white"
                  disabled={item.disabled}
                >
                  <span className="absolute left-2 flex h-4 w-4 items-center justify-center">
                    <SelectPrimitive.ItemIndicator>
                      <Check className="h-3.5 w-3.5" />
                    </SelectPrimitive.ItemIndicator>
                  </span>
                  <SelectPrimitive.ItemText>{item.label}</SelectPrimitive.ItemText>
                </SelectPrimitive.Item>
              ))}
            </SelectPrimitive.Viewport>
          </SelectPrimitive.Content>
        </SelectPrimitive.Portal>
      </SelectPrimitive.Root>
      {name ? <input type="hidden" name={name} value={resolvedValue} disabled={disabled} /> : null}
    </>
  );
}
