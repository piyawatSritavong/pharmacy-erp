import { ConvertQuotationButton } from "@/components/sections/quotation-actions";
import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { DocumentComposer } from "@/components/sections/document-composer";
import { requireRole } from "@/lib/rbac";
import { getBranches, getInvoices, getProducts, getQuotations, requireSession } from "@/services/erp";

export default async function SalesInvoicesPage() {
  const session = requireRole(await requireSession(), ["branch_admin"]);
  const branchId = session.user.branch_id;
  const [products, branches, quotations, invoices] = await Promise.all([
    getProducts(branchId),
    getBranches(),
    getQuotations(),
    getInvoices()
  ]);
  const branchOptions = branches.items.filter((item) => String(item.id) === String(branchId || ""));

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Sales"
        title="Sales & Invoices"
        description="Create quotations, issue invoices, and reprint immutable invoice snapshots. Cash and bank collection stays on the POS role only."
      />
      <Grid className="xl:grid-cols-2">
        <DocumentComposer
          branches={branchOptions}
          defaultBranchId={branchId}
          description="Branch quotation flow. Preview and totals are always computed by the backend."
          kind="quotation"
          products={products.items}
          title="Create Quotation"
        />
        <DocumentComposer
          branches={branchOptions}
          defaultBranchId={branchId}
          description="Direct invoice issue for branch staff."
          kind="invoice"
          products={products.items}
          title="Create Invoice"
        />
      </Grid>

      <SectionCard title="Quotation Log" description="Draft quotations and conversion to invoice">
        <DataTable
          columns={[
            { key: "quote_number", label: "Quotation" },
            { key: "customer_name", label: "Customer" },
            { key: "status", label: "Status" },
            { key: "total_amount", label: "Total", type: "currency" },
            { key: "created_at", label: "Created", type: "datetime" }
          ]}
          rowActions={(row) =>
            String(row.status) === "draft" ? <ConvertQuotationButton id={String(row.id)} /> : null
          }
          rows={quotations.items}
        />
      </SectionCard>

      <SectionCard title="Invoice History" description="Branch invoice history with print / reprint">
        <DataTable
          columns={[
            { key: "invoice_number", label: "Invoice" },
            { key: "customer_name", label: "Customer" },
            { key: "payment_status", label: "Payment" },
            { key: "is_government_mode", label: "Gov Mode" },
            { key: "total_amount", label: "Total", type: "currency" },
            { key: "issued_at", label: "Issued", type: "datetime" }
          ]}
          rowActions={(row) => (
            <a
              className="inline-flex rounded-md border border-border px-3 py-2 text-sm hover:bg-black/[0.03]"
              href={`/print/invoices/${String(row.id)}`}
              rel="noreferrer"
              target="_blank"
            >
              Print / Reprint
            </a>
          )}
          rows={invoices.items}
        />
      </SectionCard>
    </div>
  );
}
