"use client";

import * as React from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown, ChevronUp } from "lucide-react";

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
  const displayLabel = selected?.label || placeholder || "เลือกตัวเลือก";

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
            // text-foreground explicit for the same reason as Input — see
            // its comment (D4's dark-card "invisible text" bug).
            "ui-select flex h-10 min-w-0 max-w-full w-full items-center justify-between gap-2 rounded-md border border-input bg-card px-3 py-2 text-left text-sm text-foreground shadow-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50 [&>span:first-child]:truncate",
            className
          )}
          {...(props as React.ComponentPropsWithoutRef<typeof SelectPrimitive.Trigger>)}
        >
          <SelectPrimitive.Value aria-label={displayLabel} placeholder={displayLabel} />
          <SelectPrimitive.Icon asChild>
            <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          </SelectPrimitive.Icon>
        </SelectPrimitive.Trigger>
        <SelectPrimitive.Portal>
          {/* Long option lists (page pickers, branch/category lists) get a
              fixed cap and scroll instead of growing off-screen. The Radix
              var keeps it inside the viewport on short screens; 18rem caps it
              on tall ones. */}
          <SelectPrimitive.Content
            className="z-50 max-h-[min(18rem,var(--radix-select-content-available-height))] min-w-[8rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-md border bg-card text-card-foreground shadow-md"
            collisionPadding={16}
            position="popper"
            sideOffset={6}
          >
            <SelectPrimitive.ScrollUpButton className="flex h-6 cursor-default items-center justify-center bg-card text-muted-foreground">
              <ChevronUp className="h-4 w-4" />
            </SelectPrimitive.ScrollUpButton>
            <SelectPrimitive.Viewport className="max-h-[inherit] overflow-y-auto overscroll-contain p-1">
              {items.map((item) => (
                <SelectPrimitive.Item
                  key={item.key}
                  value={item.radixValue}
                  className="relative flex min-h-11 cursor-default select-none items-center break-words rounded-sm py-2 pl-8 pr-3 text-sm text-foreground outline-none data-[disabled]:pointer-events-none data-[disabled]:opacity-40 data-[highlighted]:bg-muted data-[highlighted]:text-foreground sm:min-h-0"
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
            <SelectPrimitive.ScrollDownButton className="flex h-6 cursor-default items-center justify-center bg-card text-muted-foreground">
              <ChevronDown className="h-4 w-4" />
            </SelectPrimitive.ScrollDownButton>
          </SelectPrimitive.Content>
        </SelectPrimitive.Portal>
      </SelectPrimitive.Root>
      {name ? <input type="hidden" name={name} value={resolvedValue} disabled={disabled} /> : null}
    </>
  );
}
