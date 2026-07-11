"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function InvoiceConsole({ invoices }: { invoices: Option[] }) {
  const router = useRouter();
  const payableInvoices = invoices.filter((invoice) => String(invoice.payment_status) !== "paid");
  const [invoiceId, setInvoiceId] = useState("");
  const [paymentType, setPaymentType] = useState("cash");
  const [referenceCode, setReferenceCode] = useState("");
  const [notes, setNotes] = useState("");
  const [message, setMessage] = useState("");

  async function submit() {
    try {
      const response = await proxyClient<{ message: string }>(`/invoices/${invoiceId}/pay`, {
        method: "POST",
        body: JSON.stringify({
          payment_type: paymentType,
          reference_code: referenceCode,
          notes
        })
      });
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Payment failed");
    }
  }

  return (
    <SectionCard title="Collect Full Payment" description="Cash and bank transfer settle the invoice in full at the backend.">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Select aria-label="Payment Invoice" value={invoiceId} onChange={(event) => setInvoiceId(event.target.value)}>
          <option value="">Select invoice</option>
          {payableInvoices.map((invoice) => (
            <option key={String(invoice.id)} value={String(invoice.id)}>
              {String(invoice.invoice_number)}
            </option>
          ))}
        </Select>
        <Select aria-label="Payment Type" value={paymentType} onChange={(event) => setPaymentType(event.target.value)}>
          <option value="cash">Cash</option>
          <option value="bank_transfer">Bank Transfer</option>
        </Select>
        <Input value={referenceCode} onChange={(event) => setReferenceCode(event.target.value)} placeholder="Reference" />
        <Input value={notes} onChange={(event) => setNotes(event.target.value)} placeholder="Notes" />
      </div>
      <div className="mt-4 flex items-center gap-3">
        <Button onClick={submit} type="button">
          Collect Payment
        </Button>
        {message ? <p className="text-sm text-muted-foreground">{message}</p> : null}
      </div>
    </SectionCard>
  );
}
