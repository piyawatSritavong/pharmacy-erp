import { PageIntro } from "@/components/sections/common";
import { FdaReportConsole } from "@/components/sections/fda-report-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getFdaReportSummary, getProductCategories, requireSession } from "@/services/erp";

export default async function FdaReportsPage({
  searchParams
}: {
  searchParams?: Promise<{ date_from?: string; date_to?: string; branch_id?: string; category_id?: string }>;
}) {
  requirePermission(await requireSession(), ["fda.manage"]);
  const resolved = await searchParams;
  const dateFrom = resolved?.date_from || "";
  const dateTo = resolved?.date_to || "";
  const branchId = resolved?.branch_id || "";
  const categoryId = resolved?.category_id || "";

  const [summary, branches, categories] = await Promise.all([
    getFdaReportSummary({ dateFrom, dateTo, branchId, categoryId }),
    getBranches(),
    getProductCategories()
  ]);

  return (
    <div className="space-y-6 print:space-y-0">
      <div className="print:hidden">
        <PageIntro
          title="เอกสารนำส่ง อย."
          description="เลือกช่วงเวลาและสินค้าที่ต้องรายงาน แล้วพิมพ์เอกสารนำส่งสำนักงานคณะกรรมการอาหารและยา"
        />
      </div>
      <FdaReportConsole
        branches={branches.items}
        categories={categories.items}
        defaultBranchId={branchId}
        defaultCategoryId={categoryId}
        defaultDateFrom={String(summary.date_from || dateFrom)}
        defaultDateTo={String(summary.date_to || dateTo)}
        summary={{
          company_name: String(summary.company_name || ""),
          company_address: String(summary.company_address || ""),
          company_tax_id: String(summary.company_tax_id || ""),
          fda_license_no: String(summary.fda_license_no || ""),
          date_from: String(summary.date_from || dateFrom),
          date_to: String(summary.date_to || dateTo),
          items: (summary.items as Array<Record<string, unknown>>) || []
        }}
      />
    </div>
  );
}
