"use client";

import { useDeferredValue, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { PackageSearch, Search } from "lucide-react";

import { EmptyState, Input } from "@/components/ui/primitives";
import { ProductThumbnail } from "@/components/sections/product-thumbnail";
import { cn } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type ProductOption = Record<string, unknown>;

export function ProductSearchPicker({
  ariaLabel,
  name,
  value,
  onChange,
  initialLabel = "",
  initialOptions = [],
  disabled = false
}: {
  ariaLabel: string;
  name?: string;
  value: string;
  onChange: (value: string, product?: ProductOption) => void;
  initialLabel?: string;
  initialOptions?: ProductOption[];
  disabled?: boolean;
}) {
  const initialProduct = initialOptions.find((product) => String(product.id) === value);
  const [query, setQuery] = useState(initialLabel || (initialProduct ? `${String(initialProduct.name)} · ${String(initialProduct.sku)}` : ""));
  const [items, setItems] = useState<ProductOption[]>(initialOptions.slice(0, 20));
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const deferredQuery = useDeferredValue(query);

  // A5 fix: the results panel is portaled to <body> and positioned via
  // getBoundingClientRect() instead of `position: absolute` inside this
  // wrapper — otherwise it gets clipped/scrolled-with whenever this picker
  // is used inside a scrollable Dialog (see D6's "dropdown clipped by
  // dialog boundary" bug).
  const anchorRef = useRef<HTMLDivElement | null>(null);
  const [panelRect, setPanelRect] = useState<{ top: number; left: number; width: number } | null>(null);
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    if (!open) return;
    function reposition() {
      const rect = anchorRef.current?.getBoundingClientRect();
      if (!rect) return;
      setPanelRect({ top: rect.bottom + 8, left: rect.left, width: rect.width });
    }
    reposition();
    window.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    return () => {
      window.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      setLoading(true);
      try {
        const search = deferredQuery.includes(" · ") && value ? "" : deferredQuery.trim();
        const response = await proxyClient<{ items: ProductOption[] }>(
          `/products?search=${encodeURIComponent(search)}&active=true&page=1&page_size=20`,
          { signal: controller.signal }
        );
        setItems(response.items);
      } catch (error) {
        if (!(error instanceof DOMException && error.name === "AbortError")) setItems([]);
      } finally {
        setLoading(false);
      }
    }, 220);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [deferredQuery, open, value]);

  function choose(product: ProductOption) {
    const label = `${String(product.name)} · ${String(product.sku)}`;
    setQuery(label);
    setOpen(false);
    onChange(String(product.id), product);
  }

  const panel =
    open && !disabled ? (
      <div
        // Radix Dialog sets `pointer-events: none` on <body> while open (and
        // `auto` on itself) to lock out the background. This panel is a
        // *sibling* portal under <body>, not nested inside the dialog, so it
        // inherits that `none` and renders visually on top but eats no
        // clicks unless explicitly opted back in here.
        className="pointer-events-auto fixed z-50 max-h-72 overflow-y-auto rounded-2xl border bg-white p-2 shadow-xl"
        role="listbox"
        style={panelRect ? { top: panelRect.top, left: panelRect.left, width: panelRect.width } : { visibility: "hidden" }}
      >
        {items.map((product) => (
          <button
            aria-label={String(product.name)}
            aria-selected={String(product.id) === value}
            className={cn("flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-muted", String(product.id) === value && "bg-primary/10")}
            key={String(product.id)}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => choose(product)}
            role="option"
            type="button"
          >
            <ProductThumbnail available={Boolean(product.image_available)} className="h-11 w-11" imageCount={Number(product.image_count || 0)} name={String(product.name)} productId={String(product.id)} />
            <span className="min-w-0 flex-1"><strong className="block truncate">{String(product.name)}</strong><span className="block truncate text-xs text-muted-foreground">{String(product.sku)}{product.barcode ? ` · ${String(product.barcode)}` : ""}</span></span>
            <span className="shrink-0 text-xs text-muted-foreground">{String(product.category_name || "")}</span>
          </button>
        ))}
        {!loading && items.length === 0 ? <EmptyState className="p-5" icon={PackageSearch} /> : null}
        {loading ? <p className="p-4 text-center text-sm text-muted-foreground">กำลังค้นหา...</p> : null}
      </div>
    ) : null;

  return (
    <div className="relative" ref={anchorRef}>
      {name ? <input name={name} type="hidden" value={value} /> : null}
      <Search className="pointer-events-none absolute left-3 top-3 z-10 h-4 w-4 text-muted-foreground" />
      <Input
        aria-autocomplete="list"
        aria-expanded={open}
        aria-label={ariaLabel}
        className="pl-9"
        disabled={disabled}
        onBlur={() => window.setTimeout(() => setOpen(false), 150)}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
          if (value) onChange("");
        }}
        onFocus={() => setOpen(true)}
        placeholder="ค้นหาชื่อสินค้า, SKU หรือบาร์โค้ด"
        role="combobox"
        value={query}
      />
      {mounted && panel ? createPortal(panel, document.body) : null}
    </div>
  );
}
