"use client";

import { FormEvent, useEffect, useState } from "react";
import { Coins, CreditCard, Search, UserPlus, UserRound, X } from "lucide-react";

import { Field } from "@/components/ui/field";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  EmptyState,
  FeedbackNotice,
  Input,
  LoadingState
} from "@/components/ui/primitives";
import { cn, currency } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

/** The member or account the bill is being rung up for. */
export type PosCustomer = {
  id: string;
  name: string;
  customer_code: string;
  phone: string;
  price_tier: string;
  points_balance: number;
  credit_limit: number;
  credit_available: number;
  credit_days: number;
  tax_id: string;
};

export function toPosCustomer(item: Option): PosCustomer {
  return {
    id: String(item.id),
    name: String(item.name || ""),
    customer_code: String(item.customer_code || ""),
    phone: String(item.phone || ""),
    price_tier: String(item.price_tier || "retail"),
    points_balance: Number(item.points_balance || 0),
    credit_limit: Number(item.credit_limit || 0),
    credit_available: Number(item.credit_available || 0),
    credit_days: Number(item.credit_days || 0),
    tax_id: String(item.tax_id || "")
  };
}

/** The customer strip at the top of the cart: who is buying, their points and credit. */
export function PosCustomerPanel({
  customer,
  redeemPoints,
  minRedeemPoints,
  pointValue,
  disabled,
  onPick,
  onClear,
  onRedeemChange
}: {
  customer: PosCustomer | null;
  redeemPoints: string;
  minRedeemPoints: number;
  pointValue: number;
  disabled?: boolean;
  onPick: () => void;
  onClear: () => void;
  onRedeemChange: (value: string) => void;
}) {
  if (!customer) {
    return (
      <button
        className="flex w-full items-center gap-2 rounded-xl border border-dashed px-3 py-2.5 text-left text-sm text-muted-foreground transition hover:border-primary hover:text-primary disabled:opacity-50"
        disabled={disabled}
        onClick={onPick}
        type="button"
      >
        <UserRound className="h-4 w-4" />
        <span className="flex-1">ลูกค้าทั่วไป · แตะเพื่อเลือกสมาชิก / สะสมแต้ม</span>
        <Search className="h-4 w-4" />
      </button>
    );
  }
  const canRedeem = customer.points_balance >= minRedeemPoints;
  const maxRedeem = customer.points_balance;
  return (
    <div className="space-y-2 rounded-xl border border-primary/30 bg-primary/5 p-3">
      <div className="flex items-start gap-2">
        <UserRound className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">
            {customer.name}
            {customer.price_tier === "wholesale" ? <Badge className="ml-2" tone="info">ขายส่ง</Badge> : null}
          </p>
          <p className="text-2xs text-muted-foreground">
            {customer.customer_code} · {customer.phone || "ไม่มีเบอร์"}
          </p>
        </div>
        <button aria-label="เอาลูกค้าออกจากบิล" className="grid h-8 w-8 place-items-center rounded-full hover:bg-muted disabled:opacity-40" disabled={disabled} onClick={onClear} type="button">
          <X className="h-4 w-4" />
        </button>
      </div>
      <div className="flex flex-wrap gap-2 text-xs">
        <span className="inline-flex items-center gap-1 rounded-full bg-card px-2 py-1">
          <Coins className="h-3 w-3 text-primary" />
          {customer.points_balance.toLocaleString("th-TH")} แต้ม
        </span>
        {customer.credit_limit > 0 ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-card px-2 py-1">
            <CreditCard className="h-3 w-3 text-primary" />
            เครดิตคงเหลือ {currency(customer.credit_available)}
          </span>
        ) : null}
      </div>
      {canRedeem ? (
        <div className="flex items-center gap-2">
          <Input
            aria-label="ใช้แต้มเป็นส่วนลด"
            className="h-10 flex-1 text-xs"
            disabled={disabled}
            inputMode="numeric"
            onChange={(event) => {
              const value = event.target.value;
              if (value === "" || /^\d+$/.test(value)) onRedeemChange(value);
            }}
            placeholder={`ใช้แต้ม (ขั้นต่ำ ${minRedeemPoints})`}
            value={redeemPoints}
          />
          <Button
            className="h-10 shrink-0 text-xs"
            disabled={disabled}
            onClick={() => onRedeemChange(String(maxRedeem))}
            size="sm"
            type="button"
            variant="secondary"
          >
            ใช้ทั้งหมด
          </Button>
        </div>
      ) : null}
      {redeemPoints ? (
        <p className="text-2xs text-muted-foreground">
          {Number(redeemPoints).toLocaleString("th-TH")} แต้ม = ส่วนลด {currency(Number(redeemPoints) * pointValue)} (ก่อนภาษี)
        </p>
      ) : null}
    </div>
  );
}

/** Find a member by phone or name, or register one on the spot. */
export function CustomerPickerDialog({
  open,
  canRegister,
  onOpenChange,
  onSelect
}: {
  open: boolean;
  canRegister: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (customer: PosCustomer) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Option[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [registering, setRegistering] = useState(false);
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setResults([]);
    setError("");
    setRegistering(false);
    setName("");
    setPhone("");
  }, [open]);

  useEffect(() => {
    if (!open || registering) return;
    const term = query.trim();
    if (term.length < 2) {
      setResults([]);
      return;
    }
    let active = true;
    setLoading(true);
    const timer = window.setTimeout(() => {
      void proxyClient<{ items: Option[] }>(`/customers/lookup?q=${encodeURIComponent(term)}`)
        .then((response) => active && setResults(response.items || []))
        .catch((caught) => active && setError(caught instanceof Error ? caught.message : "ค้นหาลูกค้าไม่สำเร็จ"))
        .finally(() => active && setLoading(false));
    }, 250);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [open, query, registering]);

  function startRegister() {
    const term = query.trim();
    const digits = term.replace(/\D/g, "");
    setPhone(digits.length >= 9 ? digits : "");
    setName(digits.length >= 9 ? "" : term);
    setError("");
    setRegistering(true);
  }

  async function register(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const created = await proxyClient<{ id: string }>("/customers", {
        method: "POST",
        body: JSON.stringify({ name, phone, customer_type: "person" })
      });
      const customer = await proxyClient<Option>(`/customers/${created.id}`);
      onSelect(toPosCustomer(customer));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "สมัครสมาชิกไม่สำเร็จ");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg">
        <DialogHeader
          description={registering ? "สมัครแล้วเริ่มสะสมแต้มกับบิลนี้ได้ทันที ใช้ได้ทุกสาขา" : "ค้นหาด้วยเบอร์โทร ชื่อ หรือรหัสสมาชิก"}
          title={registering ? "สมัครสมาชิกใหม่" : "เลือกสมาชิก"}
        />
        {registering ? (
          <form className="space-y-4" onSubmit={register}>
            <Field label="ชื่อ *">
              <Input autoFocus onChange={(event) => setName(event.target.value)} required value={name} />
            </Field>
            <Field label="เบอร์โทร *">
              <Input inputMode="tel" onChange={(event) => setPhone(event.target.value)} required value={phone} />
            </Field>
            {error ? <FeedbackNotice feedback={{ tone: "error", text: error }} /> : null}
            <DialogFooter>
              <Button disabled={busy} onClick={() => setRegistering(false)} type="button" variant="secondary">กลับไปค้นหา</Button>
              <Button loading={busy} type="submit">สมัครและเลือก</Button>
            </DialogFooter>
          </form>
        ) : (
          <div className="space-y-3">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาสมาชิก"
                autoFocus
                className="pl-9"
                inputMode="search"
                onChange={(event) => setQuery(event.target.value)}
                placeholder="เบอร์โทร ชื่อ หรือรหัสสมาชิก"
                value={query}
              />
            </div>
            {error ? <FeedbackNotice feedback={{ tone: "error", text: error }} /> : null}
            {loading ? <LoadingState compact label="กำลังค้นหา..." /> : null}
            <div className="max-h-[45vh] space-y-2 overflow-y-auto">
              {results.map((item) => (
                <button
                  className={cn("w-full rounded-xl border p-3 text-left transition hover:border-primary hover:bg-primary/5", !item.active && "opacity-50")}
                  key={String(item.id)}
                  onClick={() => onSelect(toPosCustomer(item))}
                  type="button"
                >
                  <div className="flex items-center justify-between gap-2">
                    <strong className="truncate">{String(item.name)}</strong>
                    {String(item.price_tier) === "wholesale" ? <Badge tone="info">ขายส่ง</Badge> : null}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {String(item.customer_code)} · {String(item.phone || "ไม่มีเบอร์")} · {Number(item.points_balance || 0).toLocaleString("th-TH")} แต้ม
                    {Number(item.credit_limit || 0) > 0 ? ` · เครดิตคงเหลือ ${currency(Number(item.credit_available || 0))}` : ""}
                    {Number(item.overdue_amount || 0) > 0 ? ` · เกินกำหนด ${currency(Number(item.overdue_amount))}` : ""}
                  </p>
                </button>
              ))}
              {!loading && query.trim().length >= 2 && !results.length ? (
                <EmptyState className="p-6" description="ไม่พบสมาชิก" icon={UserRound} />
              ) : null}
            </div>
            {canRegister ? (
              <Button className="w-full" onClick={startRegister} type="button" variant="secondary">
                <UserPlus className="h-4 w-4" />
                สมัครสมาชิกใหม่
              </Button>
            ) : null}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
