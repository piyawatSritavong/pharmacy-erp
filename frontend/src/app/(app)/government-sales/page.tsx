import { PageIntro } from "@/components/sections/common";
import { SalesDocumentsConsole } from "@/components/sections/sales-documents-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getInvoices, getProducts, getQuotations, requireSession } from "@/services/erp";

export default async function GovernmentSalesPage() {
  const session = requirePermission(await requireSession(), ["quotation.manage"]);
  const [products, branches, quotations, invoices] = await Promise.all([
    getProducts(undefined, { page: 1, pageSize: 200 }),
    getBranches(),
    getQuotations(true),
    getInvoices(true)
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="รพ.สต."
        description="ใบเสนอราคาและใบขายสำหรับหน่วยงานราชการโดยเฉพาะ"
      />
      <SalesDocumentsConsole
        branches={branches.items}
        governmentMode
        invoices={invoices.items}
        products={products.items}
        quotations={quotations.items}
        showFullTimestamp={session.user.role_key === "super_admin"}
      />
    </div>
  );
}
