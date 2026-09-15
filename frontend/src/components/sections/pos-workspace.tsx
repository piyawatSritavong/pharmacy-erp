"use client";

import Image from "next/image";
import Link from "next/link";
import { CheckCircle2, Minus, Package, PauseCircle, Plus, Printer, Search, ShoppingCart, Trash2, X } from "lucide-react";
import { startTransition, useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";

import { Field } from "@/components/ui/field";
import { PosCart } from "@/components/sections/pos-cart";
import { Button, CheckboxField, Dialog, DialogContent, DialogHeader, EmptyState, Input, Notice, Select } from "@/components/ui/primitives";
import { cn, currency } from "@/lib/utils";
import { RESUME_KEY } from "@/components/sections/parked-bills-console";
import { proxyClient } from "@/services/api";

/** The grid pages in from the server rather than shipping the whole catalogue
 *  (and one image request per product) on first paint. */
const GRID_PAGE_SIZE = 18;

type Option = Record<string, unknown>;

type CartLine = {
  product: Option;
  lot: Option;
  quantity: number;
  // Selling unit chosen by the cashier; empty means the product's base unit.
  unitId: string;
  discount: string;
};

type Preview = {
  lines: Option[];
  summary: {
    subtotal: number;
    tax_amount: number;
    total_amount: number;
    cash_amount?: number;
    transfer_amount?: number;
    tendered_amount?: number;
    change_amount?: number;
    line_discount_total?: number;
    bill_discount_amount?: number;
    promotion_discount_total?: number;
    discount_total?: number;
  };
  applied_promotions?: Option[];
};

// The POS portal only receives availability_status; exact counts stay with the
// back office. Fall back to qty_real when a privileged role opens this screen.
function isSellable(stock?: Option) {
  if (!stock) return false;
  if (stock.qty_real !== undefined) return Number(stock.qty_real) > 0;
  return String(stock.availability_status || "") !== "out_of_stock";
}

function stockLabel(stock: Option | undefined, unitName: string) {
  if (!stock) return "ไม่มีข้อมูลสต๊อก";
  if (stock.qty_real !== undefined) return `คงเหลือ ${Number(stock.qty_real)} ${unitName}`;
  switch (String(stock.availability_status || "")) {
    case "out_of_stock":
      return "หมดสต๊อก";
    case "low_stock":
      return "เหลือน้อย";
    default:
      return "พร้อมขาย";
  }
}

const PROMO_TYPE_LABELS: Record<string, string> = {
  buy_x_get_y: "ซื้อ X แถม Y",
  percent: "ลด %",
  amount: "ลดเงิน",
  bundle: "ราคาชุด",
  bill_giveaway: "ของแถมท้ายบิล"
};

function promoTypeLabel(value: string) {
  return PROMO_TYPE_LABELS[value] || "โปรโมชั่น";
}

function parseMoneyCents(value: string) {
  const trimmed = value.trim();
  if (!/^\d+(?:\.\d{0,2})?$/.test(trimmed)) return null;
  const amount = Number(trimmed);
  if (!Number.isFinite(amount)) return null;
  return Math.round(amount * 100);
}

export function PosWorkspace({
  branchId,
  branchName,
  products,
  inventory,
  // The POS portal sells its own branch through /pos/*; head office sells in a
  // branch's name through /admin/pos/*. Same cart, same bill, different door.
  endpointBase = "/pos",
  remoteBranchId = "",
  watchRemote = false,
}: {
  branchId: string;
  branchName: string;
  products: Option[];
  inventory: Option[];
  endpointBase?: string;
  /** รีโมตหน้าร้าน (head office): the cart is pushed to this branch's till and
   *  paid for there, rather than being settled here. */
  remoteBranchId?: string;
  /** POS: watch for a cart head office has left waiting at this till. */
  watchRemote?: boolean;
}) {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [cart, setCart] = useState<CartLine[]>([]);
  const [billDiscount, setBillDiscount] = useState("");
  const [lotProduct, setLotProduct] = useState<Option | null>(null);
  const [lotOptions, setLotOptions] = useState<Option[]>([]);
  const [lotsLoading, setLotsLoading] = useState(false);
  const [fullTaxInvoice, setFullTaxInvoice] = useState(false);
  const [customerName, setCustomerName] = useState("");
  const [customerTaxId, setCustomerTaxId] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [message, setMessage] = useState("");
  const [paymentOpen, setPaymentOpen] = useState(false);
  const [paymentType, setPaymentType] = useState<"cash" | "bank_transfer" | "mixed">("cash");
  const [tendered, setTendered] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [paymentChecking, setPaymentChecking] = useState(false);
  const [paymentReady, setPaymentReady] = useState(false);
  const [paymentError, setPaymentError] = useState("");
  const [receipt, setReceipt] = useState<Option | null>(null);
  const [cartOpen, setCartOpen] = useState(false);
  // Active, in-date promotions and the shortcut a cashier has tapped to narrow
  // the grid to that promotion's eligible products.
  const [promotions, setPromotions] = useState<Option[]>([]);
  const [activePromoId, setActivePromoId] = useState("");
  // Grid paging: the server filters and pages, the sentinel below the grid asks
  // for the next 18 as it scrolls into view.
  const [gridItems, setGridItems] = useState<Option[]>(products);
  const [gridPage, setGridPage] = useState(1);
  const [gridDone, setGridDone] = useState(products.length < GRID_PAGE_SIZE);
  const [gridLoading, setGridLoading] = useState(false);
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  // The branch till's side of a remote sale, as head office sees it.
  const [remoteSession, setRemoteSession] = useState<Option | null>(null);
  const remoteStatus = String(remoteSession?.status || "");
  // The till's side: when head office has a cart open here, it fills this very
  // cart (so the screen reads exactly as an ordinary sale) and locks editing —
  // the server settles the cart it holds, so the two screens cannot disagree.
  const [remoteLock, setRemoteLock] = useState<{ id: string; operator: string } | null>(null);
  const remoteLockRef = useRef("");
  const remoteCartSignature = useRef("");
  // พักบิล — suspend the cart without touching stock, resume it later.
  const [parking, setParking] = useState(false);
  const [parkNote, setParkNote] = useState("");
  const [parkOpen, setParkOpen] = useState(false);
  const mixedCashFocusSnapshot = useRef<{
    transferCents: number;
    requiredCashCents: number;
  } | null>(null);

  const inventoryByProduct = new Map(
    inventory.map((item) => [String(item.product_id), item])
  );
  const activePromo = promotions.find((promo) => String(promo.id) === activePromoId);
  const promoProductIds = new Set(
    ((activePromo?.items as Option[]) || []).map((item) => String(item.product_id))
  );
  // A stable key for the promo's product set, so the loader below is not
  // rebuilt on every render by a freshly-allocated Set.
  const promoIdsKey = [...promoProductIds].sort().join(",");

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  const loadProducts = useCallback(
    async (page: number, replace: boolean) => {
      setGridLoading(true);
      try {
        const query = new URLSearchParams({ active: "true", page: String(page) });
        if (branchId) query.set("branch_id", branchId);
        if (promoIdsKey) {
          // A promotion covers a known, small set — ask for exactly those
          // rather than paging the catalogue looking for them.
          query.set("ids", promoIdsKey);
          query.set("page_size", "200");
        } else {
          query.set("page_size", String(GRID_PAGE_SIZE));
          if (debouncedSearch) query.set("search", debouncedSearch);
        }
        const response = await proxyClient<{ items: Option[] }>(`/products?${query.toString()}`);
        const items = response.items || [];
        setGridItems((current) => (replace ? items : [...current, ...items]));
        setGridDone(Boolean(promoIdsKey) || items.length < GRID_PAGE_SIZE);
        setGridPage(page);
      } catch {
        // A failed page stops the scroll from asking again in a loop.
        setGridDone(true);
      } finally {
        setGridLoading(false);
      }
    },
    [branchId, debouncedSearch, promoIdsKey]
  );

  // Any change of branch, search term or promotion starts the list over.
  useEffect(() => {
    void loadProducts(1, true);
  }, [loadProducts]);

  useEffect(() => {
    const node = sentinelRef.current;
    if (!node || gridDone || gridLoading) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) void loadProducts(gridPage + 1, false);
      },
      { rootMargin: "200px" }
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [gridDone, gridLoading, gridPage, loadProducts]);

  // Head office pushes the cart itself — not its clicks — so the till shows the
  // same bill however the two screens differ, and it survives a refresh.
  useEffect(() => {
    if (!remoteBranchId) return;
    const timer = window.setTimeout(() => {
      void proxyClient<Option>("/admin/pos/remote-session", {
        method: "PUT",
        body: JSON.stringify({
          branch_id: remoteBranchId,
          cart: {
            lines: cart.map((line) => ({
              product_id: String(line.product.id),
              product_name: String(line.product.name || ""),
              sku: String(line.product.sku || ""),
              inventory_lot_id: String(line.lot.id),
              lot_number: String(line.lot.lot_number || ""),
              stock_bucket: "real",
              quantity: line.quantity,
              unit_price: Number(line.product.effective_price || 0),
              discount_amount: Number(line.discount || 0)
            })),
            bill_discount_amount: Number(billDiscount || 0),
            full_tax_invoice: fullTaxInvoice,
            customer_name: fullTaxInvoice ? customerName : "",
            customer_tax_id: fullTaxInvoice ? customerTaxId : ""
          }
        })
      })
        .then(setRemoteSession)
        .catch(() => {
          /* the next edit pushes again */
        });
    }, 600);
    return () => window.clearTimeout(timer);
  }, [billDiscount, cart, customerName, customerTaxId, fullTaxInvoice, remoteBranchId]);

  // ...and watches for the till to take the money.
  useEffect(() => {
    if (!remoteBranchId) return;
    const load = () =>
      void proxyClient<{ item: Option | null }>(`/admin/pos/remote-session?branch_id=${encodeURIComponent(remoteBranchId)}`)
        .then((response) => setRemoteSession(response.item))
        .catch(() => {});
    const timer = window.setInterval(load, 3000);
    return () => window.clearInterval(timer);
  }, [remoteBranchId]);

  // Once the branch has been paid, clear the till here so the next customer
  // starts clean rather than re-sending a bill that is already settled.
  useEffect(() => {
    if (remoteStatus !== "completed") return;
    setCart([]);
    setBillDiscount("");
    setMessage(`สาขารับชำระแล้ว · ${String(remoteSession?.invoice_number || "")}`);
  }, [remoteSession?.invoice_number, remoteStatus]);

  useEffect(() => {
    if (!watchRemote) return;
    const load = () =>
      void proxyClient<{ item: Option | null }>("/pos/remote-session")
        .then((response) => {
          const item = response.item;
          // An open session whose cart is still empty is head office mid-build,
          // not a bill to take. Treating it as one wiped the cashier's own cart
          // on every poll and locked the till against a bill that did not exist.
          const openSession =
            item &&
            String(item.status) === "open" &&
            (((item.cart as Option)?.lines as Option[]) || []).length > 0
              ? item
              : null;
          if (!openSession) {
            // Head office withdrew it, or the sale is paid: hand the till back.
            if (remoteLockRef.current) {
              remoteLockRef.current = "";
              remoteCartSignature.current = "";
              setRemoteLock(null);
              setCart([]);
              setBillDiscount("");
            }
            return;
          }
          const cartData = (openSession.cart as Option) || {};
          const lines = (cartData.lines as Option[]) || [];
          const signature = JSON.stringify(lines) + String(cartData.bill_discount_amount || "");
          remoteLockRef.current = String(openSession.id);
          setRemoteLock({ id: String(openSession.id), operator: String(openSession.operator_name || "") });
          // Only rewrite the cart when it actually changed, so a three-second
          // poll does not restart the row animations under the cashier.
          if (signature === remoteCartSignature.current) return;
          remoteCartSignature.current = signature;
          setCart(
            lines.map((line) => ({
              product: {
                id: String(line.product_id),
                name: String(line.product_name || ""),
                sku: String(line.sku || ""),
                effective_price: Number(line.unit_price || 0)
              },
              lot: { id: String(line.inventory_lot_id), lot_number: String(line.lot_number || "") },
              quantity: Number(line.quantity || 0),
              unitId: "",
              discount: Number(line.discount_amount || 0) ? String(line.discount_amount) : ""
            }))
          );
          setBillDiscount(Number(cartData.bill_discount_amount || 0) ? String(cartData.bill_discount_amount) : "");
          setFullTaxInvoice(Boolean(cartData.full_tax_invoice));
        })
        .catch(() => {
          /* the next tick retries */
        });
    load();
    const timer = window.setInterval(load, 3000);
    return () => window.clearInterval(timer);
  }, [watchRemote]);

  const filteredProducts = gridItems;

  function payload(extra?: { payment_type?: string; tendered_amount?: number; transfer_amount?: number }) {
    return {
      branch_id: branchId,
      customer_name: fullTaxInvoice ? customerName : "",
      customer_tax_id: fullTaxInvoice ? customerTaxId : "",
      is_government_mode: false,
      full_tax_invoice: fullTaxInvoice,
      payment_type: extra?.payment_type || paymentType,
      tendered_amount: extra?.tendered_amount ?? Number(tendered || 0),
      transfer_amount: extra?.transfer_amount ?? Number(transferAmount || 0),
      bill_discount_amount: Number(billDiscount || 0),
      items: cart.map((line) => ({
        product_id: String(line.product.id),
        inventory_lot_id: String(line.lot.id),
        quantity: line.quantity,
        unit_id: line.unitId,
        discount_amount: Number(line.discount || 0),
        stock_bucket: "real"
      }))
    };
  }

  useEffect(() => {
    if (!cart.length) {
      setPreview(null);
      return;
    }
    const timer = window.setTimeout(() => {
      void proxyClient<Preview>("/invoices/preview", {
        method: "POST",
        body: JSON.stringify(payload())
      })
        .then(setPreview)
        .catch((error) => setMessage(error instanceof Error ? error.message : "คำนวณยอดไม่สำเร็จ"));
    }, 250);
    return () => window.clearTimeout(timer);
    // payload intentionally follows every cart and tax-document field.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [branchId, cart, billDiscount, customerName, customerTaxId, fullTaxInvoice]);

  useEffect(() => {
    // Active, in-date promotions for the shortcut bar; the backend already
    // filters out expired ones when active_only is set.
    void proxyClient<{ items: Option[] }>("/promotions?active_only=true")
      .then((response) => setPromotions(response.items || []))
      .catch(() => setPromotions([]));
  }, []);

  async function chooseProductLot(product: Option) {
    setMessage("");
    setReceipt(null);
    if (!isSellable(inventoryByProduct.get(String(product.id)))) {
      setMessage("สินค้านี้หมดสต๊อก");
      return;
    }
    setLotProduct(product);
    setLotOptions([]);
    setLotsLoading(true);
    try {
      const query = new URLSearchParams({ branch_id: branchId, product_id: String(product.id), stock_bucket: "real" });
      const result = await proxyClient<{ items: Option[] }>(`/sales/lot-options?${query.toString()}`);
      setLotOptions(result.items);
      if (!result.items.length) setMessage("ไม่มี Lot ที่พร้อมขายสำหรับสินค้านี้");
    } catch (caught) {
      setLotProduct(null);
      setMessage(caught instanceof Error ? caught.message : "โหลด Lot ไม่สำเร็จ");
    } finally {
      setLotsLoading(false);
    }
  }

  function addSelectedLot(lot: Option) {
    if (!lotProduct) return;
    setCart((current) => {
      const key = `${String(lotProduct.id)}:${String(lot.id)}`;
      const existing = current.find((line) => cartLineKey(line) === key);
      if (existing) {
        return current.map((line) =>
          cartLineKey(line) === key ? { ...line, quantity: line.quantity + 1 } : line
        );
      }
      return [...current, { product: lotProduct, lot, quantity: 1, unitId: "", discount: "" }];
    });
    setLotProduct(null);
    setCartOpen(true);
  }

  function updateLine(key: string, patch: Partial<CartLine>) {
    setCart((current) =>
      current.map((line) => (cartLineKey(line) === key ? { ...line, ...patch } : line))
    );
  }

  // Promotional rewards come back from the backend as extra preview lines.
  const giveawayLines = (preview?.lines || []).filter((item) => Boolean(item.is_giveaway));

  function cartLineKey(line: CartLine) {
    return `${String(line.product.id)}:${String(line.lot.id)}`;
  }

  const paymentTotalCents = Math.round(Number(preview?.summary.total_amount || 0) * 100);
  const tenderedCents = parseMoneyCents(tendered);
  const transferCents = parseMoneyCents(transferAmount);
  const cashDueCents = paymentType === "mixed" && transferCents != null
    ? Math.max(0, paymentTotalCents - transferCents)
    : paymentType === "cash"
      ? paymentTotalCents
      : 0;
  const localChangeCents = tenderedCents == null
    ? 0
    : paymentType === "mixed" && transferCents != null
      ? Math.max(0, tenderedCents + transferCents - paymentTotalCents)
      : Math.max(0, tenderedCents - cashDueCents);

  function updatePositiveMoney(setter: (value: string) => void, value: string) {
    if (value === "" || /^\d+(?:\.\d{0,2})?$/.test(value)) setter(value);
  }

  function captureMixedCashSnapshot() {
    if (transferCents == null || transferCents <= 0 || transferCents >= paymentTotalCents) {
      mixedCashFocusSnapshot.current = null;
      return;
    }
    mixedCashFocusSnapshot.current = {
      transferCents,
      requiredCashCents: paymentTotalCents - transferCents
    };
  }

  function updateMixedCash(value: string) {
    if (value !== "" && !/^\d+(?:\.\d{0,2})?$/.test(value)) return;
    setTendered(value);

    const nextCashCents = parseMoneyCents(value);
    const snapshot = mixedCashFocusSnapshot.current;
    if (nextCashCents == null || nextCashCents <= 0 || !snapshot) return;

    if (nextCashCents < snapshot.requiredCashCents) {
      setTransferAmount(((paymentTotalCents - nextCashCents) / 100).toFixed(2));
      return;
    }
    setTransferAmount((snapshot.transferCents / 100).toFixed(2));
  }

  function updateMixedTransfer(value: string) {
    if (value !== "" && !/^\d+(?:\.\d{0,2})?$/.test(value)) return;
    setTransferAmount(value);
    mixedCashFocusSnapshot.current = null;

    const nextTransferCents = parseMoneyCents(value);
    if (nextTransferCents == null || nextTransferCents <= 0 || nextTransferCents >= paymentTotalCents) return;
    setTendered(((paymentTotalCents - nextTransferCents) / 100).toFixed(2));
  }

  function configurePayment(nextType: typeof paymentType, totalCents = paymentTotalCents) {
    setPaymentType(nextType);
    setPaymentReady(false);
    setPaymentError("");
    mixedCashFocusSnapshot.current = null;
    if (nextType === "cash") {
      setTendered((totalCents / 100).toFixed(2));
      setTransferAmount("");
      return;
    }
    if (nextType === "bank_transfer") {
      setTendered("");
      setTransferAmount("");
      return;
    }
    const nextTransferCents = Math.max(1, Math.min(totalCents - 1, Math.round(totalCents / 2)));
    setTransferAmount((nextTransferCents / 100).toFixed(2));
    setTendered(((totalCents - nextTransferCents) / 100).toFixed(2));
  }

  // Resuming a parked bill: the list page stashes the saved lines in
  // sessionStorage and navigates here. Rebuild the cart from the snapshot,
  // then clear the handoff so a refresh doesn't re-add it.
  useEffect(() => {
    const raw = window.sessionStorage.getItem(RESUME_KEY);
    if (!raw) return;
    window.sessionStorage.removeItem(RESUME_KEY);
    try {
      const bill = JSON.parse(raw) as {
        customer_name?: string;
        customer_tax_id?: string;
        full_tax_invoice?: boolean;
        items?: Array<Record<string, unknown>>;
      };
      const restored: CartLine[] = (bill.items || []).map((item) => ({
        product: {
          id: String(item.product_id),
          name: String(item.product_name || ""),
          sku: String(item.sku || ""),
          price: Number(item.unit_price || 0)
        },
        lot: { id: String(item.inventory_lot_id), lot_number: String(item.lot_number || "") },
        quantity: Number(item.quantity || 1),
        unitId: String(item.unit_id || ""),
        discount: item.discount_amount ? String(item.discount_amount) : ""
      }));
      if (!restored.length) return;
      setCart(restored);
      setCustomerName(String(bill.customer_name || ""));
      setCustomerTaxId(String(bill.customer_tax_id || ""));
      setFullTaxInvoice(Boolean(bill.full_tax_invoice));
      setCartOpen(true);
      setMessage("เรียกบิลที่พักไว้กลับมาแล้ว ตรวจสอบราคาและสต๊อกอีกครั้งก่อนชำระเงิน");
    } catch {
      // A malformed handoff just means no resume; the cart stays empty.
    }
  }, []);

  // Parking never calls the sales API: no invoice, no lot allocation, no
  // stock movement. It stores the cart as-is so the counter can serve the
  // next customer, and re-validates on checkout when it is resumed.
  async function parkBill() {
    if (!cart.length) return;
    setParking(true);
    try {
      const priced = (line: CartLine) =>
        preview?.lines.find(
          (item) =>
            String(item.product_id) === String(line.product.id) &&
            String(item.inventory_lot_id) === String(line.lot.id)
        );
      await proxyClient("/parked-bills", {
        method: "POST",
        body: JSON.stringify({
          customer_name: customerName,
          customer_tax_id: customerTaxId,
          full_tax_invoice: fullTaxInvoice,
          note: parkNote,
          items: cart.map((line) => ({
            product_id: String(line.product.id),
            inventory_lot_id: String(line.lot.id),
            quantity: line.quantity,
            unit_price: Number(priced(line)?.unit_price || line.product.price || 0),
            discount_amount: 0,
            product_name: String(line.product.name || ""),
            sku: String(line.product.sku || ""),
            lot_number: String(line.lot.lot_number || "")
          }))
        })
      });
      setCart([]);
      setCustomerName("");
      setCustomerTaxId("");
      setFullTaxInvoice(false);
      setParkNote("");
      setParkOpen(false);
      setMessage("พักบิลไว้แล้ว เปิดดูได้ที่เมนู พักบิล");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "พักบิลไม่สำเร็จ");
    } finally {
      setParking(false);
    }
  }

  async function openPayment() {
    if (!cart.length) {
      setMessage("กรุณาเพิ่มสินค้าอย่างน้อย 1 รายการ");
      return;
    }
    try {
      const currentPreview = preview || await proxyClient<Preview>("/invoices/preview", {
        method: "POST",
        body: JSON.stringify(payload())
      });
      setPreview(currentPreview);
      configurePayment(paymentType, Math.round(Number(currentPreview.summary.total_amount || 0) * 100));
      setPaymentOpen(true);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ตรวจสอบยอดชำระไม่สำเร็จ");
    }
  }

  useEffect(() => {
    if (!paymentOpen || paymentTotalCents <= 0) return;
    setPaymentReady(false);

    let validationError = "";
    if (fullTaxInvoice && !customerTaxId.trim()) {
      validationError = "กรุณาระบุเลขประจำตัวผู้เสียภาษี";
    } else if (paymentType === "cash" && (tenderedCents == null || tenderedCents <= 0)) {
      validationError = "เงินสดที่รับต้องมากกว่า 0";
    } else if (paymentType === "cash" && tenderedCents != null && tenderedCents < paymentTotalCents) {
      validationError = "เงินสดที่รับต้องไม่น้อยกว่ายอดที่ต้องชำระ";
    } else if (paymentType === "mixed") {
      if (transferCents == null || transferCents <= 0 || transferCents >= paymentTotalCents) {
        validationError = "ยอดเงินโอนต้องมากกว่า 0 และน้อยกว่ายอดรวม";
      } else if (tenderedCents == null || tenderedCents <= 0) {
        validationError = "ยอดเงินสดต้องมากกว่า 0";
      } else if (tenderedCents + transferCents < paymentTotalCents) {
        validationError = "ยอดเงินสดและยอดเงินโอนรวมกันน้อยกว่ายอดที่ต้องชำระ";
      }
    }
    if (validationError) {
      setPaymentChecking(false);
      setPaymentError(validationError);
      return;
    }

    let active = true;
    setPaymentChecking(true);
    setPaymentError("");
    const timer = window.setTimeout(() => {
      void proxyClient<Preview>(`${endpointBase}/preview`, {
        method: "POST",
        body: JSON.stringify(payload())
      })
        .then((checked) => {
          if (!active) return;
          setPreview(checked);
          setPaymentReady(true);
          setPaymentError("");
        })
        .catch((error) => {
          if (!active) return;
          setPaymentReady(false);
          setPaymentError(error instanceof Error ? error.message : "ตรวจสอบยอดชำระไม่สำเร็จ");
        })
        .finally(() => {
          if (active) setPaymentChecking(false);
        });
    }, 300);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
    // payload is intentionally rebuilt from the latest payment and cart state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    customerTaxId,
    fullTaxInvoice,
    paymentOpen,
    paymentTotalCents,
    paymentType,
    tendered,
    transferAmount
  ]);

  async function checkout() {
    setSubmitting(true);
    setMessage("");
    try {
      const result = remoteLock
        ? // The cart lives on the server; the till supplies only the payment.
          await proxyClient<Option>("/pos/remote-session/checkout", {
            method: "POST",
            body: JSON.stringify({
              payment_type: paymentType,
              tendered_amount: Number(tendered || 0),
              transfer_amount: Number(transferAmount || 0)
            })
          })
        : await proxyClient<Option>(`${endpointBase}/checkout`, {
            method: "POST",
            body: JSON.stringify(payload())
          });
      setReceipt(result);
      setCartOpen(false);
      setCart([]);
      setPaymentReady(false);
      setPaymentError("");
      setMessage("");
      startTransition(() => router.refresh());
    } catch (error) {
      setPaymentError(error instanceof Error ? error.message : "ชำระเงินไม่สำเร็จ");
    } finally {
      setSubmitting(false);
    }
  }

  function startNewSale() {
    setPaymentOpen(false);
    setReceipt(null);
    setCustomerName("");
    setCustomerTaxId("");
    setFullTaxInvoice(false);
    setPaymentType("cash");
    setTendered("");
    setTransferAmount("");
    setPaymentReady(false);
    setPaymentError("");
    setMessage("");
  }

  return (
    <div className="h-full min-h-0 overflow-hidden">
      <section className="grid h-full min-h-0 gap-3 xl:grid-cols-[minmax(0,1fr)_400px]">
        <div className="flex min-h-0 min-w-0 flex-col gap-3">
	          <div className="shrink-0 rounded-2xl border bg-white p-3 shadow-card">
	            <div className="grid grid-cols-[1fr_auto] gap-2 sm:flex sm:flex-col sm:gap-3 lg:flex-row lg:items-center">
	              <div className="shrink-0">
	                <p className="text-xs font-semibold text-primary">{branchName}</p>
	                <h1 className="text-lg font-bold">ขายหน้าร้าน</h1>
	              </div>
	              <label className="relative col-span-2 row-start-2 block min-w-0 flex-1">
                <Search className="absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  aria-label="ค้นหาสินค้า"
                  className="h-11 rounded-xl border-0 bg-muted pl-10 shadow-none sm:h-12 sm:rounded-full sm:pl-12"
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="ค้นหาชื่อสินค้า SKU หรือบาร์โค้ด..."
                  value={search}
                />
              </label>
              <div className="col-start-2 row-start-1 flex items-center gap-2">
                <Button className="relative rounded-xl sm:rounded-full xl:hidden" onClick={() => setCartOpen(true)} type="button">
                  <ShoppingCart className="h-4 w-4" />
                  ตะกร้า
                  {cart.length ? (
                    <span className="grid h-5 min-w-5 place-items-center rounded-full bg-white px-1 text-xs text-primary">
                      {cart.length}
                    </span>
                  ) : null}
                </Button>
              </div>
            </div>

          </div>

          {promotions.length > 0 ? (
            <div className="shrink-0">
              <div className="flex items-center gap-2 overflow-x-auto pb-1">
                <button
                  className={cn(
                    "shrink-0 rounded-full border px-3 py-1.5 text-xs font-semibold transition",
                    activePromoId ? "text-muted-foreground hover:bg-muted" : "border-primary bg-primary text-white"
                  )}
                  onClick={() => setActivePromoId("")}
                  type="button"
                >
                  ทั้งหมด
                </button>
                {promotions.map((promo) => {
                  const id = String(promo.id);
                  const active = activePromoId === id;
                  const productCount = ((promo.items as Option[]) || []).length;
                  return (
                    <button
                      className={cn(
                        "flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-semibold transition",
                        active ? "border-primary bg-primary text-white" : "hover:border-primary/40 hover:bg-muted"
                      )}
                      key={id}
                      onClick={() => setActivePromoId(active ? "" : id)}
                      type="button"
                      title={String(promo.name)}
                    >
                      <span className="max-w-40 truncate">{String(promo.name)}</span>
                      <span className={cn("rounded-full px-1.5 text-[10px]", active ? "bg-white/20" : "bg-primary/10 text-primary")}>{promoTypeLabel(String(promo.promo_type))}</span>
                      {productCount > 0 ? <span className={active ? "text-white/80" : "text-muted-foreground"}>· {productCount}</span> : null}
                    </button>
                  );
                })}
              </div>
            </div>
          ) : null}

          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1" data-testid="product-scroll-area">
            <div className="grid gap-2 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3">
              {filteredProducts.map((product) => {
              const stock = inventoryByProduct.get(String(product.id));
              return (
                <article className="overflow-hidden rounded-2xl border bg-white shadow-card" key={String(product.id)}>
                  <button
                    aria-label={`เพิ่ม ${String(product.name)} ลงตะกร้า`}
                    className="grid w-full grid-cols-[80px_minmax(0,1fr)] text-left sm:block"
                    onClick={() => void chooseProductLot(product)}
                    type="button"
                  >
                    <div className="relative h-full min-h-28 overflow-hidden sm:h-44 bg-gradient-to-br from-orange-50 to-amber-100">
                      {Boolean(product.image_available) ? (
                        <Image
                          alt={String(product.name)}
                          className="object-cover transition duration-300 hover:scale-105"
                          fill
                          sizes="(max-width: 639px) 80px, (max-width: 1023px) 50vw, 33vw"
                          src={`/api/backend/products/${String(product.id)}/image`}
                          unoptimized
                        />
                      ) : (
                        <div className="grid h-full place-items-center">
                          <Package className="h-10 w-10 text-primary/35 sm:h-16 sm:w-16" />
                        </div>
                      )}
                      {Number(product.image_count || 0) > 1 ? (
                        <span className="absolute bottom-2 right-2 rounded-full bg-black/70 px-2 py-1 text-xs font-bold text-white">
                          {Number(product.image_count)} รูป
                        </span>
                      ) : null}
                    </div>
                    <div className="min-w-0 p-2.5 sm:p-4">
                      <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-start sm:justify-between sm:gap-3">
                        <div>
                          <h2 className="break-words text-sm font-bold sm:text-base">{String(product.name)}</h2>
                          <p className="mt-1 break-all text-xs text-muted-foreground">
                            {String(product.sku)}
                          </p>
                        </div>
                        <span className="font-bold text-primary">
                          {currency(Number(product.effective_price || 0))}
                        </span>
                      </div>
                      <p className="mt-3 hidden min-h-10 text-sm sm:line-clamp-2 text-muted-foreground">
                        {String(product.description || "ไม่มีรายละเอียด")}
                      </p>
                      <div className="mt-2 flex flex-wrap items-center justify-between gap-1 text-xs sm:mt-4">
                        <span>{stockLabel(stock, String(product.unit_name || "ชิ้น"))}</span>
                        <span className="rounded-full bg-primary px-3 py-1.5 font-bold text-white">
                          {isSellable(stock) ? "+ เพิ่ม" : "หมด"}
                        </span>
                      </div>
                    </div>
                  </button>
                </article>
              );
              })}
            </div>

            {!gridLoading && filteredProducts.length === 0 ? (
              <EmptyState className="rounded-3xl border border-dashed bg-white p-12" description="ไม่พบสินค้าที่ค้นหา" />
            ) : null}
            {gridLoading ? (
              <p className="py-6 text-center text-sm text-muted-foreground">กำลังโหลดสินค้า...</p>
            ) : null}
            {/* Scrolling this into view asks for the next page. */}
            <div aria-hidden className="h-6" ref={sentinelRef} />
          </div>
        </div>

        <PosCart onOpenChange={setCartOpen} open={cartOpen}>
          <div className="sticky -top-3 z-10 flex shrink-0 items-center justify-between gap-2 bg-card pb-2 xl:static">
            <div>
              <p className="text-xs font-semibold text-primary">{branchName}</p>
	              <h2 className="mt-1 text-base font-bold sm:text-xl">รายการขายปัจจุบัน</h2>
            </div>
            <span className="grid h-11 w-11 place-items-center rounded-full bg-secondary">
              <button aria-label="ปิดตะกร้า" className="grid h-full w-full place-items-center xl:hidden" onClick={() => setCartOpen(false)} type="button">
                <X className="h-5 w-5" />
              </button>
              <ShoppingCart className="hidden h-5 w-5 xl:block" />
            </span>
          </div>

          <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-1">
            <CheckboxField
              checked={fullTaxInvoice}
              label="ออกใบกำกับภาษีเต็มรูป"
              onChange={(event) => {
                const checked = event.target.checked;
                setFullTaxInvoice(checked);
                if (!checked) {
                  setCustomerName("");
                  setCustomerTaxId("");
                }
              }}
            />
            {fullTaxInvoice ? (
              <>
                <Input
                  aria-label="ชื่อลูกค้า"
                  onChange={(event) => setCustomerName(event.target.value)}
                  placeholder="ชื่อลูกค้า (ไม่บังคับ)"
                  value={customerName}
                />
                <Input
                  aria-label="เลขประจำตัวผู้เสียภาษี"
                  onChange={(event) => setCustomerTaxId(event.target.value)}
                  placeholder="เลขประจำตัวผู้เสียภาษี"
                  value={customerTaxId}
                />
              </>
            ) : null}
          </div>

          <div className="mt-3 space-y-3 xl:min-h-0 xl:flex-1 xl:overflow-y-auto xl:pr-1">
            {cart.map((line) => {
              const id = String(line.product.id);
              const key = cartLineKey(line);
              const priced = preview?.lines.find((item) => String(item.product_id) === id && String(item.inventory_lot_id) === String(line.lot.id));
              const units = ((line.product.units as Option[]) || []).filter((unit) => Number(unit.conversion_qty) > 0);
              return (
                <div className="rounded-xl bg-muted p-2.5" key={key}>
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0 flex-1">
                      <p className="break-words text-sm font-semibold xl:truncate" title={String(line.product.name)}>{String(line.product.name)}</p>
                      <p className="truncate text-[11px] text-muted-foreground">
                        {String(line.product.sku || "")} · Lot {String(line.lot.lot_number)} × {line.quantity}
                        {line.lot.expires_on ? ` · หมดอายุ ${new Date(String(line.lot.expires_on)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })}` : ""}
                      </p>
                    </div>
                    {/* Bigger, bolder line price — the number a cashier scans down the cart to check. */}
                    {/* Before tax: ราคาต่อหน่วย × จำนวน (less any line discount),
                        so the lines foot to ยอดก่อนภาษี below rather than each
                        carrying VAT of their own. */}
                    <p className="shrink-0 text-base font-bold tabular-nums text-foreground sm:text-lg">
                      {currency(
                        priced?.line_subtotal == null
                          ? Number(line.product.effective_price || 0) * line.quantity - Number(line.discount || 0)
                          : Number(priced.line_subtotal)
                      )}
                    </p>
                  </div>
                  {units.length > 1 ? (
                    <Select
                      aria-label={`หน่วยขาย ${String(line.product.name)}`}
                      className="mt-2 h-9 text-xs"
                      onChange={(event) => updateLine(key, { unitId: event.target.value })}
                      value={line.unitId}
                    >
                      {units.map((unit) => (
                        <option key={String(unit.id)} value={unit.is_base ? "" : String(unit.id)}>
                          {String(unit.unit_name)}
                          {Number(unit.conversion_qty) > 1 ? ` (x${Number(unit.conversion_qty)})` : ""} · {currency(Number(unit.price || 0))}
                        </option>
                      ))}
                    </Select>
                  ) : null}
                  {/* Discount and quantity share one row so a full cart stays scannable. */}
                  <div className="mt-2 flex flex-wrap items-center gap-2">
                    <Input
                      aria-label={`ส่วนลด ${String(line.product.name)}`}
                      className="h-9 min-w-[5rem] flex-1 text-xs"
                      inputMode="decimal"
                      disabled={Boolean(remoteLock)}
                      onChange={(event) => updateLine(key, { discount: event.target.value })}
                      placeholder="ส่วนลด (บาท)"
                      value={line.discount}
                    />
                    <div className="flex shrink-0 items-center rounded-full bg-white">
                      <button
                        aria-label={`ลดจำนวน ${String(line.product.name)}`}
                        className="grid h-11 w-11 place-items-center disabled:opacity-30 xl:h-8 xl:w-8"
                        disabled={Boolean(remoteLock)}
                        onClick={() => line.quantity === 1
                          ? setCart((current) => current.filter((item) => cartLineKey(item) !== key))
                          : updateLine(key, { quantity: line.quantity - 1 })}
                        type="button"
                      >
                        <Minus className="h-4 w-4" />
                      </button>
                      <span className="w-7 text-center text-sm font-bold">{line.quantity}</span>
                      <button
                        aria-label={`เพิ่มจำนวน ${String(line.product.name)}`}
                        className="grid h-11 w-11 place-items-center disabled:opacity-30 xl:h-8 xl:w-8"
                        disabled={Boolean(remoteLock)}
                        onClick={() => updateLine(key, { quantity: line.quantity + 1 })}
                        type="button"
                      >
                        <Plus className="h-4 w-4" />
                      </button>
                    </div>
                    <button
                      aria-label={`ลบ ${String(line.product.name)}`}
                      className="grid h-11 w-11 shrink-0 place-items-center rounded-full p-1.5 text-muted-foreground hover:bg-white hover:text-destructive disabled:opacity-30"
                      disabled={Boolean(remoteLock)}
                      onClick={() => setCart((current) => current.filter((item) => cartLineKey(item) !== key))}
                      type="button"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              );
            })}
            {cart.length === 0 ? (
              <div className="rounded-2xl border border-dashed p-8 text-center text-sm text-muted-foreground">
                เลือกสินค้าจากรายการขาย
              </div>
            ) : null}
          </div>

          {giveawayLines.length ? (
            <div className="mt-3 space-y-1 rounded-2xl border border-dashed border-success p-3">
              <p className="text-xs font-bold text-success">ของแถมจากโปรโมชั่น</p>
              {giveawayLines.map((item, index) => (
                <p className="text-xs text-muted-foreground" key={`${String(item.product_id)}-${index}`}>
                  {String(item.display_name)} × {Number(item.quantity)}
                  {item.promotion_name ? ` · ${String(item.promotion_name)}` : ""}
                </p>
              ))}
            </div>
          ) : null}

          <div className="mt-4">
            <Input
              aria-label="ส่วนลดท้ายบิล"
              inputMode="decimal"
              onChange={(event) => setBillDiscount(event.target.value)}
              placeholder="ส่วนลดท้ายบิล (บาท)"
              value={billDiscount}
            />
          </div>

          <div className="mt-3 space-y-2 border-t pt-3 text-sm sm:mt-5 sm:pt-4">
            {Number(preview?.summary.discount_total || 0) > 0 ? (
              <div className="flex justify-between text-success">
                <span>ส่วนลดรวม</span>
                <span>-{currency(Number(preview?.summary.discount_total || 0))}</span>
              </div>
            ) : null}
            <div className="flex justify-between"><span>ยอดก่อนภาษี</span><span>{currency(Number(preview?.summary.subtotal || 0))}</span></div>
            <div className="flex justify-between"><span>ภาษีมูลค่าเพิ่ม</span><span>{currency(Number(preview?.summary.tax_amount || 0))}</span></div>
            <div className="flex flex-wrap justify-between gap-1 pt-2 text-lg font-bold sm:text-xl"><span>ยอดรวม</span><span>{currency(Number(preview?.summary.total_amount || 0))}</span></div>
          </div>

          {message ? <p className="mt-4 rounded-xl bg-surface-warm p-3 text-sm">{message}</p> : null}
          {receipt ? (
            <Link
              className="mt-3 flex w-full items-center justify-center rounded-full border px-4 py-3 text-sm font-bold hover:bg-muted"
              href={`/print/invoices/${String(receipt.invoice_id)}`}
              target="_blank"
            >
              พิมพ์ใบเสร็จ {String(receipt.invoice_number)}
            </Link>
          ) : null}
          {remoteLock ? (
            <p className="mt-3 rounded-xl bg-info-50 px-3 py-2 text-xs text-info-800" role="status">
              บิลนี้ {remoteLock.operator || "สำนักงานใหญ่"} เป็นผู้เปิด · รับชำระได้เลย แก้ไขรายการที่นี่ไม่ได้
            </p>
          ) : null}
          {remoteBranchId ? (
            <div className="mt-3 flex items-center justify-between gap-2 rounded-xl bg-info-50 px-3 py-2 text-xs text-info-800">
              <p role="status">
                {remoteStatus === "open"
                  ? "ส่งให้เครื่อง POS ของสาขาแล้ว · สาขารับชำระได้ หรือกดรับชำระที่นี่ก็ได้"
                  : remoteStatus === "completed"
                    ? `สาขารับชำระแล้ว · ${String(remoteSession?.invoice_number || "")}`
                    : "หยิบสินค้าลงตะกร้า แล้วรายการจะไปโผล่ที่เครื่อง POS ของสาขาทันที"}
              </p>
              {remoteStatus === "open" ? (
                <button
                  className="shrink-0 font-semibold underline underline-offset-2"
                  onClick={() =>
                    void proxyClient(`/admin/pos/remote-session?branch_id=${encodeURIComponent(remoteBranchId)}`, { method: "DELETE" })
                      .then(() => setRemoteSession(null))
                      .catch(() => {})
                  }
                  type="button"
                >
                  ยกเลิกการรีโมต
                </button>
              ) : null}
            </div>
          ) : null}
          <div className="sticky -bottom-3 mt-3 grid shrink-0 grid-cols-[44px_auto_1fr] gap-2 border-t bg-card py-3 sm:-bottom-4 xl:static xl:mt-4 xl:grid-cols-[auto_auto_1fr] xl:border-0 xl:py-0">
            <Button
              aria-label="ล้างรายการขาย"
              disabled={!cart.length || Boolean(remoteLock)}
              onClick={() => setCart([])}
              type="button"
              variant="secondary"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              aria-label="พักบิลนี้ไว้"
              disabled={!cart.length || parking || Boolean(remoteLock)}
              onClick={() => setParkOpen(true)}
              type="button"
              variant="secondary"
            >
              <PauseCircle className="h-4 w-4" />
              พักบิล
            </Button>
            <Button className="rounded-full" disabled={!cart.length} onClick={() => void openPayment()} type="button">
              รับชำระเงิน
            </Button>
          </div>
        </PosCart>
      </section>

      <Dialog onOpenChange={setParkOpen} open={parkOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader
            description={`${cart.length} รายการ · บิลที่พักไว้จะยังไม่ตัดสต๊อก และจะหมดอายุอัตโนมัติใน 24 ชั่วโมง`}
            title="พักบิลนี้ไว้"
          />
          <div className="space-y-4">
            <Field hint="ไม่บังคับ — ช่วยให้จำได้ว่าบิลนี้ของใคร" label="บันทึกช่วยจำ">
              <Input
                aria-label="บันทึกช่วยจำของบิลที่พัก"
                onChange={(event) => setParkNote(event.target.value)}
                placeholder="เช่น ลูกค้าไปถอนเงิน / รอญาติมาจ่าย"
                value={parkNote}
              />
            </Field>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setParkOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={parking} onClick={() => void parkBill()} type="button">
                {parking ? "กำลังพักบิล..." : "พักบิล"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(lotProduct)} onOpenChange={(open) => { if (!open) setLotProduct(null); }}>
        <DialogContent className="max-w-2xl">
          <DialogHeader
            title={`เลือก Lot · ${String(lotProduct?.name || "")}`}
            description="หนึ่งบรรทัดขายใช้หนึ่ง Lot ระบบจะแจ้งเตือนหากจำนวนที่กรอกมากกว่ายอดใน Lot"
          />
          {lotsLoading ? (
            <p className="py-10 text-center text-sm text-muted-foreground">กำลังโหลด Lot...</p>
          ) : (
            <div className="grid max-h-[60vh] gap-3 overflow-y-auto">
              {lotOptions.map((lot) => (
                <button
                  className="rounded-2xl border p-4 text-left transition hover:border-primary hover:bg-primary/5"
                  key={String(lot.id)}
                  onClick={() => addSelectedLot(lot)}
                  type="button"
                >
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="font-bold">Lot {String(lot.lot_number)}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        วันที่รับ {new Date(String(lot.received_at)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })} · วันหมดอายุ {lot.expires_on ? new Date(String(lot.expires_on)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" }) : "ไม่กำหนด"}
                      </p>
                    </div>
                    <strong className="text-primary">{currency(Number(lot.selling_price || 0))}</strong>
                  </div>
                </button>
              ))}
              {!lotOptions.length ? <p className="py-10 text-center text-sm text-muted-foreground">ไม่มี Lot ที่พร้อมขาย</p> : null}
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => {
        if (submitting) return;
        if (!open && receipt) startNewSale();
        else { setPaymentOpen(open); setPaymentReady(false); setPaymentError(""); }
      }} open={paymentOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader closeDisabled={submitting} closeLabel="ปิดหน้าชำระเงิน" closeOnDesktop title={receipt ? "ชำระเงินเสร็จสิ้น" : "รับชำระเงิน"} />
            {receipt ? (
              <div className="py-4 text-center">
                <CheckCircle2 className="mx-auto h-20 w-20 text-emerald-500" />
                <p className="mt-5 text-sm font-semibold text-emerald-700">ชำระเงินเสร็จสิ้น</p>
                <h2 className="mt-1 text-2xl font-bold">{String(receipt.invoice_number)}</h2>
                <p className="mt-2 text-3xl font-bold">{currency(Number(receipt.total_amount || 0))}</p>
                <div className="mt-6 grid grid-cols-2 gap-3 rounded-2xl bg-muted p-4 text-left text-sm">
                  <div><span className="block text-muted-foreground">เงินสดสุทธิ</span><strong>{currency(Number(receipt.cash_amount || 0))}</strong></div>
                  <div><span className="block text-muted-foreground">เงินโอน</span><strong>{currency(Number(receipt.transfer_amount || 0))}</strong></div>
                  <div><span className="block text-muted-foreground">เงินสดที่รับ</span><strong>{currency(Number(receipt.tendered_amount || 0))}</strong></div>
                  <div><span className="block text-muted-foreground">เงินทอน</span><strong>{currency(Number(receipt.change_amount || 0))}</strong></div>
                </div>
                <div className="mt-6 grid gap-3 sm:grid-cols-2">
                  <Link
                    className="inline-flex h-12 items-center justify-center gap-2 rounded-full border px-4 font-bold hover:bg-muted"
                    href={`/print/invoices/${String(receipt.invoice_id)}`}
                    target="_blank"
                  >
                    <Printer className="h-4 w-4" />พิมพ์ใบเสร็จ
                  </Link>
                  <Button className="h-12 rounded-full" onClick={startNewSale} type="button">เริ่มรายการใหม่</Button>
                </div>
              </div>
            ) : (
              <>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold text-primary">ยอดที่ต้องชำระ</p>
                <h2 className="mt-1 text-3xl font-bold">{currency(Number(preview?.summary.total_amount || 0))}</h2>
              </div>

            </div>
            <div className="mt-3 grid gap-3 sm:mt-6 sm:gap-4">
              <Select
                aria-label="ช่องทางชำระเงิน"
                onChange={(event) => configurePayment(event.target.value as typeof paymentType)}
                value={paymentType}
              >
                <option value="cash">เงินสด</option>
                <option value="bank_transfer">เงินโอน</option>
                <option value="mixed">เงินสด + เงินโอน</option>
              </Select>
              {paymentType === "bank_transfer" ? (
                <label className="space-y-2">
                  <span className="text-sm font-semibold">ยอดเงินโอน</span>
                  <Input aria-label="ยอดเงินโอน" readOnly value={(paymentTotalCents / 100).toFixed(2)} />
                </label>
              ) : null}
              {paymentType === "mixed" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <label className="space-y-2">
                    <span className="text-sm font-semibold">เงินสดที่ต้องชำระ</span>
                    <Input
                      aria-label="เงินสดที่ต้องชำระ"
                      min="0.01"
                      onChange={(event) => updateMixedCash(event.target.value)}
                      onFocus={captureMixedCashSnapshot}
                      step="0.01"
                      inputMode="decimal"
                      type="number"
                      value={tendered}
                    />
                  </label>
                  <label className="space-y-2">
                    <span className="text-sm font-semibold">ยอดเงินโอน</span>
                    <Input
                      aria-label="ยอดเงินโอน"
                      min="0.01"
                      onChange={(event) => updateMixedTransfer(event.target.value)}
                      step="0.01"
                      inputMode="decimal"
                      type="number"
                      value={transferAmount}
                    />
                  </label>
                </div>
              ) : null}
              {paymentType === "cash" ? (
                <label className="space-y-2">
                  <span className="text-sm font-semibold">เงินสดที่รับ</span>
                  <Input
                    aria-label="เงินสดที่รับ"
                    min="0.01"
                    onChange={(event) => updatePositiveMoney(setTendered, event.target.value)}
                    step="0.01"
                    inputMode="decimal"
                    type="number"
                    value={tendered}
                  />
                </label>
              ) : null}
              {paymentType === "mixed" ? (
                <div className="rounded-2xl bg-muted p-4 text-sm">
                  <div className="grid grid-cols-2 gap-3">
                    <div><span className="block text-muted-foreground">เงินสดสุทธิหลังหักเงินทอน</span><strong>{currency(cashDueCents / 100)}</strong></div>
                    <div><span className="block text-muted-foreground">ยอดเงินโอน</span><strong>{currency((transferCents || 0) / 100)}</strong></div>
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">เงินทอน = เงินสดที่กรอก + ยอดเงินโอน − ยอดที่ต้องชำระ</p>
                </div>
              ) : null}
              <div className="flex items-center justify-between rounded-2xl bg-secondary p-4">
                <span className="font-semibold">เงินทอน</span>
                <span className="text-2xl font-bold">{currency(localChangeCents / 100)}</span>
              </div>
              {paymentError ? <Notice tone="error">{paymentError}</Notice> : null}
              <Button className="h-12 rounded-full" disabled={submitting || paymentChecking || !paymentReady} onClick={() => void checkout()} type="button">
                {submitting ? "กำลังชำระเงิน..." : paymentChecking ? "กำลังตรวจสอบยอด..." : "ยืนยันการชำระเงิน"}
              </Button>
            </div>
              </>
            )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
