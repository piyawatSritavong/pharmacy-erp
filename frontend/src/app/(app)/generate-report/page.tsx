import { PageIntro } from "@/components/sections/common";
import { GenerateReportConsole } from "@/components/sections/report-builder";
import { requirePermission } from "@/lib/rbac";
import { requireSession } from "@/services/erp";

export default async function GenerateReportPage() {
  const session = requirePermission(await requireSession(), ["reports.generate.global"]);
  return (
    <div className="space-y-6">
      <PageIntro
        title="Generate Report"
        description="สร้างและปักหมุดรายงานแบบกำหนดเองจากข้อมูลทุกสาขา — อ่านอย่างเดียว ไม่แก้ไขข้อมูลต้นทาง"
      />
      <GenerateReportConsole navigation={session.navigation} />
    </div>
  );
}
