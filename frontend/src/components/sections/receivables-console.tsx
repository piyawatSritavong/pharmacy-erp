"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { HandCoins, Receipt, Search } from "lucide-react";

import { Field } from "@/components/ui/field";
import {
  Badge,
  Button,
  Checkbox,
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
  TabsTrigger
} from "@/components/ui/primitives";
import type { Feedback } from "@/components/ui/primitives";
import { cn, currency, dateTime } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type Item = Record<string, unknown>;

type Receivables = {
  items: Item[];
  customers: Item[];
  aging_totals: Record<string, number>;
  total_outstanding: number;
  overdue_outstanding: number;
};

const BUCKETS: Array<[string, string]> = [
  ["current", "ยังไม่ถึงกำหนด"],
  ["1_30", "เกิน 1-30 วัน"],
  ["31_60", "เกิน 31-60 วัน"],
  ["61_90", "เกิน 61-90 วัน"],
  ["over_90", "เกิน 90 วัน"]
];

function bucketTone(bucket: string) {
  if (bucket === "current") return "success" as const;
  if (bucket === "1_30") return "warning" as const;
  return "error" as const;
}

export function ReceivablesConsole({
  branches,
  initialCustomerId,
  canCollect,
  globalScope
}: {
  branches: Item[];
  initialCustomerId: string;
  canCollect: boolean;
  globalScope: boolean;
}) {
  const [data, setData] = useState<Receivables | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [status, setStatus] = useState("");
  const [branchId, setBranchId] = useState("");
  const [feedback, setFeedback] = useState<Feedback>(null);
  const [payFor, setPayFor] = useState<{ id: string; name: string } | null>(null);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const query = new URLSearchParams();
      if (debounced) query.set("search", debounced);
      if (status) query.set("status", status);
      if (branchId) query.set("branch_id", branchId);
      setData(await proxyClient<Receivables>(`/receivables${query.size ? `?${query.toString()}` : ""}`));
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "โหลดลูกหนี้ไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }, [branchId, debounced, status]);

  useEffect(() => {
    void load();
  }, [load]);

  // Arriving from a customer's page opens their payment straight away.
  useEffect(() => {
    if (!initialCustomerId || !canCollect) return;
    void proxyClient<Item>(`/customers/${encodeURIComponent(initialCustomerId)}`)
      .then((customer) => setPayFor({ id: String(customer.id), name: String(customer.name) }))
      .catch(() => undefined);
  }, [canCollect, initialCustomerId]);

  const customers = data?.customers || [];
  const bills = data?.items || [];

  return (
    <>
      <section className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-7">
        <div className="col-span-2 rounded-2xl border bg-card p-4 shadow-card md:col-span-1 xl:col-span-2">
          <p className="text-xs text-muted-foreground">ยอดค้างรับทั้งหมด</p>
          <p className="text-2xl font-bold">{currency(Number(data?.total_outstanding || 0))}</p>
          <p className="text-xs text-error-800">เกินกำหนด {currency(Number(data?.overdue_outstanding || 0))}</p>
        </div>
        {BUCKETS.map(([key, label]) => (
          <div className="rounded-2xl border bg-card p-4 shadow-card" key={key}>
            <p className="text-xs text-muted-foreground">{label}</p>
            <p className={cn("text-lg font-bold", key !== "current" && Number(data?.aging_totals?.[key] || 0) > 0 && "text-error-800")}>
              {currency(Number(data?.aging_totals?.[key] || 0))}
            </p>
          </div>
        ))}
      </section>

      <section className="overflow-hidden rounded-2xl border bg-card shadow-card">
        <div className="flex flex-col gap-3 border-b bg-surface-warm p-3 sm:p-5 md:flex-row md:flex-wrap md:items-center">
          <div className="relative min-w-0 flex-1 md:max-w-md">
            <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              aria-label="ค้นหาลูกหนี้"
              className="pl-9"
              onChange={(event) => setSearch(event.target.value)}
              placeholder="ชื่อลูกค้า เบอร์โทร รหัสสมาชิก หรือเลขที่บิล"
              value={search}
            />
          </div>
          <Select aria-label="สถานะหนี้" className="w-44 shrink-0" onChange={(event) => setStatus(event.target.value)} value={status}>
            <option value="">ทุกสถานะ</option>
            <option value="overdue">เกินกำหนดชำระ</option>
            <option value="current">ยังไม่ถึงกำหนด</option>
          </Select>
          {globalScope ? (
            <Select aria-label="สาขา" className="w-48 shrink-0" onChange={(event) => setBranchId(event.target.value)} value={branchId}>
              <option value="">ทุกสาขา</option>
              {branches.filter((branch) => branch.sales_enabled !== false).map((branch) => (
                <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
              ))}
            </Select>
          ) : null}
        </div>
        {payFor ? null : <FeedbackNotice className="rounded-none border-b" feedback={feedback} />}
        {loading && !data ? <LoadingState className="p-10" label="กำลังโหลดลูกหนี้" /> : null}
        {loadError ? <ErrorState action={<Button onClick={() => void load()} type="button" variant="secondary">ลองอีกครั้ง</Button>} className="p-10" description={loadError} title="โหลดลูกหนี้ไม่สำเร็จ" /> : null}
        {data && !loadError ? (
          <Tabs className="p-3 sm:p-5" defaultValue="customers">
            <TabsList>
              <TabsTrigger value="customers">รายลูกค้า ({customers.length})</TabsTrigger>
              <TabsTrigger value="bills">รายบิล ({bills.length})</TabsTrigger>
            </TabsList>
            <TabsContent className="mt-4" value="customers">
              {customers.length ? (
                <div className="overflow-x-auto">
                  <table className="responsive-table mobile-card-table w-full min-w-[900px] text-sm" role="table">
                    <thead className="bg-muted text-left" role="rowgroup">
                      <tr role="row">
                        <th className="p-3" role="columnheader" scope="col">ลูกค้า</th>
                        {BUCKETS.map(([key, label]) => <th className="p-3 text-right" key={key} role="columnheader" scope="col">{label}</th>)}
                        <th className="p-3 text-right" role="columnheader" scope="col">รวม</th>
                        <th className="p-3 text-right" role="columnheader" scope="col">จัดการ</th>
                      </tr>
                    </thead>
                    <tbody role="rowgroup">
                      {customers.map((customer) => {
                        const buckets = (customer.buckets as Record<string, number>) || {};
                        return (
                          <tr className="border-t" key={String(customer.customer_id)} role="row">
                            <td className="p-3" data-label="ลูกค้า" data-primary="true" role="cell">
                              <strong className="block">{String(customer.customer_name)}</strong>
                              <span className="text-xs text-muted-foreground">{String(customer.customer_code)} · {String(customer.bill_count)} บิล</span>
                            </td>
                            {BUCKETS.map(([key, label]) => (
                              <td className={cn("p-3 text-right", key !== "current" && Number(buckets[key] || 0) > 0 && "font-semibold text-error-800")} data-label={label} key={key} role="cell">
                                {Number(buckets[key] || 0) ? currency(Number(buckets[key])) : "-"}
                              </td>
                            ))}
                            <td className="p-3 text-right font-bold" data-label="รวม" role="cell">{currency(Number(customer.total))}</td>
                            <td className="p-3" data-actions="true" data-label="จัดการ" role="cell">
                              <div className="flex justify-end">
                                {canCollect ? (
                                  <Button onClick={() => { setFeedback(null); setPayFor({ id: String(customer.customer_id), name: String(customer.customer_name) }); }} size="sm" type="button">
                                    <HandCoins className="h-4 w-4" />
                                    รับชำระ
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
              ) : (
                <EmptyState className="p-12" description="ไม่มีบิลขายเชื่อค้างชำระตามเงื่อนไขนี้" icon={Receipt} />
              )}
            </TabsContent>
            <TabsContent className="mt-4" value="bills">
              {bills.length ? (
                <div className="overflow-x-auto">
                  <table className="responsive-table mobile-card-table w-full min-w-[900px] text-sm" role="table">
                    <thead className="bg-muted text-left" role="rowgroup">
                      <tr role="row">
                        <th className="p-3" role="columnheader" scope="col">บิล</th>
                        <th className="p-3" role="columnheader" scope="col">ลูกค้า</th>
                        <th className="p-3" role="columnheader" scope="col">สาขา</th>
                        <th className="p-3" role="columnheader" scope="col">ครบกำหนด</th>
                        <th className="p-3 text-right" role="columnheader" scope="col">ยอดบิล</th>
                        <th className="p-3 text-right" role="columnheader" scope="col">ชำระแล้ว</th>
                        <th className="p-3 text-right" role="columnheader" scope="col">ค้างชำระ</th>
                      </tr>
                    </thead>
                    <tbody role="rowgroup">
                      {bills.map((bill) => (
                        <tr className="border-t" key={String(bill.invoice_id)} role="row">
                          <td className="p-3" data-label="บิล" data-primary="true" role="cell">
                            <strong className="block">{String(bill.invoice_number)}</strong>
                            <span className="text-xs text-muted-foreground">{dateTime(String(bill.issued_at))}</span>
                          </td>
                          <td className="p-3" data-label="ลูกค้า" role="cell">{String(bill.customer_name)}</td>
                          <td className="p-3" data-label="สาขา" role="cell">{String(bill.branch_name)}</td>
                          <td className="p-3" data-label="ครบกำหนด" role="cell">
                            {String(bill.due_date || "-")}
                            <Badge className="ml-2" tone={bucketTone(String(bill.aging_bucket))}>
                              {Number(bill.days_overdue || 0) > 0 ? `เกิน ${String(bill.days_overdue)} วัน` : "ยังไม่ถึงกำหนด"}
                            </Badge>
                          </td>
                          <td className="p-3 text-right" data-label="ยอดบิล" role="cell">{currency(Number(bill.total_amount))}</td>
                          <td className="p-3 text-right" data-label="ชำระแล้ว" role="cell">{currency(Number(bill.paid_amount))}</td>
                          <td className="p-3 text-right font-bold" data-label="ค้างชำระ" role="cell">{currency(Number(bill.outstanding))}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <EmptyState className="p-12" description="ไม่มีบิลขายเชื่อค้างชำระตามเงื่อนไขนี้" icon={Receipt} />
              )}
            </TabsContent>
          </Tabs>
        ) : null}
      </section>

      <ReceivePaymentDialog
        customer={payFor}
        onClose={() => setPayFor(null)}
        onPaid={(text) => {
          setPayFor(null);
          setFeedback({ tone: "success", text });
          void load();
        }}
      />
    </>
  );
}

export function ReceivePaymentDialog({
  customer,
  onClose,
  onPaid
}: {
  customer: { id: string; name: string } | null;
  onClose: () => void;
  onPaid: (message: string) => void;
}) {
  const [bills, setBills] = useState<Item[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState("cash");
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!customer) return;
    setError("");
    setReference("");
    setNote("");
    setMethod("cash");
    setLoading(true);
    void proxyClient<Receivables>(`/receivables?customer_id=${encodeURIComponent(customer.id)}`)
      .then((response) => {
        const items = response.items || [];
        setBills(items);
        setSelected(new Set(items.map((item) => String(item.invoice_id))));
        setAmount(Number(response.total_outstanding || 0).toFixed(2));
      })
      .catch((caught) => setError(caught instanceof Error ? caught.message : "โหลดบิลค้างชำระไม่สำเร็จ"))
      .finally(() => setLoading(false));
  }, [customer]);

  const selectedTotal = useMemo(
    () => bills.filter((bill) => selected.has(String(bill.invoice_id))).reduce((sum, bill) => sum + Number(bill.outstanding || 0), 0),
    [bills, selected]
  );

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      const total = bills.filter((bill) => next.has(String(bill.invoice_id))).reduce((sum, bill) => sum + Number(bill.outstanding || 0), 0);
      setAmount(total.toFixed(2));
      return next;
    });
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!customer) return;
    setBusy(true);
    setError("");
    try {
      const response = await proxyClient<{ allocations: Item[]; amount: number }>("/receivables/payments", {
        method: "POST",
        body: JSON.stringify({
          customer_id: customer.id,
          payment_type: method,
          amount: Number(amount),
          reference_code: reference,
          note,
          invoice_ids: [...selected]
        })
      });
      const settled = response.allocations.map((allocation) => `${String(allocation.invoice_number)} ${currency(Number(allocation.amount))}`).join(", ");
      onPaid(`รับชำระจาก ${customer.name} ${currency(response.amount)} แล้ว · ${settled}`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "รับชำระไม่สำเร็จ");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open={Boolean(customer)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader description="ตัดชำระบิลที่ครบกำหนดก่อนตามลำดับ เงินที่รับจะเข้ายอดรับเงินสด/เงินโอนของสาขาที่รับในวันนี้" title={`รับชำระหนี้ · ${customer?.name || ""}`} />
        {loading ? <LoadingState label="กำลังโหลดบิลค้างชำระ" /> : null}
        {!loading ? (
          <form className="space-y-4" onSubmit={submit}>
            <div className="max-h-[35vh] overflow-auto rounded-xl border">
              {bills.length ? (
                <table className="w-full text-sm">
                  <thead className="sticky top-0 bg-muted text-left">
                    <tr>
                      <th className="w-10 p-2" />
                      <th className="p-2">บิล</th>
                      <th className="p-2">ครบกำหนด</th>
                      <th className="p-2 text-right">ค้างชำระ</th>
                    </tr>
                  </thead>
                  <tbody>
                    {bills.map((bill) => {
                      const id = String(bill.invoice_id);
                      return (
                        <tr className="border-t" key={id}>
                          <td className="p-2">
                            <Checkbox aria-label={`เลือก ${String(bill.invoice_number)}`} checked={selected.has(id)} onChange={() => toggle(id)} />
                          </td>
                          <td className="p-2">
                            {String(bill.invoice_number)}
                            <span className="block text-xs text-muted-foreground">{String(bill.branch_name)}</span>
                          </td>
                          <td className="p-2">
                            {String(bill.due_date || "-")}
                            {Number(bill.days_overdue || 0) > 0 ? <Badge className="ml-2" tone="error">เกิน {String(bill.days_overdue)} วัน</Badge> : null}
                          </td>
                          <td className="p-2 text-right font-semibold">{currency(Number(bill.outstanding))}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              ) : (
                <EmptyState className="p-8" description="ลูกค้ารายนี้ไม่มีบิลค้างชำระ" icon={Receipt} />
              )}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field hint={`บิลที่เลือก ${currency(selectedTotal)}`} label="ยอดรับชำระ (บาท)">
                <Input inputMode="decimal" min={0.01} onChange={(event) => setAmount(event.target.value)} required step="0.01" type="number" value={amount} />
              </Field>
              <Field label="รับชำระด้วย">
                <Select onChange={(event) => setMethod(event.target.value)} value={method}>
                  <option value="cash">เงินสด</option>
                  <option value="bank_transfer">เงินโอน</option>
                </Select>
              </Field>
              {method === "bank_transfer" ? (
                <Field label="เลขอ้างอิงการโอน">
                  <Input onChange={(event) => setReference(event.target.value)} value={reference} />
                </Field>
              ) : null}
              <Field className={method === "bank_transfer" ? "" : "sm:col-span-2"} label="หมายเหตุ">
                <Input onChange={(event) => setNote(event.target.value)} value={note} />
              </Field>
            </div>
            {error ? <FeedbackNotice feedback={{ tone: "error", text: error }} /> : null}
            <DialogFooter>
              <Button disabled={busy} onClick={onClose} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={!bills.length || !selected.size} loading={busy} loadingText="กำลังบันทึก..." type="submit">
                <HandCoins className="h-4 w-4" />
                รับชำระ {amount ? currency(Number(amount)) : ""}
              </Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
