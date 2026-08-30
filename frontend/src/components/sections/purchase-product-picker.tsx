"use client";

import { useDeferredValue, useEffect, useRef, useState } from "react";
import { PackageSearch, Search } from "lucide-react";

import { EmptyState, Input } from "@/components/ui/primitives";
import { ProductThumbnail } from "@/components/sections/product-thumbnail";
import { currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Item = Record<string, unknown>;

export function PurchaseProductPicker({
  branchId,
  stockBucket,
  onChoose,
}: {
  branchId: string;
  stockBucket: "real" | "ghost";
  onChoose: (item: Item) => void;
}) {
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  const [items, setItems] = useState<Item[]>([]);
  const [cursor, setCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const controllerRef = useRef<AbortController | null>(null);

  async function load(reset: boolean) {
    if (!branchId || (!reset && (loading || !hasMore))) return;
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    setLoading(true);
    try {
      const params = new URLSearchParams({
        branch_id: branchId,
        stock_bucket: stockBucket,
        query: deferredQuery.trim(),
        limit: "20",
      });
      if (!reset && cursor) params.set("cursor", cursor);
      const response = await proxyClient<{
        items: Item[];
        next_cursor?: string;
        has_more: boolean;
      }>(`/purchase-orders/product-options?${params.toString()}`, {
        signal: controller.signal,
      });
      setItems((current) =>
        reset
          ? response.items
          : [
              ...current,
              ...response.items.filter(
                (next) =>
                  !current.some((item) => String(item.id) === String(next.id)),
              ),
            ],
      );
      setCursor(response.next_cursor || "");
      setHasMore(response.has_more);
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError"))
        setItems([]);
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }

  useEffect(() => {
    const timer = window.setTimeout(() => void load(true), 220);
    return () => {
      window.clearTimeout(timer);
      controllerRef.current?.abort();
    }; // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [branchId, stockBucket, deferredQuery]);

  return (
    <div className="overflow-hidden rounded-2xl border bg-white">
      <div className="relative border-b p-2">
        <Search className="pointer-events-none absolute left-5 top-5 h-4 w-4 text-muted-foreground" />
        <Input
          aria-label="ค้นหาสินค้าเข้า PO"
          className="pl-9"
          onChange={(event) => setQuery(event.target.value)}
          placeholder="ค้นหาชื่อสินค้า SKU หรือบาร์โค้ด"
          value={query}
        />
      </div>
      {!query ? (
        <p className="border-b bg-secondary/15 px-3 py-2 text-xs text-muted-foreground">
          แสดงสินค้าที่หมดในสต๊อก{stockBucket === "real" ? "จริง" : "ผี"}ก่อน ·
          เลื่อนลงเพื่อโหลดครั้งละ 20
        </p>
      ) : null}
      <div
        className="max-h-72 overflow-y-auto p-2"
        onScroll={(event) => {
          const node = event.currentTarget;
          if (node.scrollHeight - node.scrollTop - node.clientHeight < 48)
            void load(false);
        }}
        role="listbox"
      >
        {items.map((item) => (
          <button
            className="flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-sm hover:bg-muted"
            key={String(item.id)}
            onClick={() => onChoose(item)}
            type="button"
          >
            <ProductThumbnail
              available={Boolean(item.image_available)}
              className="h-12 w-12"
              imageCount={Number(item.image_count || 0)}
              name={String(item.name)}
              productId={String(item.id)}
            />
            <span className="min-w-0 flex-1">
              <strong className="block truncate">{String(item.name)}</strong>
              <span className="block truncate text-xs text-muted-foreground">
                {String(item.sku)}
                {item.barcode ? ` · ${String(item.barcode)}` : ""}
              </span>
            </span>
            <span className="shrink-0 text-right text-xs">
              <strong className="block">
                คงเหลือ {Number(item.sellable_quantity || 0)}
              </strong>
              <span className="text-muted-foreground">
                ทุน {currency(Number(item.cost_price || 0))}
              </span>
            </span>
          </button>
        ))}
        {!loading && items.length === 0 ? (
          <EmptyState className="p-6" description={query ? "ลองปรับคำค้นหาแล้วลองใหม่" : "ไม่มีสินค้าที่หมดใน bucket นี้"} icon={PackageSearch} />
        ) : null}
        {loading ? (
          <p className="p-3 text-center text-xs text-muted-foreground">
            กำลังโหลด...
          </p>
        ) : null}
      </div>
    </div>
  );
}
