import { getInvoicePrint, requireSession } from "@/services/erp";
import { currency, dateTime } from "@/lib/utils";
import { PrintButton } from "@/components/ui/print-button";

export default async function InvoicePrintPage({
  params
}: {
  params: { invoiceID: string };
}) {
  await requireSession();
  const payload = await getInvoicePrint(params.invoiceID);
  const document = (payload.document as Record<string, unknown>) || {};
  const company = (payload.company as Record<string, unknown>) || {};
  const branch = (payload.branch as Record<string, unknown>) || {};
  const summary = (payload.summary as Record<string, unknown>) || {};
  const items = (payload.items as Array<Record<string, unknown>>) || [];
  const payments = (payload.payments as Array<Record<string, unknown>>) || [];

  return (
    <main className="mx-auto max-w-4xl bg-white px-8 py-10 text-black print:max-w-none print:px-4">
      <div className="flex items-start justify-between gap-6 border-b border-black pb-6">
        <div className="space-y-2">
          <p className="text-xs uppercase tracking-[0.24em] text-black/55">Tax Invoice</p>
          <h1 className="text-3xl font-semibold">{String(document.invoice_number || "-")}</h1>
          <p className="text-sm text-black/65">{dateTime(String(document.issued_at || ""))}</p>
        </div>
        <PrintButton />
      </div>

      <section className="mt-6 grid gap-6 md:grid-cols-3">
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-black/45">Company</p>
          <h2 className="mt-2 text-lg font-semibold">{String(company.name || "-")}</h2>
          <p className="mt-2 text-sm text-black/65">{String(company.address || "-")}</p>
          <p className="mt-2 text-sm text-black/65">Tax ID: {String(company.tax_id || "-")}</p>
        </div>
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-black/45">Branch</p>
          <h2 className="mt-2 text-lg font-semibold">{String(branch.name || "-")}</h2>
          <p className="mt-2 text-sm text-black/65">{String(branch.address || "-")}</p>
          <p className="mt-2 text-sm text-black/65">Code: {String(branch.code || "-")}</p>
        </div>
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-black/45">Customer</p>
          <h2 className="mt-2 text-lg font-semibold">{String(document.customer_name || "-")}</h2>
          <p className="mt-2 text-sm text-black/65">Tax ID: {String(document.customer_tax_id || "-")}</p>
          <p className="mt-2 text-sm text-black/65">Seller: {String(document.seller_name || "-")}</p>
          <p className="mt-2 text-sm text-black/65">
            Mode: {Boolean(document.is_government_mode) ? "Government mode" : "Retail"}
          </p>
        </div>
      </section>

      <section className="mt-8">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead>
            <tr className="border-y border-black">
              <th className="py-3 pr-3">Display Name</th>
              <th className="py-3 pr-3">Actual Product</th>
              <th className="py-3 pr-3">Qty</th>
              <th className="py-3 pr-3">Bucket</th>
              <th className="py-3 pr-3 text-right">Unit</th>
              <th className="py-3 pr-0 text-right">Total</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, index) => (
              <tr key={String(item.id || index)} className="border-b border-black/10">
                <td className="py-3 pr-3">{String(item.display_name || "-")}</td>
                <td className="py-3 pr-3 text-black/55">{String(item.actual_name || "-")}</td>
                <td className="py-3 pr-3">{String(item.quantity || "-")}</td>
                <td className="py-3 pr-3">{String(item.stock_bucket || "-")}</td>
                <td className="py-3 pr-3 text-right">{currency(Number(item.unit_price || 0))}</td>
                <td className="py-3 pr-0 text-right">{currency(Number(item.line_total || 0))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="mt-8 grid gap-6 md:grid-cols-[1fr_320px]">
        <div>
          <p className="text-xs uppercase tracking-[0.2em] text-black/45">Payments</p>
          <div className="mt-3 space-y-2">
            {payments.length ? (
              payments.map((payment, index) => (
                <div key={String(payment.id || index)} className="rounded-md border border-black/10 px-4 py-3">
                  <p className="font-medium">{String(payment.payment_type || "-")}</p>
                  <p className="text-sm text-black/65">
                    {currency(Number(payment.amount || 0))} / {String(payment.reference_code || "-")}
                  </p>
                </div>
              ))
            ) : (
              <p className="text-sm text-black/55">No payment has been collected yet.</p>
            )}
          </div>
        </div>
        <div className="rounded-lg border border-black p-4">
          <div className="flex items-center justify-between py-2 text-sm">
            <span>Subtotal</span>
            <span>{currency(Number(summary.subtotal || 0))}</span>
          </div>
          <div className="flex items-center justify-between py-2 text-sm">
            <span>VAT ({Number(summary.tax_rate || 0)}%)</span>
            <span>{currency(Number(summary.tax_amount || 0))}</span>
          </div>
          <div className="mt-2 flex items-center justify-between border-t border-black pt-3 text-base font-semibold">
            <span>Total</span>
            <span>{currency(Number(summary.total_amount || 0))}</span>
          </div>
        </div>
      </section>
    </main>
  );
}
