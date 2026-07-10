"use client";

import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { DataTable, MetricGrid, SectionCard } from "@/components/sections/common";
import { Badge, Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";
import { currency } from "@/lib/utils";

type Option = Record<string, unknown>;

export function InstallmentConsole({
  plans,
  summary,
  invoices,
  canManage,
  canCollect
}: {
  plans: Option[];
  summary: Option;
  invoices: Option[];
  canManage: boolean;
  canCollect: boolean;
}) {
  const router = useRouter();
  const [invoiceId, setInvoiceId] = useState("");
  const [months, setMonths] = useState("6");
  const [startDate, setStartDate] = useState("");
  const [payAmounts, setPayAmounts] = useState<Record<string, string>>({});
  const [payTypes, setPayTypes] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");

  const eligibleInvoices = useMemo(
    () => invoices.filter((invoice) => String(invoice.payment_status) === "unpaid"),
    [invoices]
  );

  async function createPlan() {
    try {
      const response = await proxyClient<{ message: string }>("/installments", {
        method: "POST",
        body: JSON.stringify({
          invoice_id: invoiceId,
          months: Number(months),
          start_date: startDate || undefined
        })
      });
      setMessage(response.message);
      setInvoiceId("");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Request failed");
    }
  }

  async function recordPayment(paymentId: string, remaining: number) {
    try {
      const response = await proxyClient<{ message: string }>(
        `/installments/payments/${paymentId}/pay`,
        {
          method: "POST",
          body: JSON.stringify({
            amount: Number(payAmounts[paymentId] || remaining),
            payment_type: payTypes[paymentId] || "cash"
          })
        }
      );
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Request failed");
    }
  }

  return (
    <div className="space-y-6">
      <MetricGrid
        items={[
          { key: "plan_count", label: "Plans", value: Number(summary.plan_count || 0) },
          {
            key: "outstanding_total",
            label: "Outstanding",
            value: currency(Number(summary.outstanding_total || 0))
          },
          { key: "overdue_count", label: "Overdue Installments", value: Number(summary.overdue_count || 0) }
        ]}
      />

      {canManage ? (
        <SectionCard
          title="Create Installment Plan"
          description="Split an unpaid invoice into monthly installments. Amounts and statuses are managed by the backend."
        >
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <Select value={invoiceId} onChange={(event) => setInvoiceId(event.target.value)}>
              <option value="">Select unpaid invoice</option>
              {eligibleInvoices.map((invoice) => (
                <option key={String(invoice.id)} value={String(invoice.id)}>
                  {String(invoice.invoice_number)} · {String(invoice.customer_name)} ·{" "}
                  {currency(Number(invoice.total_amount || 0))}
                </option>
              ))}
            </Select>
            <Input
              aria-label="Months"
              value={months}
              onChange={(event) => setMonths(event.target.value)}
              placeholder="Months"
              type="number"
            />
            <Input
              aria-label="First Due Date"
              value={startDate}
              onChange={(event) => setStartDate(event.target.value)}
              type="date"
            />
            <Button disabled={!invoiceId} onClick={createPlan} type="button">
              Create Plan
            </Button>
          </div>
        </SectionCard>
      ) : null}

      {message ? <p className="text-sm text-black/70">{message}</p> : null}

      {plans.map((plan) => {
        const payments = (plan.payments as Option[]) || [];
        return (
          <SectionCard
            key={String(plan.id)}
            title={`${plan.invoice_number} · ${plan.customer_name}`}
            description={`${plan.branch_name} · ${plan.months} months · ${currency(Number(plan.monthly_amount || 0))}/month`}
            actions={
              <div className="flex items-center gap-2">
                <Badge>{String(plan.status)}</Badge>
                <Badge className="bg-surface-50">
                  Outstanding {currency(Number(plan.outstanding || 0))}
                </Badge>
              </div>
            }
          >
            <DataTable
              columns={[
                { key: "seq_number", label: "#" },
                { key: "due_date", label: "Due Date" },
                { key: "amount", label: "Amount", type: "currency" },
                { key: "paid_amount", label: "Paid", type: "currency" },
                { key: "remaining", label: "Remaining", type: "currency" },
                { key: "status", label: "Status" }
              ]}
              rows={payments}
              rowActions={
                canCollect
                  ? (row) => {
                      if (String(row.status) === "paid") {
                        return <span className="text-xs text-black/40">Settled</span>;
                      }
                      const paymentId = String(row.id);
                      const remaining = Number(row.remaining || 0);
                      return (
                        <div className="flex items-center justify-end gap-2">
                          <Input
                            aria-label={`Amount ${paymentId}`}
                            className="w-28"
                            value={payAmounts[paymentId] ?? String(remaining)}
                            onChange={(event) =>
                              setPayAmounts((current) => ({ ...current, [paymentId]: event.target.value }))
                            }
                            type="number"
                          />
                          <Select
                            aria-label={`Payment Type ${paymentId}`}
                            className="w-36"
                            value={payTypes[paymentId] || "cash"}
                            onChange={(event) =>
                              setPayTypes((current) => ({ ...current, [paymentId]: event.target.value }))
                            }
                          >
                            <option value="cash">Cash</option>
                            <option value="bank_transfer">Bank Transfer</option>
                          </Select>
                          <Button onClick={() => recordPayment(paymentId, remaining)} type="button">
                            Collect
                          </Button>
                        </div>
                      );
                    }
                  : undefined
              }
            />
          </SectionCard>
        );
      })}

      {plans.length === 0 ? (
        <SectionCard title="No Installment Plans" description="Create a plan from an unpaid invoice to get started.">
          <p className="text-sm text-black/55">Installment plans and overdue tracking will appear here.</p>
        </SectionCard>
      ) : null}
    </div>
  );
}
