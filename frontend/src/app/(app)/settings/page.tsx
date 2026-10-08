import type { Metadata } from "next";
import { PageIntro } from "@/components/sections/common";
import { SettingsConsole } from "@/components/sections/settings-console";
import { requirePermission } from "@/lib/rbac";
import { ApiError } from "@/services/api-server";
import {
  getBranches,
  getMarketplaceOrders,
  getMarketplaceProviders,
  getPermissions,
  getRoles,
  getSequences,
  getUsers,
  requireSession
} from "@/services/erp";

export const metadata: Metadata = { title: "ตั้งค่า" };

export default async function SettingsPage({
  searchParams
}: {
  searchParams?: Promise<{ tab?: string | string[] }>;
}) {
  const session = requirePermission(await requireSession(), ["settings.manage", "users.manage"]);
  const resolvedSearchParams = await searchParams;
  // D11: this page is reachable with EITHER settings.manage OR users.manage
  // (see requirePermission above), but users/roles/permissions specifically
  // require users.manage on the backend. A role holding only settings.manage
  // (e.g. แอดมิน, by design — see migration 025) would otherwise 403 on
  // getUsers()/getRoles()/getPermissions() and, since these were all in one
  // Promise.all, take the *whole* page down with them. Degrade those three
  // to an empty list instead so the Branches/Sequences/Marketplace tabs such
  // a role legitimately has access to still render; the ผู้ใช้ and
  // บทบาทและสิทธิ์ tabs just end up empty for them, which is correct.
  const emptyList = { items: [] as Array<Record<string, unknown>> };
  // Only a 403 means "this role doesn't manage users". Any other failure is a
  // real error and goes to the error page, rather than passing for an empty
  // user list.
  const usersOnly = (promise: Promise<{ items: Array<Record<string, unknown>> }>) =>
    promise.catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 403) return emptyList;
      throw error;
    });
  const [branches, users, roles, permissions, sequences, providers, marketplaceOrders] =
    await Promise.all([
      getBranches(),
      usersOnly(getUsers()),
      usersOnly(getRoles()),
      usersOnly(getPermissions()),
      getSequences(),
      getMarketplaceProviders(),
      getMarketplaceOrders()
    ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="ตั้งค่า"
        description="จัดการสาขา ผู้ใช้ บทบาทและสิทธิ์ เลขที่เอกสาร และตลาดออนไลน์"
      />
      <SettingsConsole
        branches={branches.items}
        currentUserId={session.user.id}
        defaultTab={typeof resolvedSearchParams?.tab === "string" ? resolvedSearchParams.tab : "branches"}
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
