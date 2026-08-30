import { DataTable, PageIntro, SectionCard } from "@/components/sections/common";
import { requirePermission } from "@/lib/rbac";
import { getProfitLossReport, getTaxReport, requireSession } from "@/services/erp";

export default async function GlobalReportsPage() {
  requirePermission(await requireSession(), ["reports.view.global"]);
  const [tax, profitLoss] = await Promise.all([getTaxReport(), getProfitLossReport()]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="รายงาน"
        description="รายงานภาษีและกำไรขาดทุนจากยอดขายและต้นทุนที่บันทึกจริง"
      />
      <div className="space-y-6">
        <SectionCard title="รายงานภาษี" description="สรุปใบขายแยกตามสาขา">
          <DataTable
            columns={[
              { key: "branch_name", label: "สาขา" },
              { key: "invoice_count", label: "จำนวนใบขาย" },
              { key: "subtotal", label: "ยอดก่อนภาษี", type: "currency" },
              { key: "tax_amount", label: "VAT", type: "currency" },
              { key: "total_amount", label: "ยอดรวม", type: "currency" }
            ]}
            rows={tax.items}
          />
        </SectionCard>
        <SectionCard title="กำไรและขาดทุน" description="คำนวณจากยอดขาย ต้นทุน และกำไรตามข้อมูลจริงในระบบ">
          <DataTable
            columns={[
              { key: "branch_name", label: "สาขา" },
              { key: "sales_revenue", label: "ยอดขาย", type: "currency" },
              { key: "cost", label: "ต้นทุน", type: "currency" },
              { key: "profit", label: "กำไร", type: "currency" }
            ]}
            rows={profitLoss.items}
          />
        </SectionCard>
      </div>
    </div>
  );
}
