"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { Coins, CreditCard, Eye, Pencil, Plus, Search, Settings2, UserRound } from "lucide-react";

import { Field } from "@/components/ui/field";
import {
  Badge,
  Button,
  buttonVariants,
  CheckboxField,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  EmptyState,
  ErrorState,
  FeedbackNotice,
  Input,
  LoadingState,
  Select,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea
} from "@/components/ui/primitives";
import type { Feedback } from "@/components/ui/primitives";
import { cn, currency, dateTime } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Item = Record<string, unknown>;

export type LoyaltySettings = { baht_per_point: number; point_value: number; min_redeem_points: number };

const blankCustomer: Item = {
  customer_type: "person",
  name: "",
  phone: "",
  tax_id: "",
  address: "",
  note: "",
  price_tier: "retail",
  credit_limit: 0,
  credit_days: 0,
  active: true
};

const ENTRY_LABEL: Record<string, string> = {
  earn: "ได้รับแต้ม",
  redeem: "ใช้แต้ม",
  earn_reversal: "หักแต้มคืน",
  redeem_reversal: "คืนแต้ม",
  adjust: "ปรับแต้ม"
};

const PAYMENT_STATUS: Record<string, { label: string; tone: "success" | "warning" | "error" }> = {
  paid: { label: "ชำระแล้ว", tone: "success" },
  partial: { label: "ชำระบางส่วน", tone: "warning" },
  unpaid: { label: "ค้างชำระ", tone: "error" }
};

export function TierBadge({ tier }: { tier: unknown }) {
  return String(tier) === "wholesale" ? <Badge tone="info">ขายส่ง</Badge> : <Badge tone="neutral">ขายปลีก</Badge>;
}

export function CustomerConsole({
  canManage,
  canTerms,
  canCollect,
  loyalty
}: {
  canManage: boolean;
  /** Head office: credit lines, wholesale tier, points adjustments, loyalty rules. */
  canTerms: boolean;
  canCollect: boolean;
  loyalty: LoyaltySettings;
}) {
  const [items, setItems] = useState<Item[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [tier, setTier] = useState("");
  const [creditOnly, setCreditOnly] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [feedback, setFeedback] = useState<Feedback>(null);
  const [editing, setEditing] = useState<Item | null>(null);
  const [busy, setBusy] = useState(false);
  const [dialogError, setDialogError] = useState("");
  const [detailId, setDetailId] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [rules, setRules] = useState(loyalty);
  const pageSize = 50;

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const query = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
      if (debounced) query.set("search", debounced);
      if (tier) query.set("price_tier", tier);
      if (creditOnly) query.set("credit_only", "true");
      const response = await proxyClient<{ items: Item[]; total: number }>(`/customers?${query.toString()}`);
      setItems(response.items || []);
      setTotal(response.total || 0);
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "โหลดรายชื่อลูกค้าไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }, [creditOnly, debounced, page, tier]);

  useEffect(() => {
    void load();
  }, [load]);

  function openCreate() {
    setDialogError("");
    setEditing({ ...blankCustomer });
  }

  async function openEdit(id: string) {
    setDialogError("");
    try {
      setEditing(await proxyClient<Item>(`/customers/${id}`));
    } catch (error) {
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : "โหลดข้อมูลลูกค้าไม่สำเร็จ" });
    }
  }

  function update(key: string, value: unknown) {
    setEditing((current) => (current ? { ...current, [key]: value } : current));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!editing) return;
    setBusy(true);
    setDialogError("");
    try {
      const id = String(editing.id || "");
      const body = {
        customer_type: editing.customer_type,
        name: editing.name,
        phone: editing.phone,
        tax_id: editing.tax_id,
        address: editing.address,
        note: editing.note,
        ...(canTerms
          ? {
              price_tier: editing.price_tier,
              credit_limit: Number(editing.credit_limit || 0),
              credit_days: Number(editing.credit_days || 0),
              active: Boolean(editing.active)
            }
          : {})
      };
      const response = await proxyClient<{ customer_code?: string; message: string }>(id ? `/customers/${id}` : "/customers", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(body)
      });
      setEditing(null);
      setFeedback({ tone: "success", text: id ? "บันทึกข้อมูลลูกค้าแล้ว" : `สมัครสมาชิกแล้ว · รหัส ${response.customer_code || ""}` });
      void load();
    } catch (error) {
      setDialogError(error instanceof Error ? error.message : "บันทึกข้อมูลลูกค้าไม่สำเร็จ");
    } finally {
      setBusy(false);
    }
  }

  async function saveRules(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setDialogError("");
    try {
      await proxyClient("/loyalty-settings", { method: "PUT", body: JSON.stringify(rules) });
      setSettingsOpen(false);
      setFeedback({ tone: "success", text: "บันทึกเงื่อนไขแต้มสะสมแล้ว" });
    } catch (error) {
      setDialogError(error instanceof Error ? error.message : "บันทึกเงื่อนไขไม่สำเร็จ");
    } finally {
      setBusy(false);
    }
  }

  const filtered = Boolean(debounced || tier || creditOnly);
  const pages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <>
      <section className="rounded-2xl border bg-card p-4 shadow-card sm:p-5">
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <Coins className="h-5 w-5 text-primary" />
          <p className="min-w-0 flex-1">
            <strong>แต้มสะสม:</strong> ซื้อทุก {rules.baht_per_point.toLocaleString("th-TH")} บาท ได้ 1 แต้ม · 1 แต้ม = {currency(rules.point_value)} ·
            ใช้ขั้นต่ำครั้งละ {rules.min_redeem_points.toLocaleString("th-TH")} แต้ม
          </p>
          {canTerms ? (
            <Button onClick={() => { setDialogError(""); setSettingsOpen(true); }} size="sm" type="button" variant="secondary">
              <Settings2 className="h-4 w-4" />
              ตั้งค่าแต้ม
            </Button>
          ) : null}
        </div>
      </section>

      <section className="overflow-hidden rounded-2xl border bg-card shadow-card">
        <div className="flex flex-col gap-3 border-b bg-surface-warm p-3 sm:p-5 md:flex-row md:items-center md:justify-between">
          <div className="flex flex-1 flex-wrap items-center gap-3">
            <div className="relative min-w-0 flex-1 md:max-w-md">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input
                aria-label="ค้นหาลูกค้า"
                className="pl-9"
                onChange={(event) => { setSearch(event.target.value); setPage(1); }}
                placeholder="ชื่อ เบอร์โทร รหัสสมาชิก หรือเลขผู้เสียภาษี"
                value={search}
              />
            </div>
            <Select aria-label="กรองระดับราคา" className="w-40 shrink-0" onChange={(event) => { setTier(event.target.value); setPage(1); }} value={tier}>
              <option value="">ทุกระดับราคา</option>
              <option value="retail">ขายปลีก</option>
              <option value="wholesale">ขายส่ง</option>
            </Select>
            <CheckboxField checked={creditOnly} label="เฉพาะที่มีเครดิต" onChange={(event) => { setCreditOnly(event.target.checked); setPage(1); }} />
          </div>
          {canManage ? (
            <Button onClick={openCreate} type="button">
              <Plus className="h-4 w-4" />
              สมัครสมาชิก
            </Button>
          ) : null}
        </div>
        {editing || detailId || settingsOpen ? null : <FeedbackNotice className="rounded-none border-b" feedback={feedback} />}
        {loading && !items.length ? <LoadingState className="p-10" label="กำลังโหลดรายชื่อลูกค้า" /> : null}
        {loadError ? <ErrorState action={<Button onClick={() => void load()} type="button" variant="secondary">ลองอีกครั้ง</Button>} className="p-10" description={loadError} title="โหลดรายชื่อลูกค้าไม่สำเร็จ" /> : null}
        {!loadError && items.length ? (
          <div className="overflow-x-auto p-3 sm:p-0">
            <table className="responsive-table mobile-card-table w-full min-w-[900px] text-sm" role="table">
              <thead className="bg-muted text-left" role="rowgroup">
                <tr role="row">
                  <th className="p-3" role="columnheader" scope="col">ลูกค้า</th>
                  <th className="p-3" role="columnheader" scope="col">ระดับราคา</th>
                  <th className="p-3 text-right" role="columnheader" scope="col">แต้มคงเหลือ</th>
                  <th className="p-3 text-right" role="columnheader" scope="col">วงเงินเครดิต</th>
                  <th className="p-3 text-right" role="columnheader" scope="col">ค้างชำระ</th>
                  <th className="p-3 text-right" role="columnheader" scope="col">จัดการ</th>
                </tr>
              </thead>
              <tbody role="rowgroup">
                {items.map((item) => {
                  const overdue = Number(item.overdue_amount || 0);
                  return (
                    <tr className={cn("border-t", !item.active && "opacity-60")} key={String(item.id)} role="row">
                      <td className="p-3" data-label="ลูกค้า" data-primary="true" role="cell">
                        <strong className="block">{String(item.name)}</strong>
                        <span className="text-xs text-muted-foreground">
                          {String(item.customer_code)} · {String(item.phone || "ไม่มีเบอร์")}
                          {item.active ? "" : " · ปิดใช้งาน"}
                        </span>
                      </td>
                      <td className="p-3" data-label="ระดับราคา" role="cell"><TierBadge tier={item.price_tier} /></td>
                      <td className="p-3 text-right font-semibold" data-label="แต้มคงเหลือ" role="cell">{Number(item.points_balance || 0).toLocaleString("th-TH")}</td>
                      <td className="p-3 text-right" data-label="วงเงินเครดิต" role="cell">
                        {item.credit_enabled ? (
                          <>
                            {currency(Number(item.credit_limit))}
                            <span className="block text-xs text-muted-foreground">ใช้ได้อีก {currency(Number(item.credit_available))} · {String(item.credit_days)} วัน</span>
                          </>
                        ) : (
                          <span className="text-muted-foreground">เงินสด</span>
                        )}
                      </td>
                      <td className="p-3 text-right" data-label="ค้างชำระ" role="cell">
                        {Number(item.outstanding || 0) > 0 ? (
                          <>
                            <span className="font-semibold">{currency(Number(item.outstanding))}</span>
                            {overdue > 0 ? <Badge className="ml-2" tone="error">เกินกำหนด {currency(overdue)}</Badge> : null}
                          </>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </td>
                      <td className="p-3" data-actions="true" data-label="จัดการ" role="cell">
                        <div className="flex justify-end gap-2">
                          <Button onClick={() => setDetailId(String(item.id))} size="sm" type="button" variant="secondary">
                            <Eye className="h-4 w-4" />
                            ดู
                          </Button>
                          {canManage ? (
                            <Button onClick={() => void openEdit(String(item.id))} size="sm" type="button" variant="secondary">
                              <Pencil className="h-4 w-4" />
                              แก้ไข
                            </Button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}
        {!loading && !loadError && !items.length ? (
          <EmptyState
            action={filtered || !canManage ? undefined : <Button onClick={openCreate} type="button"><Plus className="h-4 w-4" />สมัครสมาชิก</Button>}
            className="p-12"
            description={filtered ? "ไม่พบลูกค้าตามเงื่อนไข ลองปรับคำค้นหาหรือตัวกรอง" : "ยังไม่มีสมาชิก สมัครสมาชิกให้ลูกค้าเพื่อสะสมแต้มทุกสาขา"}
            icon={UserRound}
          />
        ) : null}
        {pages > 1 ? (
          <div className="flex items-center justify-between gap-3 border-t p-4 text-sm">
            <span className="text-muted-foreground">ทั้งหมด {total.toLocaleString("th-TH")} ราย · หน้า {page}/{pages}</span>
            <div className="flex gap-2">
              <Button disabled={page <= 1} onClick={() => setPage(page - 1)} size="sm" type="button" variant="secondary">ก่อนหน้า</Button>
              <Button disabled={page >= pages} onClick={() => setPage(page + 1)} size="sm" type="button" variant="secondary">ถัดไป</Button>
            </div>
          </div>
        ) : null}
      </section>

      <Dialog onOpenChange={(open) => !open && setEditing(null)} open={Boolean(editing)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader
            description={canTerms ? "ข้อมูลลูกค้าใช้ร่วมกันทุกสาขา วงเงินเครดิตและระดับราคาส่งมีผลทันทีที่หน้าร้าน" : "ข้อมูลลูกค้าใช้ร่วมกันทุกสาขา วงเงินเครดิตและราคาส่งกำหนดโดยสำนักงานใหญ่"}
            title={editing?.id ? "แก้ไขข้อมูลลูกค้า" : "สมัครสมาชิก"}
          />
          {editing ? (
            <form className="grid gap-4 md:grid-cols-2" onSubmit={submit}>
              <Field label="ชื่อลูกค้า *">
                <Input autoFocus onChange={(event) => update("name", event.target.value)} required value={String(editing.name || "")} />
              </Field>
              <Field hint="ใช้ค้นหาที่หน้าร้าน" label={String(editing.customer_type) === "business" ? "เบอร์โทร" : "เบอร์โทร *"}>
                <Input
                  inputMode="tel"
                  onChange={(event) => update("phone", event.target.value)}
                  required={String(editing.customer_type) !== "business"}
                  value={String(editing.phone || "")}
                />
              </Field>
              <Field label="ประเภทลูกค้า">
                <Select onChange={(event) => update("customer_type", event.target.value)} value={String(editing.customer_type || "person")}>
                  <option value="person">บุคคล</option>
                  <option value="business">ร้านค้า/นิติบุคคล</option>
                </Select>
              </Field>
              <Field label="เลขผู้เสียภาษี">
                <Input inputMode="numeric" onChange={(event) => update("tax_id", event.target.value)} value={String(editing.tax_id || "")} />
              </Field>
              <Field className="md:col-span-2" label="ที่อยู่ (สำหรับใบกำกับภาษี)">
                <Input onChange={(event) => update("address", event.target.value)} value={String(editing.address || "")} />
              </Field>
              {canTerms ? (
                <>
                  <Field label="ระดับราคา">
                    <Select onChange={(event) => update("price_tier", event.target.value)} value={String(editing.price_tier || "retail")}>
                      <option value="retail">ขายปลีก</option>
                      <option value="wholesale">ขายส่ง (ได้ราคาส่งที่ตั้งไว้ในสินค้า)</option>
                    </Select>
                  </Field>
                  <div className="grid grid-cols-2 gap-2">
                    <Field hint="0 = ไม่ให้เครดิต" label="วงเงินเครดิต (บาท)">
                      <Input min={0} onChange={(event) => update("credit_limit", event.target.value)} step="0.01" type="number" value={String(editing.credit_limit ?? 0)} />
                    </Field>
                    <Field label="เครดิต (วัน)">
                      <Input max={365} min={0} onChange={(event) => update("credit_days", event.target.value)} type="number" value={String(editing.credit_days ?? 0)} />
                    </Field>
                  </div>
                </>
              ) : null}
              <Field className="md:col-span-2" label="หมายเหตุ">
                <Textarea onChange={(event) => update("note", event.target.value)} value={String(editing.note || "")} />
              </Field>
              {canTerms && editing.id ? (
                <CheckboxField checked={Boolean(editing.active)} label="เปิดใช้งาน" onChange={(event) => update("active", event.target.checked)} />
              ) : null}
              {dialogError ? <FeedbackNotice className="md:col-span-2" feedback={{ tone: "error", text: dialogError }} /> : null}
              <DialogFooter className="md:col-span-2">
                <Button disabled={busy} onClick={() => setEditing(null)} type="button" variant="secondary">ยกเลิก</Button>
                <Button loading={busy} loadingText="กำลังบันทึก..." type="submit">บันทึก</Button>
              </DialogFooter>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setSettingsOpen} open={settingsOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader description="มีผลกับบิลที่ขายหลังจากบันทึก แต้มที่สะสมไว้แล้วยังอยู่ครบ" title="ตั้งค่าแต้มสะสม" />
          <form className="space-y-4" onSubmit={saveRules}>
            <Field label="ยอดซื้อกี่บาทได้ 1 แต้ม">
              <Input min={1} onChange={(event) => setRules({ ...rules, baht_per_point: Number(event.target.value) })} step="1" type="number" value={rules.baht_per_point} />
            </Field>
            <Field hint="ส่วนลดที่ได้เมื่อใช้แต้ม" label="มูลค่า 1 แต้ม (บาท)">
              <Input min={0.01} onChange={(event) => setRules({ ...rules, point_value: Number(event.target.value) })} step="0.01" type="number" value={rules.point_value} />
            </Field>
            <Field label="ใช้แต้มขั้นต่ำครั้งละ (แต้ม)">
              <Input min={1} onChange={(event) => setRules({ ...rules, min_redeem_points: Number(event.target.value) })} step="1" type="number" value={rules.min_redeem_points} />
            </Field>
            {dialogError ? <FeedbackNotice feedback={{ tone: "error", text: dialogError }} /> : null}
            <DialogFooter>
              <Button disabled={busy} onClick={() => setSettingsOpen(false)} type="button" variant="secondary">ยกเลิก</Button>
              <Button loading={busy} type="submit">บันทึก</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <CustomerDetailDialog
        canCollect={canCollect}
        canTerms={canTerms}
        customerId={detailId}
        onChanged={() => void load()}
        onClose={() => setDetailId("")}
      />
    </>
  );
}

function CustomerDetailDialog({
  customerId,
  canTerms,
  canCollect,
  onClose,
  onChanged
}: {
  customerId: string;
  canTerms: boolean;
  canCollect: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const [detail, setDetail] = useState<Item | null>(null);
  const [error, setError] = useState("");
  const [adjustPoints, setAdjustPoints] = useState("");
  const [adjustNote, setAdjustNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);

  const load = useCallback(async () => {
    if (!customerId) return;
    setError("");
    try {
      setDetail(await proxyClient<Item>(`/customers/${customerId}`));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "โหลดข้อมูลลูกค้าไม่สำเร็จ");
    }
  }, [customerId]);

  useEffect(() => {
    setDetail(null);
    setFeedback(null);
    setAdjustPoints("");
    setAdjustNote("");
    void load();
  }, [load]);

  async function adjust(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setFeedback(null);
    try {
      await proxyClient(`/customers/${customerId}/points`, {
        method: "POST",
        body: JSON.stringify({ points: Number(adjustPoints), note: adjustNote })
      });
      setAdjustPoints("");
      setAdjustNote("");
      setFeedback({ tone: "success", text: "ปรับแต้มแล้ว" });
      await load();
      onChanged();
    } catch (caught) {
      setFeedback({ tone: "error", text: caught instanceof Error ? caught.message : "ปรับแต้มไม่สำเร็จ" });
    } finally {
      setBusy(false);
    }
  }

  const invoices = (detail?.invoices as Item[]) || [];
  const ledger = (detail?.points_ledger as Item[]) || [];
  const outstanding = Number(detail?.outstanding || 0);

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open={Boolean(customerId)}>
      <DialogContent className="max-w-4xl">
        <DialogHeader
          description={detail ? `${String(detail.customer_code)} · ${String(detail.phone || "ไม่มีเบอร์")}${detail.home_branch_name ? ` · สมัครที่ ${String(detail.home_branch_name)}` : ""}` : ""}
          title={detail ? String(detail.name) : "ข้อมูลลูกค้า"}
        />
        {error ? <ErrorState description={error} title="โหลดข้อมูลลูกค้าไม่สำเร็จ" /> : null}
        {!detail && !error ? <LoadingState label="กำลังโหลด" /> : null}
        {detail ? (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              {[
                ["แต้มคงเหลือ", Number(detail.points_balance || 0).toLocaleString("th-TH") + " แต้ม"],
                ["ยอดซื้อสะสม", currency(Number(detail.total_spent || 0))],
                ["จำนวนบิล", Number(detail.visit_count || 0).toLocaleString("th-TH")],
                ["ค้างชำระ", currency(outstanding)]
              ].map(([label, value]) => (
                <div className="rounded-xl border bg-surface-warm p-3" key={label}>
                  <p className="text-xs text-muted-foreground">{label}</p>
                  <p className="text-lg font-bold">{value}</p>
                </div>
              ))}
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <TierBadge tier={detail.price_tier} />
              {detail.credit_enabled ? (
                <Badge tone="primary">
                  <CreditCard className="mr-1 h-3 w-3" />
                  เครดิต {currency(Number(detail.credit_limit))} · {String(detail.credit_days)} วัน · ใช้ได้อีก {currency(Number(detail.credit_available))}
                </Badge>
              ) : null}
              {Number(detail.overdue_amount || 0) > 0 ? <Badge tone="error">เกินกำหนด {currency(Number(detail.overdue_amount))}</Badge> : null}
              {canCollect && outstanding > 0 ? (
                <Link className={buttonVariants({ size: "sm", className: "ml-auto" })} href={`/receivables?customer_id=${encodeURIComponent(String(detail.id))}`}>
                  รับชำระหนี้
                </Link>
              ) : null}
            </div>
            <Tabs defaultValue="invoices">
              <TabsList>
                <TabsTrigger value="invoices">ประวัติการซื้อ</TabsTrigger>
                <TabsTrigger value="points">ประวัติแต้ม</TabsTrigger>
              </TabsList>
              <TabsContent className="mt-3" value="invoices">
                {invoices.length ? (
                  <div className="max-h-[50vh] overflow-auto rounded-xl border">
                    <table className="w-full text-sm">
                      <thead className="sticky top-0 bg-muted text-left">
                        <tr>
                          <th className="p-2">เลขที่</th>
                          <th className="p-2">สาขา</th>
                          <th className="p-2 text-right">ยอด</th>
                          <th className="p-2">สถานะ</th>
                          <th className="p-2 text-right">แต้ม</th>
                        </tr>
                      </thead>
                      <tbody>
                        {invoices.map((invoice) => {
                          const status = PAYMENT_STATUS[String(invoice.payment_status)] || PAYMENT_STATUS.paid;
                          return (
                            <tr className="border-t" key={String(invoice.id)}>
                              <td className="p-2">
                                <Link className="font-medium text-primary hover:underline" href={`/print/invoices/${String(invoice.id)}`} target="_blank">{String(invoice.invoice_number)}</Link>
                                <span className="block text-xs text-muted-foreground">{dateTime(String(invoice.issued_at))}</span>
                              </td>
                              <td className="p-2">{String(invoice.branch_name)}</td>
                              <td className="p-2 text-right">{currency(Number(invoice.total_amount))}</td>
                              <td className="p-2">
                                <Badge tone={status.tone}>{String(invoice.sale_type) === "credit" ? `เครดิต · ${status.label}` : status.label}</Badge>
                                {invoice.due_date && Number(invoice.outstanding || 0) > 0 ? <span className="block text-xs text-muted-foreground">ครบกำหนด {String(invoice.due_date)}</span> : null}
                              </td>
                              <td className="p-2 text-right text-xs">
                                {Number(invoice.points_earned || 0) ? <span className="block text-success-800">+{String(invoice.points_earned)}</span> : null}
                                {Number(invoice.points_redeemed || 0) ? <span className="block text-error-800">-{String(invoice.points_redeemed)}</span> : null}
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <EmptyState className="p-8" description="ยังไม่มีบิลของลูกค้ารายนี้" icon={UserRound} />
                )}
              </TabsContent>
              <TabsContent className="mt-3 space-y-3" value="points">
                {canTerms ? (
                  <form className="flex flex-wrap items-end gap-2 rounded-xl border p-3" onSubmit={adjust}>
                    <Field className="w-32" hint="ติดลบ = หัก" label="ปรับแต้ม">
                      <Input onChange={(event) => setAdjustPoints(event.target.value)} required step="1" type="number" value={adjustPoints} />
                    </Field>
                    <Field className="min-w-48 flex-1" label="เหตุผล">
                      <Input onChange={(event) => setAdjustNote(event.target.value)} required value={adjustNote} />
                    </Field>
                    <Button loading={busy} type="submit">บันทึก</Button>
                  </form>
                ) : null}
                <FeedbackNotice feedback={feedback} />
                {ledger.length ? (
                  <div className="max-h-[45vh] overflow-auto rounded-xl border">
                    <table className="w-full text-sm">
                      <thead className="sticky top-0 bg-muted text-left">
                        <tr>
                          <th className="p-2">วันที่</th>
                          <th className="p-2">รายการ</th>
                          <th className="p-2 text-right">แต้ม</th>
                          <th className="p-2 text-right">คงเหลือ</th>
                        </tr>
                      </thead>
                      <tbody>
                        {ledger.map((entry) => (
                          <tr className="border-t" key={String(entry.id)}>
                            <td className="p-2 text-xs">{dateTime(String(entry.created_at))}</td>
                            <td className="p-2">
                              {ENTRY_LABEL[String(entry.entry_type)] || String(entry.entry_type)}
                              <span className="block text-xs text-muted-foreground">
                                {[entry.invoice_number, entry.branch_name, entry.note].filter(Boolean).join(" · ")}
                              </span>
                            </td>
                            <td className={cn("p-2 text-right font-semibold", Number(entry.points) > 0 ? "text-success-800" : "text-error-800")}>
                              {Number(entry.points) > 0 ? "+" : ""}{String(entry.points)}
                            </td>
                            <td className="p-2 text-right">{String(entry.balance_after)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <EmptyState className="p-8" description="ยังไม่มีการเคลื่อนไหวของแต้ม" icon={Coins} />
                )}
              </TabsContent>
            </Tabs>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
