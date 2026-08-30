import { PageIntro } from "@/components/sections/common";
import { SalesDocumentsConsole } from "@/components/sections/sales-documents-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getInvoices, getProducts, getQuotations, requireSession } from "@/services/erp";

export default async function SalesManagementPage() {
  const session = requirePermission(await requireSession(), ["quotation.manage"]);
  const [products, branches, quotations, invoices] = await Promise.all([
    getProducts(undefined, { page: 1, pageSize: 200 }),
    getBranches(),
    getQuotations(false),
    getInvoices(false)
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
