"use client";

import { FormEvent, startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { AuditTimeline, DataTable, SectionCard } from "@/components/sections/common";
import { MarketplaceConsole } from "@/components/sections/marketplace-console";
import { Button, Checkbox, Input, Select, Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function SettingsConsole({
  branches,
  users,
  roles,
  permissions,
  sequences,
  providers,
  marketplaceOrders,
  auditLogs,
  auditFilters,
  defaultTab = "branches"
}: {
  branches: Option[];
  users: Option[];
  roles: Option[];
  permissions: Option[];
  sequences: Option[];
  providers: Option[];
  marketplaceOrders: Option[];
  auditLogs: Option[];
  auditFilters: Record<string, string>;
  defaultTab?: string;
}) {
  const router = useRouter();
  const [message, setMessage] = useState("");

  async function submitJSON(path: string, method: "POST" | "PUT", body: Record<string, unknown>) {
    try {
      const response = await proxyClient<{ id?: string; message?: string }>(path, {
        method,
        body: JSON.stringify(body)
      });
      setMessage(response.message || "Saved");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Save failed");
    }
  }

  async function createBranch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    await submitJSON("/branches", "POST", {
      code: formData.get("code"),
      name: formData.get("name"),
      address: formData.get("address"),
      active: formData.get("active") === "on"
    });
    form.reset();
  }

  async function updateBranch(branchID: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    await submitJSON(`/branches/${branchID}`, "PUT", {
      code: formData.get("code"),
      name: formData.get("name"),
      address: formData.get("address"),
      active: formData.get("active") === "on"
    });
  }

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    await submitJSON("/users", "POST", {
      full_name: formData.get("full_name"),
      email: formData.get("email"),
      password: formData.get("password"),
      role_id: formData.get("role_id"),
      branch_id: formData.get("branch_id") || undefined,
      active: formData.get("active") === "on"
    });
    form.reset();
  }

  async function updateUser(userID: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    await submitJSON(`/users/${userID}`, "PUT", {
      full_name: formData.get("full_name"),
      email: formData.get("email"),
      role_id: formData.get("role_id"),
      branch_id: formData.get("branch_id") || undefined,
      active: formData.get("active") === "on"
    });
  }

  async function resetPassword(userID: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    await submitJSON(`/users/${userID}/reset-password`, "POST", {
      password: formData.get("password")
    });
    form.reset();
  }

  async function createRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    await submitJSON("/roles", "POST", {
      role_key: formData.get("role_key"),
      name: formData.get("name"),
      active: formData.get("active") === "on"
    });
    form.reset();
  }

  async function updateRole(roleID: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    await submitJSON(`/roles/${roleID}`, "PUT", {
      name: formData.get("name"),
      active: formData.get("active") === "on"
    });
  }

  async function updateRolePermissions(roleID: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    await submitJSON(`/roles/${roleID}/permissions`, "PUT", {
      permission_keys: formData.getAll("permission_keys").map(String)
    });
  }

  async function updateSequence(sequence: Option, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    await submitJSON(`/branches/${String(sequence.branch_id)}/sequences/${String(sequence.doc_type)}`, "PUT", {
      prefix: formData.get("prefix"),
      next_number: Number(formData.get("next_number")),
      is_locked: formData.get("is_locked") === "on"
    });
  }

  return (
    <div className="space-y-4">
      {message ? <p className="rounded-md border border-border bg-white px-4 py-3 text-sm text-muted-foreground">{message}</p> : null}
      <Tabs className="space-y-6" defaultValue={defaultTab}>
        <TabsList>
          <TabsTrigger value="branches">Branches</TabsTrigger>
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="roles">Roles</TabsTrigger>
          <TabsTrigger value="sequences">Sequences</TabsTrigger>
          <TabsTrigger value="marketplace">Marketplace</TabsTrigger>
          <TabsTrigger value="audit">Audit Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="branches" className="space-y-6">
          <SectionCard title="Create Branch" description="Create a branch and provision invoice / quotation sequences automatically.">
            <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-4" onSubmit={createBranch}>
              <Input name="code" placeholder="Code" />
              <Input name="name" placeholder="Branch name" />
              <Input className="xl:col-span-2" name="address" placeholder="Address" />
              <label className="flex items-center gap-3 rounded-md border border-border px-4 py-3 text-sm">
                <Checkbox defaultChecked name="active" />
                Active
              </label>
              <Button type="submit">Create Branch</Button>
            </form>
          </SectionCard>

          <SectionCard title="Branch Directory" description="Update branch code, name, address, and active status.">
            <div className="space-y-3">
              {branches.map((branch) => (
                <form
                  key={String(branch.id)}
                  className="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 md:grid-cols-2 xl:grid-cols-5"
                  onSubmit={(event) => void updateBranch(String(branch.id), event)}
                >
                  <Input defaultValue={String(branch.code)} name="code" />
                  <Input defaultValue={String(branch.name)} name="name" />
                  <Input defaultValue={String(branch.address || "")} name="address" />
                  <label className="flex items-center gap-3 rounded-md border border-border bg-white px-4 py-3 text-sm">
                    <Checkbox defaultChecked={Boolean(branch.active)} name="active" />
                    Active
                  </label>
                  <Button type="submit">Save</Button>
                </form>
              ))}
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent value="users" className="space-y-6">
          <SectionCard title="Create User" description="Create a real login account, role assignment, and branch assignment.">
            <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-4" onSubmit={createUser}>
              <Input name="full_name" placeholder="Full name" />
              <Input name="email" placeholder="Email" type="email" />
              <Input name="password" placeholder="Password" type="password" />
              <Select aria-label="User Role" name="role_id">
                <option value="">Role</option>
                {roles.map((role) => (
                  <option key={String(role.id)} value={String(role.id)}>
                    {String(role.name)}
                  </option>
                ))}
              </Select>
              <Select aria-label="User Branch" name="branch_id">
                <option value="">Enterprise / no branch</option>
                {branches.map((branch) => (
                  <option key={String(branch.id)} value={String(branch.id)}>
                    {String(branch.name)}
                  </option>
                ))}
              </Select>
              <label className="flex items-center gap-3 rounded-md border border-border px-4 py-3 text-sm">
                <Checkbox defaultChecked name="active" />
                Active
              </label>
              <Button type="submit">Create User</Button>
            </form>
          </SectionCard>

          <SectionCard title="User Directory" description="Update user assignment and reset passwords.">
            <div className="space-y-5">
              {users.map((user) => (
                <div key={String(user.id)} className="rounded-lg border border-border bg-muted/50 p-4">
                  <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-5" onSubmit={(event) => void updateUser(String(user.id), event)}>
                    <Input defaultValue={String(user.name)} name="full_name" />
                    <Input defaultValue={String(user.email)} name="email" type="email" />
                    <Select defaultValue={String(roles.find((role) => String(role.role_key) === String(user.role_key))?.id || "")} name="role_id">
                      <option value="">Role</option>
                      {roles.map((role) => (
                        <option key={String(role.id)} value={String(role.id)}>
                          {String(role.name)}
                        </option>
                      ))}
                    </Select>
                    <Select defaultValue={String(user.branch_id || "")} name="branch_id">
                      <option value="">Enterprise / no branch</option>
                      {branches.map((branch) => (
                        <option key={String(branch.id)} value={String(branch.id)}>
                          {String(branch.name)}
                        </option>
                      ))}
                    </Select>
                    <label className="flex items-center gap-3 rounded-md border border-border bg-white px-4 py-3 text-sm">
                      <Checkbox defaultChecked={Boolean(user.active)} name="active" />
                      Active
                    </label>
                    <Button className="xl:col-span-5 xl:w-fit" type="submit">
                      Save User
                    </Button>
                  </form>

                  <form className="mt-3 grid gap-3 md:grid-cols-[minmax(0,320px)_auto]" onSubmit={(event) => void resetPassword(String(user.id), event)}>
                    <Input name="password" placeholder={`Reset password for ${String(user.email)}`} type="password" />
                    <Button type="submit" variant="secondary">
                      Reset Password
                    </Button>
                  </form>
                </div>
              ))}
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent value="roles" className="space-y-6">
          <SectionCard title="Create Role" description="Create a custom role and assign permissions afterwards.">
            <form className="grid gap-4 md:grid-cols-2 xl:grid-cols-4" onSubmit={createRole}>
              <Input name="role_key" placeholder="role_key" />
              <Input name="name" placeholder="Role name" />
              <label className="flex items-center gap-3 rounded-md border border-border px-4 py-3 text-sm">
                <Checkbox defaultChecked name="active" />
                Active
              </label>
              <Button type="submit">Create Role</Button>
            </form>
          </SectionCard>

          <div className="space-y-6">
            {roles.map((role) => {
              const selectedPermissions = new Set((role.permissions as string[]) || []);
              return (
                <SectionCard
                  key={String(role.id)}
                  title={`${String(role.name)}${role.is_system ? " · System" : ""}`}
                  description={`Role key: ${String(role.role_key)}`}
                >
                  <form className="grid gap-3 md:grid-cols-3" onSubmit={(event) => void updateRole(String(role.id), event)}>
                    <Input defaultValue={String(role.name)} name="name" />
                    <label className="flex items-center gap-3 rounded-md border border-border px-4 py-3 text-sm">
                      <Checkbox defaultChecked={Boolean(role.active)} name="active" />
                      Active
                    </label>
                    <Button type="submit">Save Role</Button>
                  </form>

                  <form className="mt-4" onSubmit={(event) => void updateRolePermissions(String(role.id), event)}>
                    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                      {permissions.map((permission) => (
                        <label
                          key={String(permission.permission_key)}
                          className="flex items-start gap-3 rounded-md border border-border bg-muted/50 px-4 py-3 text-sm"
                        >
                          <Checkbox
                            defaultChecked={selectedPermissions.has(String(permission.permission_key))}
                            name="permission_keys"
                            value={String(permission.permission_key)}
                          />
                          <span>
                            <strong className="block text-black">{String(permission.name)}</strong>
                            <span className="text-muted-foreground">{String(permission.permission_key)}</span>
                          </span>
                        </label>
                      ))}
                    </div>
                    <Button className="mt-4" type="submit" variant="secondary">
                      Save Permissions
                    </Button>
                  </form>
                </SectionCard>
              );
            })}
          </div>
        </TabsContent>

        <TabsContent value="sequences">
          <SectionCard title="Sequence Settings" description="Per-branch document numbering is configured centrally. Locked sequences still issue documents, but prefix and next number must be unlocked before editing.">
            <div className="space-y-3">
              {sequences.map((sequence) => (
                <form
                  key={String(sequence.id)}
                  className="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 md:grid-cols-[1fr_1fr_1fr_auto]"
                  onSubmit={(event) => void updateSequence(sequence, event)}
                >
                  <Input
                    aria-label={`Sequence Prefix ${String(sequence.doc_type)} ${String(sequence.branch_code || "")}`.trim()}
                    defaultValue={String(sequence.prefix)}
                    name="prefix"
                  />
                  <Input
                    aria-label={`Sequence Next Number ${String(sequence.doc_type)} ${String(sequence.branch_code || "")}`.trim()}
                    defaultValue={String(sequence.next_number)}
                    name="next_number"
                    type="number"
                  />
                  <label className="flex items-center gap-3 rounded-md border border-border bg-white px-4 py-3 text-sm">
                    <Checkbox
                      aria-label={`Sequence Locked ${String(sequence.doc_type)} ${String(sequence.branch_code || "")}`.trim()}
                      defaultChecked={Boolean(sequence.is_locked)}
                      name="is_locked"
                    />
                    Locked
                  </label>
                  <Button type="submit">Save</Button>
                  <p className="text-sm text-muted-foreground md:col-span-4">
                    Next example: <span className="font-medium text-black">{String(sequence.example_number || "-")}</span>
                  </p>
                </form>
              ))}
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent value="marketplace" className="space-y-6">
          <MarketplaceConsole
            branches={branches}
            defaultBranchId={String(branches[0]?.id || "")}
            providers={providers}
          />
          <SectionCard title="Marketplace Inbox" description="Provider-ready order inbox stored in PostgreSQL.">
            <DataTable
              columns={[
                { key: "provider_name", label: "Provider" },
                { key: "external_order_id", label: "Order" },
                { key: "customer_name", label: "Customer" },
                { key: "status", label: "Status" },
                { key: "order_total", label: "Total", type: "currency" }
              ]}
              rows={marketplaceOrders}
            />
          </SectionCard>
        </TabsContent>

        <TabsContent value="audit" className="space-y-6">
          <SectionCard title="Audit Filters" description="Filter persisted audit logs by entity, action, and Bangkok date range.">
            <form action="/settings" className="grid gap-4 md:grid-cols-2 xl:grid-cols-6" method="GET">
              <input name="tab" type="hidden" value="audit" />
              <Select defaultValue={auditFilters.branch_id} name="branch_id">
                <option value="">All branches</option>
                {branches.map((branch) => (
                  <option key={String(branch.id)} value={String(branch.id)}>
                    {String(branch.name)}
                  </option>
                ))}
              </Select>
              <Input defaultValue={auditFilters.entity_type} name="entity_type" placeholder="Entity type" />
              <Input defaultValue={auditFilters.action} name="action" placeholder="Action" />
              <Input defaultValue={auditFilters.date_from} name="date_from" type="date" />
              <Input defaultValue={auditFilters.date_to} name="date_to" type="date" />
              <Button type="submit">Apply Filters</Button>
            </form>
          </SectionCard>
          <SectionCard title="Recent Audit Logs" description="All mutating actions are persisted centrally with actor, entity, and timestamp.">
            <AuditTimeline items={auditLogs} />
          </SectionCard>
        </TabsContent>
      </Tabs>
    </div>
  );
}
