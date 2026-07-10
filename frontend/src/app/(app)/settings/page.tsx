import { PageIntro } from "@/components/sections/common";
import { SettingsConsole } from "@/components/sections/settings-console";
import { requireRole } from "@/lib/rbac";
import {
  getAuditLogs,
  getBranches,
  getMarketplaceOrders,
  getMarketplaceProviders,
  getPermissions,
  getRoles,
  getSequences,
  getUsers,
  requireSession
} from "@/services/erp";

export default async function SettingsPage({
  searchParams
}: {
  searchParams?: {
    tab?: string | string[];
    branch_id?: string | string[];
    entity_type?: string | string[];
    action?: string | string[];
    date_from?: string | string[];
    date_to?: string | string[];
  };
}) {
  requireRole(await requireSession(), ["super_admin"]);
  const auditFilters = {
    branch_id: typeof searchParams?.branch_id === "string" ? searchParams.branch_id : "",
    entity_type: typeof searchParams?.entity_type === "string" ? searchParams.entity_type : "",
    action: typeof searchParams?.action === "string" ? searchParams.action : "",
    date_from: typeof searchParams?.date_from === "string" ? searchParams.date_from : "",
    date_to: typeof searchParams?.date_to === "string" ? searchParams.date_to : ""
  };
  const [branches, users, roles, permissions, sequences, providers, marketplaceOrders, auditLogs] =
    await Promise.all([
      getBranches(),
      getUsers(),
      getRoles(),
      getPermissions(),
      getSequences(),
      getMarketplaceProviders(),
      getMarketplaceOrders(),
      getAuditLogs(auditFilters)
    ]);

  return (
    <div className="space-y-6">
      <PageIntro
        eyebrow="Settings"
        title="Settings"
        description="Manage branches, roles, users, invoice sequence, marketplace configuration, and audit logs from a single super-admin workspace."
      />
      <SettingsConsole
        auditLogs={auditLogs.items}
        auditFilters={auditFilters}
        branches={branches.items}
        defaultTab={typeof searchParams?.tab === "string" ? searchParams.tab : "branches"}
        marketplaceOrders={marketplaceOrders.items}
        permissions={permissions.items}
        providers={providers.items}
        roles={roles.items}
        sequences={sequences.items}
        users={users.items}
      />
    </div>
  );
}
