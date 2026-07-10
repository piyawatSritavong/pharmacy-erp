"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { InvoiceSummary, SectionCard } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function FinanceConsole({
  outstandingInvoices,
  branches,
  defaultBranchId
}: {
  outstandingInvoices: Option[];
  branches: Option[];
  defaultBranchId?: string;
}) {
  const router = useRouter();
  const [branchId, setBranchId] = useState(defaultBranchId || "");
  const [checkNumber, setCheckNumber] = useState("");
  const [bankName, setBankName] = useState("");
  const [payerName, setPayerName] = useState("");
  const [amount, setAmount] = useState("");
  const [invoiceIds, setInvoiceIds] = useState<string[]>([]);
  const [preview, setPreview] = useState<Record<string, unknown> | null>(null);
  const [message, setMessage] = useState("");

  async function previewApply() {
    try {
      const result = await proxyClient<Record<string, unknown>>("/checks/preview-apply", {
        method: "POST",
        body: JSON.stringify({
          branch_id: branchId,
          check_number: checkNumber,
          bank_name: bankName,
          payer_name: payerName,
          amount: Number(amount),
          invoice_ids: invoiceIds
        })
      });
      setPreview(result);
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Preview failed");
    }
  }

  async function saveCheck() {
    try {
      await proxyClient("/checks", {
        method: "POST",
        body: JSON.stringify({
          branch_id: branchId,
          check_number: checkNumber,
          bank_name: bankName,
          payer_name: payerName,
          amount: Number(amount),
          invoice_ids: invoiceIds
        })
      });
      setMessage("Check saved");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Save failed");
    }
  }

  return (
    <SectionCard title="Check Reconciliation" description="1 check can settle many invoices, but totals must match exactly.">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Select aria-label="Finance Branch" value={branchId} onChange={(event) => setBranchId(event.target.value)}>
          <option value="">Branch</option>
          {branches.map((branch) => (
            <option key={String(branch.id)} value={String(branch.id)}>
              {String(branch.name)}
            </option>
          ))}
        </Select>
        <Input aria-label="Check Number" value={checkNumber} onChange={(event) => setCheckNumber(event.target.value)} placeholder="Check number" />
        <Input aria-label="Bank Name" value={bankName} onChange={(event) => setBankName(event.target.value)} placeholder="Bank name" />
        <Input aria-label="Payer Name" value={payerName} onChange={(event) => setPayerName(event.target.value)} placeholder="Payer" />
        <Input aria-label="Check Amount" value={amount} onChange={(event) => setAmount(event.target.value)} placeholder="Amount" type="number" />
      </div>

      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {outstandingInvoices.map((invoice) => {
          const checked = invoiceIds.includes(String(invoice.id));
          return (
            <label key={String(invoice.id)} className="flex items-center gap-3 rounded-[22px] border border-black/10 bg-surface-50 px-4 py-3 text-sm">
              <input
                aria-label={`Invoice ${String(invoice.invoice_number)}`}
                checked={checked}
                onChange={(event) => {
                  setInvoiceIds((current) =>
                    event.target.checked
                      ? [...current, String(invoice.id)]
                      : current.filter((item) => item !== String(invoice.id))
                  );
                }}
                type="checkbox"
              />
              <span>
                {String(invoice.invoice_number)} / {String(invoice.customer_name)}
              </span>
            </label>
          );
        })}
      </div>

      <div className="mt-4 flex gap-3">
        <Button onClick={previewApply} type="button" variant="secondary">
          Preview Apply
        </Button>
        <Button onClick={saveCheck} type="button">
          Save Check
        </Button>
      </div>

      {preview ? (
        <div className="mt-4">
          <InvoiceSummary
            summary={{
              subtotal: Number(preview.invoices_total || 0),
              tax_rate: 0,
              tax_amount: 0,
              total_amount: Number(preview.check_amount || 0)
            }}
          />
        </div>
      ) : null}
      {message ? <p className="mt-4 text-sm text-black/70">{message}</p> : null}
    </SectionCard>
  );
}
