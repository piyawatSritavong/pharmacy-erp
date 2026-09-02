import { getInvoicePrint, requireSession } from "@/services/erp";
import { currency, dateTime } from "@/lib/utils";
import { PrintButton } from "@/components/ui/print-button";

export default async function InvoicePrintPage({
  params
}: {
  params: Promise<{ invoiceID: string }>;
}) {
  await requireSession();
  const { invoiceID } = await params;
  const payload = await getInvoicePrint(invoiceID);
  const document = (payload.document as Record<string, unknown>) || {};
  const company = (payload.company as Record<string, unknown>) || {};
  const branch = (payload.branch as Record<string, unknown>) || {};
  const summary = (payload.summary as Record<string, unknown>) || {};
  const items = (payload.items as Array<Record<string, unknown>>) || [];
  const payments = (payload.payments as Array<Record<string, unknown>>) || [];
  // A customer document never names a stock bucket, for any role.

  return (
    <main className="mx-auto max-w-4xl bg-white px-8 py-10 text-black print:max-w-none print:px-4">
      <div className="flex items-start justify-between gap-6 border-b border-black pb-6">
        <div className="space-y-2">
          <p className="text-xs tracking-[0.18em] text-muted-foreground">{document.tax_invoice_type === "full" ? "ใบกำกับภาษีเต็มรูป / ใบเสร็จรับเงิน" : "ใบกำกับภาษีอย่างย่อ / ใบเสร็จรับเงิน"}</p>
          <h1 className="text-3xl font-semibold">{String(document.invoice_number || "-")}</h1>
          <p className="text-sm text-muted-foreground">{dateTime(String(document.issued_at || ""))}</p>
        </div>
        <PrintButton />
      </div>

      <section className="mt-6 grid gap-6 md:grid-cols-3">
        <div>
          <p className="text-xs text-muted-foreground">บริษัท</p>
          <h2 className="mt-2 text-lg font-semibold">{String(company.name || "-")}</h2>
          <p className="mt-2 text-sm text-muted-foreground">{String(company.address || "-")}</p>
          <p className="mt-2 text-sm text-muted-foreground">เลขประจำตัวผู้เสียภาษี: {String(company.tax_id || "-")}</p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">สาขา</p>
          <h2 className="mt-2 text-lg font-semibold">{String(branch.name || "-")}</h2>
          <p className="mt-2 text-sm text-muted-foreground">{String(branch.address || "-")}</p>
          <p className="mt-2 text-sm text-muted-foreground">รหัส: {String(branch.code || "-")}</p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">ลูกค้า</p>
          <h2 className="mt-2 text-lg font-semibold">{String(document.customer_name || "-")}</h2>
          <p className="mt-2 text-sm text-muted-foreground">เลขประจำตัวผู้เสียภาษี: {String(document.customer_tax_id || "-")}</p>
          <p className="mt-2 text-sm text-muted-foreground">ผู้ขาย: {String(document.seller_name || "-")}</p>
          <p className="mt-2 text-sm text-muted-foreground">
            รูปแบบ: {Boolean(document.is_government_mode) ? "เอกสารราชการ" : "ขายปลีก"}
          </p>
          <p className="mt-2 text-sm text-muted-foreground">ชนิดใบกำกับ: {document.tax_invoice_type === "full" ? "เต็มรูป" : "อย่างย่อ"}</p>
        </div>
      </section>

      <section className="mt-8">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead>
            <tr className="border-y border-black">
              <th className="py-3 pr-3">ชื่อบนเอกสาร</th>
              <th className="py-3 pr-3">สินค้าจริง</th>
              <th className="py-3 pr-3">จำนวน</th>
              <th className="py-3 pr-3">Lot</th>
              <th className="py-3 pr-3 text-right">ราคาต่อหน่วย</th>
              <th className="py-3 pr-0 text-right">รวม</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, index) => (
              <tr key={String(item.id || index)} className="border-b border-border">
                <td className="py-3 pr-3">{String(item.display_name || "-")}</td>
                <td className="py-3 pr-3 text-muted-foreground">{String(item.actual_name || "-")}</td>
                <td className="py-3 pr-3">{String(item.quantity || "-")}</td>
                <td className="py-3 pr-3"><span className="block">{String(item.lot_number || "-")}</span><span className="text-xs text-muted-foreground">หมดอายุ {item.lot_expires_on ? new Date(String(item.lot_expires_on)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" }) : "ไม่กำหนด"}</span></td>
                <td className="py-3 pr-3 text-right">
                  <span className="block">{currency(Number(item.unit_price || 0))}</span>
                  {Number(item.discount_amount || 0) > 0 ? <span className="mt-1 block text-xs text-muted-foreground">ส่วนลดรายการ {currency(Number(item.discount_amount))}</span> : null}
                </td>
                <td className="py-3 pr-0 text-right">{currency(Number(item.line_total || 0))}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="mt-8 grid gap-6 md:grid-cols-[1fr_320px]">
        <div>
          <p className="text-xs text-muted-foreground">การชำระเงิน</p>
          <div className="mt-3 space-y-2">
            {payments.length ? (
              payments.map((payment, index) => (
                <div key={String(payment.id || index)} className="rounded-md border border-border px-4 py-3">
                  <p className="font-medium">{paymentTypeLabel(String(payment.payment_type || ""))}</p>
                  <p className="text-sm text-muted-foreground">
                    {currency(Number(payment.amount || 0))}
                  </p>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted-foreground">ยังไม่มีรายการรับชำระ</p>
            )}
          </div>
        </div>
        <div className="rounded-lg border border-black p-4">
          <div className="flex items-center justify-between py-2 text-sm">
            <span>ยอดก่อนภาษี</span>
            <span>{currency(Number(summary.subtotal || 0))}</span>
          </div>
          <div className="flex items-center justify-between py-2 text-sm">
            <span>VAT ({Number(summary.tax_rate || 0)}%)</span>
            <span>{currency(Number(summary.tax_amount || 0))}</span>
          </div>
          <div className="mt-2 flex items-center justify-between border-t border-black pt-3 text-base font-semibold">
            <span>ยอดรวม</span>
            <span>{currency(Number(summary.total_amount || 0))}</span>
          </div>
        </div>
      </section>
    </main>
  );
}

function paymentTypeLabel(value: string) {
  const labels: Record<string, string> = {
    bank_transfer: "เงินโอน",
    cash: "เงินสด"
  };
  return labels[value] || "-";
}
