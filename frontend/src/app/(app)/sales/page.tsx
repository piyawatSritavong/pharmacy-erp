import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { DocumentComposer } from "@/components/sections/document-composer";
import { InvoiceConsole } from "@/components/sections/invoice-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getInvoices, getProducts, requireSession } from "@/services/erp";

export default async function SalesPage() {
  const session = requireRole(await requireSession(), ["branch_pos"]);
  const branchId = session.user.branch_id;
  const [products, branches, invoices] = await Promise.all([
    getProducts(branchId),
    getBranches(),
    getInvoices()
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(branchId || ""));

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="POS"
        title="POS Screen"
        description="Thin-client POS screen for retail and government-mode alias billing. Pricing, VAT, stock deduction, and invoice numbering are computed only by the Go backend."
      />
      <Grid>
        <DocumentComposer
          branches={branchOptions}
          defaultBranchId={branchId}
          description="Use the same composer for retail sales or government-mode alias billing."
          kind="invoice"
          products={products.items}
          title="Create POS Invoice"
        />
        <InvoiceConsole invoices={invoices.items} />
      </Grid>
      <SectionCard title="Recent Sales" description="POS invoice history with print view">
        <DataTable
          columns={[
            { key: "invoice_number", label: "Invoice" },
            { key: "customer_name", label: "Customer" },
            { key: "payment_status", label: "Payment" },
            { key: "total_amount", label: "Total", type: "currency" },
            { key: "issued_at", label: "Issued", type: "datetime" }
          ]}
          rowActions={(row) => (
            <a
              className="inline-flex rounded-md border border-black/10 px-3 py-2 text-sm hover:bg-black/[0.03]"
              href={`/print/invoices/${String(row.id)}`}
              rel="noreferrer"
              target="_blank"
            >
              Print
            </a>
          )}
          rows={invoices.items}
        />
      </SectionCard>
    </div>
  );
}
