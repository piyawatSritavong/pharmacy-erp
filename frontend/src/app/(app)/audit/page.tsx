import { AuditConsole } from "@/components/sections/audit-console";
import { PageIntro } from "@/components/sections/common";
import { requirePermission } from "@/lib/rbac";
import { getAuditLogs, getBranches, requireSession } from "@/services/erp";

export default async function AuditPage({
  searchParams
}: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
}) {
  // Gated on the same permission as GET /audit-logs. business-flow.md also
  // wants branch-scoped roles to see their own branch's history; that needs a
  // new permission + backend filter and is flagged, not silently added here.
  requirePermission(await requireSession(), ["audit.view.global"]);
  const params = await searchParams;
  const value = (key: string) => (typeof params?.[key] === "string" ? String(params[key]) : "");
  const filters = {
    branch_id: value("branch_id"),
    entity_type: value("entity_type"),
    action: value("action"),
    date_from: value("date_from"),
    date_to: value("date_to")
  };

  const [branches, logs] = await Promise.all([getBranches(), getAuditLogs(filters)]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="ประวัติระบบ"
        description="ประวัติการทำงานทุกอย่างของระบบ — ผู้ดูแลระบบเห็นทุกสาขา บทบาทอื่นเห็นเฉพาะสาขาที่สังกัด"
      />
      <AuditConsole branches={branches.items} filters={filters} items={logs.items} />
    </div>
  );
}
