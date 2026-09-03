"use client";

import { startTransition, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Layers3, Pencil, Trash2 } from "lucide-react";

import { DataTable, SectionCard } from "@/components/sections/common";
import { ProductSearchPicker } from "@/components/sections/product-search-picker";
import { ProductThumbnail } from "@/components/sections/product-thumbnail";
import { ProductImageSwiper } from "@/components/sections/product-image-gallery";
import { Field } from "@/components/ui/field";
import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogHeader,
  EmptyState,
  InfiniteScrollTrigger,
  Input,
  Notice,
  Pagination as ListPagination,
  Select,
  useInfiniteList,
} from "@/components/ui/primitives";
import { cn, currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";
import type { Pagination } from "@/services/erp";

type Option = Record<string, unknown>;

/** Same footprint as a <Field> so read-only context lines up with the inputs
 *  beside them, but renders plain text instead of a disabled control. */
function ReadOnlyField({
  className,
  label,
  value
}: {
  className?: string;
  label: string;
  value: string;
}) {
  return (
    <div className={cn("space-y-1.5", className)}>
      <p className="text-sm font-medium text-muted-foreground">{label}</p>
      <p className="text-sm font-medium text-foreground">{value || "-"}</p>
    </div>
  );
}

export function InventoryConsole({
  products,
  branches,
  inventory = [],
  defaultBranchId,
  mode = "manage",
  manageBucket,
  routePath = "/real-inventory",
  canManageGhost = false,
  showFullTimestamp = false,
  pagination,
  filters,
}: {
  products: Option[];
  branches: Option[];
  inventory?: Option[];
  defaultBranchId?: string;
  mode?: "manage" | "check";
  manageBucket?: "real" | "ghost";
  routePath?: string;
  canManageGhost?: boolean;
  showFullTimestamp?: boolean;
  pagination?: Pagination;
  filters?: { search: string; branchId: string; page: number; pageSize?: number };
}) {
  const router = useRouter();
  // Only the superadmin has a Ghost bucket to tell this one apart from, so for
  // everyone else "สต๊อกจริง" is just "สต๊อก".
  const realLabel = canManageGhost ? "สต๊อกจริง" : "สต๊อก";
  const [branchId, setBranchId] = useState(
    defaultBranchId || String(inventory[0]?.branch_id || ""),
  );
  const [productId, setProductId] = useState(
    String(inventory[0]?.product_id || ""),
  );
  const stockBucket = manageBucket || "real";
  const [reason, setReason] = useState("");
  const [lotsOpen, setLotsOpen] = useState(false);
  const [lots, setLots] = useState<Option[]>([]);
  const [lotsLoading, setLotsLoading] = useState(false);
  const [movements, setMovements] = useState<Option[]>([]);
  const [branchSettingsOpen, setBranchSettingsOpen] = useState(false);
  // Lot-targeted stock adjustment, run from the ตั้งค่าเฉพาะสาขา dialog.
  const [adjustLotId, setAdjustLotId] = useState("");
  const [adjustDelta, setAdjustDelta] = useState("");
  const [adjustReason, setAdjustReason] = useState("");
  const [adjustConfirmed, setAdjustConfirmed] = useState(false);
  const [adjustBusy, setAdjustBusy] = useState(false);
  const [adjustError, setAdjustError] = useState("");
  const [branchSettings, setBranchSettings] = useState({
    selling_price: "",
    warehouse_price: "",
    selling_price_source: "warehouse",
    max_discount_amount: "",
    low_stock_real_threshold: "",
    low_stock_ghost_threshold: "",
  });
  const [branchPrice, setBranchPrice] = useState({
    value: "",
    catalog: 0,
    source: "warehouse",
    loading: false,
    error: "",
  });
  // Only used when an expiry-tracked product has no existing lot to inherit
  // an expiry date from — otherwise the date comes from stock on hand.
  const [message, setMessage] = useState("");
  const [search, setSearch] = useState(filters?.search || "");
  const [selectedInventoryId, setSelectedInventoryId] = useState(
    String(inventory[0]?.id || ""),
  );
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteImpact, setDeleteImpact] = useState<{ confirmation: string; counts: Record<string, number> } | null>(null);
  const [deleteText, setDeleteText] = useState("");

  useEffect(() => {
    setBranchId(defaultBranchId || "");
  }, [defaultBranchId]);

  useEffect(() => {
    if (!inventory.some((item) => String(item.id) === selectedInventoryId)) {
      const first = inventory[0];
      setSelectedInventoryId(String(first?.id || ""));
      if (first) {
        setBranchId(String(first.branch_id));
        setProductId(String(first.product_id));
      }
    }
  }, [inventory, selectedInventoryId]);

  // "เช็กสต๊อก" (mode="check") loads its list itself, 20 at a time (A4),
  // instead of the page server-fetching the whole branch in one shot.
  const [debouncedSearch, setDebouncedSearch] = useState(search);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search), 250);
    return () => window.clearTimeout(timer);
  }, [search]);
  // POS "เช็กสต๊อก" shows reorder-point alerts only — never the branch's full
  // stock list with on-hand numbers. Cashiers who can read the system's count
  // stop counting the shelf during a physical audit, and the two quietly drift
  // apart. GET /inventory has no low-stock filter, so the pages are walked here
  // and only the flagged rows are kept.
  const [alerts, setAlerts] = useState<Option[]>([]);
  const [alertsLoading, setAlertsLoading] = useState(false);
  const [alertsError, setAlertsError] = useState("");
  const [alertPage, setAlertPage] = useState(1);
  const [alertPageSize, setAlertPageSize] = useState(20);
  useEffect(() => {
    if (mode !== "check" || !branchId) return;
    let cancelled = false;
    setAlertsLoading(true);
    setAlertsError("");
    (async () => {
      const collected: Option[] = [];
      try {
        for (let page = 1; page <= 40; page += 1) {
          const query = new URLSearchParams({ branch_id: branchId, page: String(page), page_size: "100" });
          if (debouncedSearch.trim()) query.set("search", debouncedSearch.trim());
          const response = await proxyClient<{ items: Option[]; pagination: Pagination }>(`/inventory?${query.toString()}`);
          if (cancelled) return;
          collected.push(...response.items.filter((item) => Boolean(item.is_low_stock_real)));
          if (page >= response.pagination.total_pages) break;
        }
        setAlerts(collected);
        setAlertPage(1);
      } catch (error) {
        if (!cancelled) setAlertsError(error instanceof Error ? error.message : "โหลดการแจ้งเตือนสต๊อกไม่สำเร็จ");
      } finally {
        if (!cancelled) setAlertsLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [branchId, debouncedSearch, mode]);
  const branchLabel = String(
    branches.find((item) => String(item.id) === branchId)?.name || "",
  );
  const selectedInventory = inventory.find(
    (item) => String(item.id) === selectedInventoryId,
  );
  const selectedBranch = branches.find((item) => String(item.id) === String(selectedInventory?.branch_id || ""));
  const isWarehouseBranch = String(selectedBranch?.branch_type) === "main_warehouse";
  const resolvedPagination = pagination || {
    page: 1,
    page_size: inventory.length,
    total: inventory.length,
    total_pages: 1,
  };
  // D2: "จำนวนที่ปรับ" is not a +/- delta — it defaults to the current stock
  // value for the bucket being managed, and whatever's typed overwrites it.
  // Tracks whichever branch/product the form actually targets (not just the
  // highlighted aside row), since the two can be pointed at different items.
  const formInventoryMatch = inventory.find(
    (item) => String(item.branch_id) === branchId && String(item.product_id) === productId,
  );
  const activeBucket = manageBucket || stockBucket;
  const currentBucketQty = formInventoryMatch
    ? Number(formInventoryMatch[activeBucket === "ghost" ? "qty_ghost" : "qty_real"] || 0)
    : 0;

  useEffect(() => {
    if (mode === "check") return;
    void loadBranchPrice();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedInventoryId, mode]);

  function selectInventory(item: Option) {
    setSelectedInventoryId(String(item.id));
    setBranchId(String(item.branch_id));
    setProductId(String(item.product_id));
  }

  function applyFilters(
    page = 1,
    nextBranchId = branchId,
    searchOverride?: string,
    pageSizeOverride?: number,
  ) {
    const query = new URLSearchParams({ inventory_page: String(page) });
    const searchValue = searchOverride ?? search;
    if (searchValue.trim()) query.set("inventory_q", searchValue.trim());
    if (nextBranchId && manageBucket !== "ghost") query.set("inventory_branch", nextBranchId);
    const size = pageSizeOverride ?? filters?.pageSize;
    if (size) query.set("inventory_size", String(size));
    startTransition(() => router.push(`${routePath}?${query.toString()}`));
  }

  async function submitLotAdjustment() {
    const delta = Number(adjustDelta);
    if (!selectedInventory || !adjustLotId || !Number.isFinite(delta) || delta === 0) return;
    setAdjustBusy(true);
    setAdjustError("");
    try {
      // Every field the backend needs to touch exactly one lot. It writes an
      // inventory_movements row and an audit_logs entry with before/after
      // quantities, so the change is traceable to whoever saved it.
      await proxyClient("/inventory/adjust", {
        method: "POST",
        body: JSON.stringify({
          branch_id: String(selectedInventory.branch_id),
          product_id: String(selectedInventory.product_id),
          stock_bucket: activeBucket,
          inventory_lot_id: adjustLotId,
          quantity_delta: delta,
          reason: adjustReason,
        }),
      });
      setMessage(`ปรับยอด lot แล้ว (${delta > 0 ? "+" : ""}${delta.toLocaleString("th-TH")} ชิ้น) และบันทึกลงประวัติระบบ`);
      setAdjustDelta("");
      setAdjustReason("");
      setAdjustConfirmed(false);
      setBranchSettingsOpen(false);
      startTransition(() => router.refresh());
    } catch (caught) {
      setAdjustError(caught instanceof Error ? caught.message : "ปรับยอดสต๊อกไม่สำเร็จ");
    } finally {
      setAdjustBusy(false);
    }
  }

  async function openDeleteProduct() {
    if (!selectedInventory) return;
    try {
      const impact = await proxyClient<{ confirmation: string; counts: Record<string, number> }>(
        `/products/${String(selectedInventory.product_id)}/deletion-impact`,
      );
      setDeleteImpact(impact);
      setDeleteText("");
      setDeleteOpen(true);
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ตรวจสอบผลกระทบไม่สำเร็จ");
    }
  }

  async function confirmDeleteProduct() {
    if (!selectedInventory) return;
    try {
      const response = await proxyClient<{ message: string }>(
        `/products/${String(selectedInventory.product_id)}`,
        { method: "DELETE", body: JSON.stringify({ confirmation: deleteText }) },
      );
      setDeleteOpen(false);
      setSelectedInventoryId("");
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ลบสินค้าไม่สำเร็จ");
    }
  }

  // Lots for the selected row, loaded on selection rather than on dialog open:
  // the panel shows the newest one, and the branch-settings dialog picks from
  // the same list, so one fetch serves both.
  useEffect(() => {
    if (mode === "check" || !selectedInventory) {
      setLots([]);
      return;
    }
    let cancelled = false;
    const params = new URLSearchParams({
      branch_id: String(selectedInventory.branch_id),
      product_id: String(selectedInventory.product_id),
      stock_bucket: activeBucket,
    });
    proxyClient<{ items: Option[] }>(`/inventory/lots?${params.toString()}`)
      .then((response) => { if (!cancelled) setLots(response.items); })
      .catch(() => { if (!cancelled) setLots([]); });
    return () => { cancelled = true; };
  }, [activeBucket, mode, selectedInventory]);

  useEffect(() => {
    if (mode === "check" || !selectedInventory) {
      setMovements([]);
      return;
    }
    let cancelled = false;
    const params = new URLSearchParams({
      branch_id: String(selectedInventory.branch_id),
      product_id: String(selectedInventory.product_id),
      stock_bucket: activeBucket,
      page: "1",
      page_size: "20",
    });
    proxyClient<{ items: Option[] }>(`/inventory/movements?${params.toString()}`)
      .then((response) => { if (!cancelled) setMovements(response.items); })
      .catch(() => { if (!cancelled) setMovements([]); });
    return () => { cancelled = true; };
  }, [activeBucket, mode, selectedInventory]);

  // "Most recent lot entered" — newest received_at wins, ties broken by the
  // lot number so the label never flickers between equal timestamps.
  const latestLot = useMemo(() => {
    if (!lots.length) return null;
    return [...lots].sort((a, b) => {
      const byDate = String(b.received_at || "").localeCompare(String(a.received_at || ""));
      return byDate !== 0 ? byDate : String(b.lot_number || "").localeCompare(String(a.lot_number || ""));
    })[0];
  }, [lots]);
  const thaiDate = (value: unknown) =>
    value ? new Date(String(value)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" }) : "";
  const latestLotLabel = latestLot
    ? `${String(latestLot.lot_number)} · รับเข้า ${thaiDate(latestLot.received_at)}`
    : "ยังไม่มี lot";
  const latestLotExpiryLabel = latestLot
    ? thaiDate(latestLot.expires_on) || "ไม่กำหนดวันหมดอายุ"
    : "-";

  async function openLots() {
    if (!selectedInventory) return;
    setLotsOpen(true);
    setLotsLoading(true);
    try {
      const params = new URLSearchParams({
        branch_id: String(selectedInventory.branch_id),
        product_id: String(selectedInventory.product_id),
        stock_bucket: manageBucket || stockBucket,
        include_depleted: "true",
      });
      const response = await proxyClient<{ items: Option[] }>(
        `/inventory/lots?${params.toString()}`,
      );
      setLots(response.items);
    } catch (caught) {
      setMessage(
        caught instanceof Error ? caught.message : "โหลดข้อมูล Lot ไม่สำเร็จ",
      );
      setLots([]);
    } finally {
      setLotsLoading(false);
    }
  }

  // business-flow.md: "สามารถแก้ไข ราคาขายเฉพาะสาขาได้ แต่ Default จะเป็นราคาจาก
  // รายการสินค้า หากไม่มีการตั้งราคาใหม่" — the branch price is the same field
  // as the catalog's ราคาขายตั้งต้น with an optional per-branch override, so it
  // is edited inline here rather than buried in the settings dialog.
  async function loadBranchPrice() {
    if (!selectedInventory) {
      setBranchPrice({ value: "", catalog: 0, source: "warehouse", loading: false, error: "" });
      return;
    }
    setBranchPrice((current) => ({ ...current, loading: true, error: "" }));
    try {
      const settings = await proxyClient<Option>(
        `/products/${String(selectedInventory.product_id)}/branch-settings/${String(selectedInventory.branch_id)}`,
      );
      setBranchPrice({
        value: settings.selling_price == null ? "" : String(settings.selling_price),
        catalog: Number(settings.warehouse_price ?? 0),
        source: String(settings.selling_price_source || "warehouse"),
        loading: false,
        error: "",
      });
    } catch (caught) {
      setBranchPrice((current) => ({
        ...current,
        loading: false,
        error: caught instanceof Error ? caught.message : "โหลดราคาสาขาไม่สำเร็จ",
      }));
    }
  }

  async function saveBranchPrice() {
    if (!selectedInventory || isWarehouseBranch) return;
    setBranchPrice((current) => ({ ...current, loading: true, error: "" }));
    try {
      await proxyClient(
        `/products/${String(selectedInventory.product_id)}/branch-settings/${String(selectedInventory.branch_id)}`,
        {
          method: "PUT",
          // Empty clears the override and falls back to the catalog price.
          body: JSON.stringify({
            selling_price: branchPrice.value.trim() === "" ? null : Number(branchPrice.value),
          }),
        },
      );
      setMessage("บันทึกราคาขายเฉพาะสาขาแล้ว");
      await loadBranchPrice();
      startTransition(() => router.refresh());
    } catch (caught) {
      setBranchPrice((current) => ({
        ...current,
        loading: false,
        error: caught instanceof Error ? caught.message : "บันทึกราคาสาขาไม่สำเร็จ",
      }));
    }
  }

  async function openBranchSettings() {
    setAdjustLotId("");
    setAdjustDelta("");
    setAdjustReason("");
    setAdjustConfirmed(false);
    setAdjustError("");
    if (!selectedInventory) return;
    try {
      const settings = await proxyClient<Option>(
        `/products/${String(selectedInventory.product_id)}/branch-settings/${String(selectedInventory.branch_id)}`,
      );
      setBranchSettings({
        selling_price:
          settings.selling_price == null ? "" : String(settings.selling_price),
        warehouse_price: String(settings.warehouse_price ?? 0),
        selling_price_source: String(settings.selling_price_source || "warehouse"),
        max_discount_amount:
          settings.max_discount_amount == null
            ? ""
            : String(settings.max_discount_amount),
        low_stock_real_threshold:
          settings.low_stock_real_threshold == null
            ? ""
            : String(settings.low_stock_real_threshold),
        low_stock_ghost_threshold:
          settings.low_stock_ghost_threshold == null
            ? ""
            : String(settings.low_stock_ghost_threshold),
      });
      setBranchSettingsOpen(true);
    } catch (caught) {
      setMessage(
        caught instanceof Error ? caught.message : "โหลดค่าเฉพาะสาขาไม่สำเร็จ",
      );
    }
  }

  async function saveBranchSettings() {
    if (!selectedInventory) return;
    const optionalNumber = (value: string) =>
      value.trim() === "" ? null : Number(value);
    try {
      await proxyClient(
        `/products/${String(selectedInventory.product_id)}/branch-settings/${String(selectedInventory.branch_id)}`,
        {
          method: "PUT",
          body: JSON.stringify({
            ...(String(selectedBranch?.branch_type) === "main_warehouse" ? {} : { selling_price: optionalNumber(branchSettings.selling_price) }),
            max_discount_amount: optionalNumber(
              branchSettings.max_discount_amount,
            ),
            low_stock_real_threshold: optionalNumber(
              branchSettings.low_stock_real_threshold,
            ),
            low_stock_ghost_threshold: optionalNumber(
              branchSettings.low_stock_ghost_threshold,
            ),
          }),
        },
      );
      setBranchSettingsOpen(false);
      setMessage("บันทึกค่าเฉพาะสาขาแล้ว");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(
        caught instanceof Error
          ? caught.message
          : "บันทึกค่าเฉพาะสาขาไม่สำเร็จ",
      );
    }
  }

  if (mode === "check") {
    // Alerts only — no full stock list, and deliberately no on-hand quantity
    // column: the point of this screen is to say WHICH items need attention,
    // not to tell the person counting the shelf what the system thinks is on it.
    const totalPages = Math.max(1, Math.ceil(alerts.length / alertPageSize));
    const safePage = Math.min(Math.max(1, alertPage), totalPages);
    const rows = alerts.slice((safePage - 1) * alertPageSize, safePage * alertPageSize).map((item) => ({
      ...item,
      threshold_label: `${Number(item.low_stock_real_threshold || 0).toLocaleString("th-TH")} ชิ้น`,
    }));
    return (
      <SectionCard
        title="แจ้งเตือนสต๊อกใกล้หมด"
        description="เฉพาะสินค้าที่ถึงจุดแจ้งเตือนสต๊อก — ให้ตรวจนับของจริงบนชั้นเสมอ ไม่ใช้ยอดในระบบแทนการนับ"
      >
        {alertsLoading ? (
          <Notice tone="info">กำลังตรวจสอบสต๊อกของสาขา...</Notice>
        ) : alertsError ? (
          <Notice tone="error">{alertsError}</Notice>
        ) : alerts.length > 0 ? (
          <Notice tone="warning">
            มีสินค้า {alerts.length.toLocaleString("th-TH")} รายการถึงจุดแจ้งเตือนสต๊อก — ควรส่งคำขอเบิกสินค้า
          </Notice>
        ) : (
          <Notice tone="success">ไม่มีสินค้าที่ถึงจุดแจ้งเตือนสต๊อกในขณะนี้</Notice>
        )}
        <div className="mb-4 mt-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
          <Input
            aria-label="ค้นหาในรายการแจ้งเตือน"
            onChange={(event) => setSearch(event.target.value)}
            placeholder="ค้นหาด้วยชื่อสินค้าหรือ SKU"
            value={search}
          />
          <Input aria-label="สาขาปัจจุบัน" disabled value={branchLabel} />
        </div>
        <DataTable
          columns={[
            { key: "product_name", label: "สินค้า" },
            { key: "sku", label: "SKU", className: "whitespace-nowrap" },
            { key: "threshold_label", label: "จุดแจ้งเตือน", className: "whitespace-nowrap" },
            { key: "price", label: "ราคา", type: "currency", className: "whitespace-nowrap" },
          ]}
          emptyDescription={alertsLoading ? "กำลังโหลด..." : "ไม่มีสินค้าที่ถึงจุดแจ้งเตือนสต๊อก"}
          rows={rows}
        />
        <ListPagination
          className="mt-4"
          onPageChange={setAlertPage}
          onPageSizeChange={(size) => { setAlertPageSize(size); setAlertPage(1); }}
          page={safePage}
          pageSize={alertPageSize}
          total={alerts.length}
          totalPages={totalPages}
        />
      </SectionCard>
    );
  }

  return (
    <div className="space-y-6">
      <section className="overflow-hidden rounded-3xl border bg-white shadow-card lg:grid lg:min-h-[690px] lg:grid-cols-[440px_minmax(0,1fr)]">
      <aside className="flex min-h-[560px] flex-col border-b lg:border-b-0 lg:border-r">
        <h2 className="sr-only">
          จัดการ
          {manageBucket === "ghost"
            ? "สต๊อกผี"
            : manageBucket === "real"
              ? realLabel
              : "สต๊อก"}
        </h2>
        <div className="space-y-3 border-b bg-surface-warm p-4">
          <ProductSearchPicker
            ariaLabel={manageBucket === "ghost" ? "ค้นหาสต๊อกผี" : "ค้นหาสต๊อกทุกสาขา"}
            initialOptions={products}
            onChange={(value, product) => {
              if (!product) return;
              const label = String(product.name);
              setSearch(label);
              const match = inventory.find((item) => String(item.product_id) === value);
              if (match) selectInventory(match);
              else setProductId(value);
              applyFilters(1, branchId, label);
            }}
            value=""
          />
          {manageBucket !== "ghost" ? (
            <Select
              aria-label="กรองสาขาสต๊อก"
              onChange={(event) => {
                setBranchId(event.target.value);
                applyFilters(1, event.target.value);
              }}
              value={branchId}
            >
              {branches.map((branch) => (
                <option key={String(branch.id)} value={String(branch.id)}>
                  {String(branch.name)}
                </option>
              ))}
            </Select>
          ) : null}
        </div>

        {/* Fills whatever height is left between the filters and the pager
            below, so the list runs flush to the pagination instead of
            stopping at an arbitrary fixed height. */}
        <div
          aria-label="รายการสต๊อก"
          className="min-h-0 flex-1 divide-y overflow-y-auto"
          role="listbox"
        >
          {inventory.map((item) => {
            const selected = String(item.id) === selectedInventoryId;
            return (
              <button
                aria-selected={selected}
                className={cn(
                  "w-full px-4 py-3 text-left transition",
                  selected ? "bg-primary text-white" : "hover:bg-muted",
                )}
                key={String(item.id)}
                onClick={() => selectInventory(item)}
                role="option"
                type="button"
              >
                <span className="flex items-start justify-between gap-3">
                  <ProductThumbnail
                    available={Boolean(item.image_available)}
                    className="h-12 w-12"
                    imageCount={Number(item.image_count || 0)}
                    name={String(item.product_name)}
                    productId={String(item.product_id)}
                  />
                  <span className="min-w-0 flex-1">
                    <strong className="block truncate text-sm">
                      {String(item.product_name)}
                    </strong>
                    <span
                      className={cn(
                        "block truncate text-xs",
                        selected ? "text-white/75" : "text-muted-foreground",
                      )}
                    >
                      {manageBucket === "ghost"
                        ? String(item.sku)
                        : `${String(item.sku)} · ${String(item.branch_name)}`}
                    </span>
                  </span>
                  <span className="shrink-0 text-right text-xs">
                    {manageBucket ? (
                      <>
                        <strong className="block text-base">
                          {Number(
                            item[
                              manageBucket === "real" ? "qty_real" : "qty_ghost"
                            ] || 0,
                          ).toLocaleString("th-TH")}
                        </strong>
                        {manageBucket === "real" ? realLabel : "สต๊อกผี"}
                      </>
                    ) : (
                      <>
                        <strong className="block text-base">
                          {Number(item.qty_real).toLocaleString("th-TH")}
                        </strong>
                        จริง /{" "}
                        {Number(item.qty_ghost || 0).toLocaleString("th-TH")} ผี
                      </>
                    )}
                  </span>
                </span>
              </button>
            );
          })}
          {inventory.length === 0 ? <EmptyState description="ไม่พบสต๊อกตามเงื่อนไข" /> : null}
        </div>
        <div className="shrink-0 border-t p-4">
          <ListPagination
            onPageChange={(nextPage) => applyFilters(nextPage)}
            onPageSizeChange={(nextSize) => applyFilters(1, branchId, undefined, nextSize)}
            page={resolvedPagination.page}
            pageSize={resolvedPagination.page_size}
            stacked
            total={resolvedPagination.total}
            totalPages={resolvedPagination.total_pages}
          />
        </div>
      </aside>

      <section className="min-w-0 p-5 lg:p-7">
        <div className="mb-6 border-b pb-5">
          <div className="min-w-0">
            <h2 className="text-2xl font-black">{String(selectedInventory?.product_name || "เลือกรายการสินค้า")}</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {String(selectedInventory?.sku || "")}
              {selectedInventory && manageBucket !== "ghost"
                ? ` · ${String(selectedInventory.branch_name)} · ${currency(Number(selectedInventory.price || 0))}`
                : ""}
            </p>
          </div>
          {selectedInventory ? (
            <div className="mt-4 flex justify-center">
              <ProductImageSwiper
                available={Boolean(selectedInventory.image_available)}
                branchId={manageBucket === "ghost" ? undefined : String(selectedInventory.branch_id)}
                imageCount={Number(selectedInventory.image_count || 0)}
                onImageAdded={manageBucket === "ghost" ? undefined : () => startTransition(() => router.refresh())}
                productId={String(selectedInventory.product_id)}
                productName={String(selectedInventory.product_name)}
              />
            </div>
          ) : null}
          {/* Catalog-level actions (add product / edit product details) live on
              the "รายการสินค้า" screen, and stock intake lives on "ใบสั่งซื้อเข้า" —
              this screen only owns per-branch stock. */}
          {selectedInventory ? (
            <div className="mt-4 flex flex-wrap gap-2">
              <Button onClick={openLots} type="button" variant="secondary">
                <Layers3 className="h-4 w-4" />
                ดู Lot/วันหมดอายุ
              </Button>
              {manageBucket !== "ghost" ? (
                <>
                  <Button
                    onClick={openBranchSettings}
                    type="button"
                    variant="secondary"
                  >
                    <Pencil className="h-4 w-4" />
                    ตั้งค่าเฉพาะสาขา
                  </Button>
                  <Button onClick={openDeleteProduct} type="button" variant="destructive">
                    <Trash2 className="h-4 w-4" />
                    ลบสินค้า
                  </Button>
                </>
              ) : null}
            </div>
          ) : null}
        </div>

        {/* Everything here is read-only. Reading a stock figure and editing it
            are different jobs: this panel answers "what is this?", and every
            change goes through ตั้งค่าเฉพาะสาขา, where an adjustment has to name
            the lot it touches and be confirmed before it is written. */}
        <div className="grid gap-4 md:grid-cols-2">
          <ReadOnlyField
            className="md:col-span-2"
            label="ชื่อสินค้า"
            value={
              selectedInventory
                ? `${String(selectedInventory.product_name)} · ${String(selectedInventory.sku)}`
                : "-"
            }
          />
          {manageBucket !== "ghost" ? (
            <ReadOnlyField label="สาขา" value={String(selectedInventory?.branch_name || branchLabel || "-")} />
          ) : null}
          <ReadOnlyField
            label={activeBucket === "ghost" ? "จำนวนในสต๊อก (สต๊อกผี)" : `จำนวนในสต๊อก (${realLabel})`}
            value={`${currentBucketQty.toLocaleString("th-TH")} ชิ้น`}
          />
          {/* Newest lot, not the FEFO-nearest one: this line answers "when did
              stock last come in", which is the question someone standing at the
              shelf is actually asking. */}
          <ReadOnlyField label="Lot ล่าสุด" value={latestLotLabel} />
          <ReadOnlyField label="วันหมดอายุ" value={latestLotExpiryLabel} />
          {manageBucket !== "ghost" ? (
            <>
              <ReadOnlyField
                label="ราคาตั้งต้น"
                value={currency(Number(selectedInventory?.base_selling_price || 0))}
              />
              <ReadOnlyField
                label="ราคาขาย"
                value={currency(Number(selectedInventory?.price || 0))}
              />
            </>
          ) : null}
        </div>
        {message ? (
          <p className="mt-4 rounded-2xl bg-muted px-4 py-3 text-sm">
            {message}
          </p>
        ) : null}
        {selectedInventory ? (
          <div className="mt-6 rounded-2xl border">
            <div className="border-b px-4 py-3"><p className="font-bold">ประวัติ movement ล่าสุด</p><p className="text-xs text-muted-foreground">บิลที่ถูกซ่อน, internal reversal และ Ghost จะไม่แสดงแก่บัญชีที่ไม่ใช่ Superadmin</p></div>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px] text-sm">
                <thead className="bg-muted/60 text-left"><tr><th className="p-3">วันที่</th><th className="p-3">รายการ</th><th className="p-3">ประเภท</th><th className="p-3 text-right">จำนวน</th><th className="p-3">หมายเหตุ</th></tr></thead>
                <tbody>
                  {movements.map((movement) => (
                    <tr className="border-t" key={String(movement.id)}><td className="whitespace-nowrap p-3">{showFullTimestamp ? new Date(String(movement.created_at)).toLocaleString("th-TH", { timeZone: "Asia/Bangkok" }) : new Date(String(movement.created_at)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })}</td><td className="p-3">{String(movement.movement_type)}</td><td className="p-3">{String(movement.stock_bucket) === "ghost" ? "Ghost Stock (สต๊อกผี)" : canManageGhost ? "Real Stock (สต๊อกจริง)" : "สต๊อก"}</td><td className={`p-3 text-right font-semibold ${Number(movement.quantity_delta) < 0 ? "text-red-700" : "text-emerald-700"}`}>{Number(movement.quantity_delta) > 0 ? "+" : ""}{Number(movement.quantity_delta).toLocaleString("th-TH")}</td><td className="p-3 text-muted-foreground">{String(movement.note || "-")}</td></tr>
                  ))}
                  {movements.length === 0 ? <tr><td className="p-8 text-center text-muted-foreground" colSpan={5}>ยังไม่มีประวัติ movement ที่มองเห็นได้</td></tr> : null}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </section>
      <Dialog onOpenChange={setLotsOpen} open={lotsOpen}>
        <DialogContent className="max-w-5xl">
          <DialogHeader
            description={manageBucket === "ghost"
              ? "สต๊อกผี · เรียงตาม FEFO"
              : `${String(selectedInventory?.branch_name || "")} · ${realLabel} · เรียงตาม FEFO`}
            title={`Lot ของ ${String(selectedInventory?.product_name || "สินค้า")}`}
          />
          {lotsLoading ? (
            <p className="p-10 text-center text-sm text-muted-foreground">
              กำลังโหลด Lot...
            </p>
          ) : (
            <div className="overflow-x-auto rounded-2xl border">
              <table className="w-full min-w-[850px] text-sm">
                <thead className="bg-muted text-left">
                  <tr>
                    <th className="p-3">Lot/Batch</th>
                    <th className="p-3">รับเข้า</th>
                    <th className="p-3">คงเหลือ</th>
                    <th className="p-3">ต้นทุน</th>
                    <th className="p-3">วันหมดอายุ</th>
                    <th className="p-3">สถานะ</th>
                    <th className="p-3">ต้นทาง</th>
                  </tr>
                </thead>
                <tbody>
                  {lots.map((lot) => (
                    <tr className="border-t" key={String(lot.id)}>
                      <td className="p-3 font-bold">
                        {String(lot.lot_number)}
                      </td>
                      <td className="p-3">
                        {Number(lot.received_quantity).toLocaleString("th-TH")}
                      </td>
                      <td className="p-3 font-bold">
                        {Number(lot.remaining_quantity).toLocaleString("th-TH")}
                      </td>
                      <td className="p-3">
                        {currency(Number(lot.unit_cost || 0))}
                      </td>
                      <td className="p-3">
                        {lot.expires_on
                          ? new Date(String(lot.expires_on)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })
                          : "ไม่กำหนด"}
                      </td>
                      <td className="p-3">
                        {lot.expiry_status === "expired"
                          ? "หมดอายุ"
                          : lot.expiry_status === "expiring"
                            ? "ใกล้หมดอายุ"
                            : "ปกติ"}
                      </td>
                      <td className="p-3">
                        {String(lot.po_number || lot.source_type || "-")}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {lots.length === 0 ? <EmptyState className="p-10" description="ยังไม่มี Lot ใน bucket นี้" /> : null}
            </div>
          )}
        </DialogContent>
      </Dialog>
      {manageBucket !== "ghost" ? (
      <Dialog onOpenChange={setBranchSettingsOpen} open={branchSettingsOpen}>
        <DialogContent className="max-w-xl">
          <DialogHeader
            description="ราคาขายเว้นว่างเพื่อสืบทอดราคาตั้งต้นจากโกดังแบบอัตโนมัติ ค่าอื่นมีผลเฉพาะสินค้าและสาขาที่เลือก"
            title={`ตั้งค่าเฉพาะสาขา ${String(selectedInventory?.branch_name || "")}`}
          />
          <div className="grid gap-4">
            <div className="rounded-2xl bg-muted p-4 text-sm">
              <span className="text-muted-foreground">ราคาตั้งต้นจากโกดัง</span>
              <strong className="ml-2">{currency(Number(branchSettings.warehouse_price || 0))}</strong>
              <span className="ml-2 text-muted-foreground">· {branchSettings.selling_price_source === "branch_override" ? "กำลังใช้ราคาสาขา" : "กำลังสืบทอดราคาโกดัง"}</span>
            </div>
            <Field label="ราคาขายเฉพาะสาขา (บาท)">
              <div className="flex gap-2">
                <Input
                  disabled={String(selectedBranch?.branch_type) === "main_warehouse"}
                  min="0"
                  onChange={(event) => setBranchSettings({ ...branchSettings, selling_price: event.target.value })}
                  placeholder="ใช้ราคาตั้งต้นจากโกดัง"
                  step="0.01"
                  type="number"
                  value={branchSettings.selling_price}
                />
                <Button
                  disabled={String(selectedBranch?.branch_type) === "main_warehouse"}
                  onClick={() => setBranchSettings({ ...branchSettings, selling_price: "", selling_price_source: "warehouse" })}
                  type="button"
                  variant="secondary"
                >
                  กลับมาใช้ราคาโกดัง
                </Button>
              </div>
            </Field>
            <Field label="ส่วนลด — ลดได้สูงสุด/หน่วย (บาท)">
              <Input
                min="0"
                onChange={(event) =>
                  setBranchSettings({
                    ...branchSettings,
                    max_discount_amount: event.target.value,
                  })
                }
                placeholder="ใช้ค่ากลาง"
                step="0.01"
                type="number"
                value={branchSettings.max_discount_amount}
              />
            </Field>
            {/* Stock adjustment sits in its own block with its own button: it
                posts to a different endpoint than the settings above, takes
                effect immediately, and is written to the audit trail — so it
                should not ride along on a "save settings" click. */}
            <div className="space-y-3 rounded-2xl border border-warning-200 bg-warning-50/40 p-4">
              <div>
                <p className="text-sm font-semibold">จำนวนในสต๊อก</p>
                <p className="text-xs text-muted-foreground">
                  เลือก lot ที่นับแล้วระบุจำนวนที่เปลี่ยน (ใส่ค่าติดลบเพื่อลดยอด) — ระบบจะบันทึกลงประวัติระบบทุกครั้ง
                </p>
              </div>
              <Field hint={`${activeBucket === "ghost" ? "สต๊อกผี" : realLabel} · ${lots.length.toLocaleString("th-TH")} lot ที่มีของ`} label="เลือก lot ที่ต้องการปรับ">
                <Select
                  aria-label="เลือก lot ที่ต้องการปรับ"
                  onChange={(event) => setAdjustLotId(event.target.value)}
                  value={adjustLotId}
                >
                  <option value="">— เลือก lot —</option>
                  {lots.map((lot) => (
                    <option key={String(lot.id)} value={String(lot.id)}>
                      {String(lot.lot_number)} · คงเหลือ {Number(lot.remaining_quantity).toLocaleString("th-TH")} ชิ้น
                      {lot.expires_on ? ` · หมดอายุ ${thaiDate(lot.expires_on)}` : ""}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field hint="ใส่จำนวนที่เพิ่มขึ้นหรือลดลง เช่น 5 หรือ -3" label="จำนวนที่เปลี่ยน (+/-)">
                <Input
                  aria-label="จำนวนที่เปลี่ยน"
                  onChange={(event) => setAdjustDelta(event.target.value)}
                  placeholder="เช่น -3"
                  step="1"
                  type="number"
                  value={adjustDelta}
                />
              </Field>
              <Field label="เหตุผลการปรับยอด">
                <Input
                  aria-label="เหตุผลการปรับยอด"
                  onChange={(event) => setAdjustReason(event.target.value)}
                  placeholder="เช่น ตรวจนับประจำเดือน พบของชำรุด"
                  value={adjustReason}
                />
              </Field>
              <label className="flex items-start gap-2 text-sm">
                <Checkbox
                  aria-label="ยืนยันการปรับยอดสต๊อก"
                  checked={adjustConfirmed}
                  className="mt-0.5"
                  onChange={(event) => setAdjustConfirmed(event.target.checked)}
                />
                <span>
                  ยืนยันว่าได้ตรวจนับของจริงแล้ว และต้องการปรับยอด lot นี้
                  {adjustLotId && adjustDelta ? (
                    <strong className="ml-1">
                      ({Number(adjustDelta) > 0 ? "+" : ""}{Number(adjustDelta).toLocaleString("th-TH")} ชิ้น)
                    </strong>
                  ) : null}
                </span>
              </label>
              {adjustError ? <Notice tone="error">{adjustError}</Notice> : null}
              <Button
                disabled={
                  adjustBusy ||
                  !adjustLotId ||
                  !adjustReason.trim() ||
                  !adjustConfirmed ||
                  !Number(adjustDelta)
                }
                onClick={() => void submitLotAdjustment()}
                type="button"
              >
                {adjustBusy ? "กำลังบันทึก..." : "บันทึกการปรับยอด"}
              </Button>
            </div>

            <p className="text-sm font-semibold">แจ้งเตือนสต๊อกใกล้หมด</p>
            <Field label={`จุดเตือน${realLabel}`}>
              <Input
                min="0"
                onChange={(event) =>
                  setBranchSettings({
                    ...branchSettings,
                    low_stock_real_threshold: event.target.value,
                  })
                }
                placeholder="ใช้ค่ากลาง"
                type="number"
                value={branchSettings.low_stock_real_threshold}
              />
            </Field>
            {canManageGhost ? (
              <Field label="จุดเตือนสต๊อกผี">
                <Input
                  min="0"
                  onChange={(event) =>
                    setBranchSettings({
                      ...branchSettings,
                      low_stock_ghost_threshold: event.target.value,
                    })
                  }
                  placeholder="ใช้ค่ากลาง"
                  type="number"
                  value={branchSettings.low_stock_ghost_threshold}
                />
              </Field>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                onClick={() => setBranchSettingsOpen(false)}
                type="button"
                variant="secondary"
              >
                ยกเลิก
              </Button>
              <Button onClick={saveBranchSettings} type="button">
                บันทึก
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
      ) : null}
      {manageBucket !== "ghost" ? (
      <Dialog onOpenChange={setDeleteOpen} open={deleteOpen}>
        <DialogContent>
          <DialogHeader
            description="ข้อมูลทั้งหมดด้านล่างจะถูกลบถาวรและไม่สามารถย้อนกลับได้"
            title={`ลบสินค้า ${String(selectedInventory?.product_name || "")}`}
          />
          <div className="space-y-4">
            <div className="rounded-2xl bg-destructive/10 p-4 text-sm">
              {Object.entries(deleteImpact?.counts || {}).map(([key, count]) => (
                <div className="flex justify-between py-1" key={key}>
                  <span>{key}</span>
                  <strong>{count}</strong>
                </div>
              ))}
            </div>
            <p className="text-sm">
              พิมพ์ <strong>{deleteImpact?.confirmation}</strong> เพื่อยืนยัน
            </p>
            <Input aria-label="ข้อความยืนยันการลบ" onChange={(event) => setDeleteText(event.target.value)} value={deleteText} />
            <div className="flex justify-end gap-2">
              <Button onClick={() => setDeleteOpen(false)} type="button" variant="secondary">
                ยกเลิก
              </Button>
              <Button
                disabled={deleteText !== deleteImpact?.confirmation}
                onClick={() => void confirmDeleteProduct()}
                type="button"
                variant="destructive"
              >
                ลบถาวร
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
      ) : null}
      </section>
    </div>
  );
}
