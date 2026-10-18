import type { Metadata } from "next";
import { PageIntro } from "@/components/sections/common";
import { SalesDocumentsConsole } from "@/components/sections/sales-documents-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getInvoices, getProducts, getQuotations, requireSession } from "@/services/erp";

export const metadata: Metadata = { title: "ใบขาย" };

export default async function SalesManagementPage() {
  const session = requirePermission(await requireSession(), ["quotation.manage"]);
  const [products, branches, quotations, invoices] = await Promise.all([
    getProducts(undefined, { page: 1, pageSize: 200 }),
    getBranches(),
    getQuotations(false),
    // The invoice list pages on the server now; the newest 200 are what this
    // desk works from, and the full history is a sales-history search away.
    getInvoices({ government_mode: "false", page: "1", page_size: "200" })
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="ใบขาย"
        description="สร้างและจัดการใบเสนอราคากับใบขายทั่วไปของทุกสาขา"
      />
      <SalesDocumentsConsole
        branches={branches.items}
        governmentMode={false}
        invoices={invoices.items}
        products={products.items}
        quotations={quotations.items}
        showFullTimestamp={session.user.role_key === "super_admin"}
      />
    </div>
  );
}
