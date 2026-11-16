import { notFound } from "next/navigation";

import { ApiError } from "@/services/api-server";
import { getInvoicePrint, requireSession } from "@/services/erp";
import { currency, dateTime } from "@/lib/utils";
import { PrintButton } from "@/components/ui/print-button";
import { BusinessDocument } from "@/components/print/business-document";

export default async function InvoicePrintPage({
  params
}: {
  params: Promise<{ invoiceID: string }>;
}) {
  await requireSession();
  const { invoiceID } = await params;
  const payload = await getInvoicePrint(invoiceID).catch((error: unknown) => {
    // A wrong or deleted invoice number is "not found", not a crash.
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  });
  const document = (payload.document as Record<string, unknown>) || {};
  const company = (payload.company as Record<string, unknown>) || {};
  const branch = (payload.branch as Record<string, unknown>) || {};
  const summary = (payload.summary as Record<string, unknown>) || {};
  const items = (payload.items as Array<Record<string, unknown>>) || [];
  const payments = (payload.payments as Array<Record<string, unknown>>) || [];
  // A customer document never names a stock bucket, for any role.

  // A cancelled bill is a voided document — most often an abbreviated tax
  // invoice the customer came back to swap for a full one. It must never print
  // looking like a valid receipt, or the same sale is evidenced twice.
  const cancelled = String(document.invoice_status || "") === "cancelled";

  const fullTax = document.tax_invoice_type === "full";
  const member = document.customer_code
    ? `สมาชิก ${String(document.customer_code)} · แต้มบิลนี้ +${Number(document.points_earned || 0).toLocaleString("th-TH")}` +
      (Number(document.points_redeemed || 0) ? ` / ใช้ ${Number(document.points_redeemed).toLocaleString("th-TH")}` : "") +
      (document.points_balance != null ? ` · คงเหลือ ${Number(document.points_balance).toLocaleString("th-TH")} แต้ม` : "")
    : "";
  const paymentLines = payments.map((payment) => `${paymentTypeLabel(String(payment.payment_type || ""))} ${currency(Number(payment.amount || 0))}`);
  if (document.sale_type === "credit") {
    const due = document.due_date
      ? new Date(String(document.due_date)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok", dateStyle: "long" })
      : "-";
    paymentLines.push(`ขายเชื่อ · ครบกำหนดชำระ ${due} · ค้างชำระ ${currency(Number(document.outstanding || 0))}`);
  }

  return (
    <main className="mx-auto max-w-4xl px-8 py-10 print:max-w-none print:px-0 print:py-0">
      <BusinessDocument
        actions={<PrintButton />}
        banner={
          cancelled ? (
            <>
              <p className="text-lg font-bold tracking-widest">ยกเลิกแล้ว</p>
              <p className="mt-1">เอกสารนี้ถูกยกเลิกและออกใบกำกับภาษีเต็มรูปแทนแล้ว ใช้เป็นหลักฐานการชำระเงินไม่ได้</p>
            </>
          ) : null
        }
        columns={[
          { key: "item", label: "รายการ" },
          { key: "lot", label: "Lot / หมดอายุ" },
          { key: "quantity", label: "จำนวน", align: "right" },
          { key: "unit", label: "ราคาต่อหน่วย", hint: "รวม VAT", align: "right" },
          { key: "total", label: "จำนวนเงิน", hint: "รวม VAT", align: "right" }
        ]}
        emptyText="เอกสารนี้ไม่มีรายการสินค้า"
        facts={[
          { label: "วันที่", value: dateTime(String(document.issued_at || "")) },
          { label: "สาขา", value: `${String(branch.name || "-")}${branch.code ? ` (${String(branch.code)})` : ""}` },
          { label: "ผู้ขาย", value: String(document.seller_name || "-") },
          ...(document.is_government_mode ? [{ label: "รูปแบบ", value: "เอกสารราชการ" }] : [])
        ]}
        issuer={{
          name: String(company.name || "-"),
          lines: [
            String(company.address || ""),
            company.tax_id ? `เลขประจำตัวผู้เสียภาษี ${String(company.tax_id)}` : "",
            branch.address ? `สาขา ${String(branch.name || "")} ${String(branch.address)}` : ""
          ]
        }}
        notes={[
          { label: "การชำระเงิน", body: paymentLines.length ? paymentLines.join("\n") : "ยังไม่มีรายการรับชำระ" },
          // A replacement full tax invoice says which slip it stands in for, so
          // the customer holding it does not have to take the pairing on trust.
          ...(document.notes ? [{ label: "หมายเหตุ", body: String(document.notes) }] : [])
        ]}
        number={String(document.invoice_number || "-")}
        party={{
          label: "ลูกค้า",
          name: String(document.customer_name || "-"),
          lines: [document.customer_tax_id ? `เลขประจำตัวผู้เสียภาษี ${String(document.customer_tax_id)}` : "", member]
        }}
        rows={items.map((item) => ({
          item: (
            <>
              {String(item.display_name || "-")}
              {item.actual_name && item.actual_name !== item.display_name ? (
                <span className="block text-[11px] text-neutral-600">สินค้าจริง: {String(item.actual_name)}</span>
              ) : null}
              {Number(item.discount_amount || 0) > 0 ? (
                <span className="block text-[11px] text-neutral-600">ส่วนลดรายการ {currency(Number(item.discount_amount))}</span>
              ) : null}
            </>
          ),
          lot: (
            <>
              {String(item.lot_number || "-")}
              <span className="block text-[11px] text-neutral-600">
                {item.lot_expires_on
                  ? new Date(String(item.lot_expires_on)).toLocaleDateString("th-TH", { timeZone: "Asia/Bangkok" })
                  : "ไม่กำหนด"}
              </span>
            </>
          ),
          quantity: String(item.quantity || "-"),
          unit: currency(unitPriceWithTax(item)),
          // Both money columns carry VAT, so a line reads as what it costs the
          // customer and the column foots to the grand total.
          total: currency(lineTotalWithTax(item))
        }))}
        signatures={["ผู้รับเงิน", "ผู้รับสินค้า"]}
        subtitle={fullTax ? "ใบกำกับภาษีเต็มรูป" : "ใบกำกับภาษีอย่างย่อ"}
        title="ใบเสร็จรับเงิน"
        totals={[
          ...(Number(document.points_discount || 0) > 0
            ? [{ label: `ส่วนลดจากแต้ม (${Number(document.points_redeemed || 0).toLocaleString("th-TH")} แต้ม)`, value: `-${currency(Number(document.points_discount))}` }]
            : []),
          { label: "ยอดก่อนภาษี", value: currency(Number(summary.subtotal || 0)) },
          { label: `VAT ${Number(summary.tax_rate || 0)}%`, value: currency(Number(summary.tax_amount || 0)) },
          { label: "ยอดรวมทั้งสิ้น", value: currency(Number(summary.total_amount || 0)), strong: true }
        ]}
      />
    </main>
  );
}

/**
 * line_total is the line with VAT already on it, which is the figure the
 * customer is actually being asked for. It is taken as stored rather than
 * recomputed, so the column adds up to ยอดรวม exactly instead of drifting by a
 * satang of rounding.
 */
function lineTotalWithTax(item: Record<string, unknown>) {
  if (item.line_total != null) return Number(item.line_total);
  const subtotal =
    item.line_subtotal == null
      ? Number(item.unit_price || 0) * Number(item.quantity || 0) - Number(item.discount_amount || 0)
      : Number(item.line_subtotal);
  return subtotal * (1 + Number(item.tax_rate || 0) / 100);
}

/** The per-unit share of that line, so price × quantity reads back to the total. */
function unitPriceWithTax(item: Record<string, unknown>) {
  const quantity = Number(item.quantity || 0);
  if (quantity > 0) return lineTotalWithTax(item) / quantity;
  return Number(item.unit_price || 0) * (1 + Number(item.tax_rate || 0) / 100);
}

function paymentTypeLabel(value: string) {
  const labels: Record<string, string> = {
    bank_transfer: "เงินโอน",
    cash: "เงินสด"
  };
  return labels[value] || "-";
}
