"use client";

import { FormEvent, startTransition, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Globe, Minus, Pencil, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { z } from "zod";

import { DataTable, SectionCard } from "@/components/sections/common";
import { MarketplaceConsole } from "@/components/sections/marketplace-console";
import { Field } from "@/components/ui/field";
import { AutoResizeTextarea, Badge, Button, Checkbox, CheckboxField, Dialog, DialogContent, DialogHeader, EmptyState, Input, Pagination, Select, Switch, Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";
import { cn, generateReadableCode } from "@/lib/utils";
import { rules, useFormErrors } from "@/lib/validation";

const PAGE_SIZE_DEFAULT = 20;

function branchTypeLabel(value: unknown) {
  return String(value) === "main_warehouse" ? "คลังหลัก" : "สาขาหน้าร้าน";
}

/** Compact on/off marker — a coloured dot plus a two-word label, so a status
 *  column never wraps the way the old "เปิดใช้งาน · ..." string did. */
function StatusDot({ on, offLabel = "ปิดใช้งาน", onLabel = "เปิดใช้งาน" }: { on: boolean; offLabel?: string; onLabel?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <span aria-hidden className={cn("h-2 w-2 shrink-0 rounded-full", on ? "bg-success-600" : "bg-muted-foreground/40")} />
      {on ? onLabel : offLabel}
    </span>
  );
}

/** Slices an in-memory list for the shared pager. These two tabs read every
 *  branch/user in one server fetch (there are tens, not thousands), so paging
 *  stays client-side rather than adding query params the API doesn't take. */
function paginate<T>(rows: T[], page: number, pageSize: number) {
  const total = rows.length;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  return {
    rows: rows.slice((safePage - 1) * pageSize, safePage * pageSize),
    pagination: { page: safePage, page_size: pageSize, total, total_pages: totalPages }
  };
}

const branchSchema = z.object({
  code: rules.text("รหัสสาขา", { max: 20 }),
  name: rules.text("ชื่อสาขา", { max: 120 }),
  branch_type: rules.select("ประเภทสถานที่"),
  address: rules.optionalText("ที่อยู่", { max: 200 })
});

type Option = Record<string, unknown>;
/** Create and edit share one dialog per tab — same fields, different verb. */
type EditorState = { mode: "create" | "edit"; record: Option | null } | null;
type DeleteState = {
  kind: "branch" | "user";
  id: string;
  label: string;
  confirmation: string;
  counts: Record<string, number>;
  path: string;
} | null;

export function SettingsConsole({
  branches,
  users,
  roles,
  permissions,
  sequences,
  providers,
  marketplaceOrders,
  currentUserId,
  defaultTab = "branches"
}: {
  branches: Option[];
  users: Option[];
  roles: Option[];
  permissions: Option[];
  sequences: Option[];
  providers: Option[];
  marketplaceOrders: Option[];
  currentUserId: string;
  defaultTab?: string;
}) {
  const router = useRouter();
  const [message, setMessage] = useState("");
  const [deleteState, setDeleteState] = useState<DeleteState>(null);
  const [deleteText, setDeleteText] = useState("");
  const [newBranchCode, setNewBranchCode] = useState("");
  // Bumped after each successful create, forced as the Switch's key below
  // so it remounts to a fresh defaultChecked instead of keeping whatever
  // the user last toggled (form.reset() doesn't reach React-owned state).
  const [branchFormResetKey, setBranchFormResetKey] = useState(0);
  const branchErrors = useFormErrors(branchSchema);
  const [sequenceSearch, setSequenceSearch] = useState("");
  // D11: which role is selected in the "เพิ่มผู้ใช้" form / each user's own
  // update form — drives whether the branch field is disabled (global-scope
  // role) and which branches it offers (POS roles need a sales_enabled
  // branch; other branch-scoped roles like หัวหน้าสาขา can run a warehouse).
  // Role picked inside whichever user dialog is open — drives whether the
  // branch field is enabled and which branches it offers.
  const [createRoleId, setCreateRoleId] = useState("");
  // Branch tab: filter bar + client-side pager.
  const [branchSearch, setBranchSearch] = useState("");
  const [branchTypeFilter, setBranchTypeFilter] = useState("");
  const [branchStatusFilter, setBranchStatusFilter] = useState("");
  const [branchPage, setBranchPage] = useState(1);
  const [branchPageSize, setBranchPageSize] = useState(PAGE_SIZE_DEFAULT);
  const [branchEditor, setBranchEditor] = useState<EditorState>(null);
  // User tab: the same three controls, filtering on role instead of type.
  const [userSearch, setUserSearch] = useState("");
  const [userRoleFilter, setUserRoleFilter] = useState("");
  const [userStatusFilter, setUserStatusFilter] = useState("");
  const [userPage, setUserPage] = useState(1);
  const [userPageSize, setUserPageSize] = useState(PAGE_SIZE_DEFAULT);
  const [userEditor, setUserEditor] = useState<EditorState>(null);

  function roleFor(roleId: string) {
    return roles.find((role) => String(role.id) === roleId);
  }

  function branchOptionsFor(roleId: string) {
    const role = roleFor(roleId);
    if (role && String(role.portal) !== "pos") {
      return branches;
    }
    return branches.filter((branch) => Boolean(branch.sales_enabled));
  }

  useEffect(() => {
    setNewBranchCode(generateReadableCode("BR"));
  }, []);

  function branchNameById(branchId: unknown) {
    if (!branchId) return "";
    return String(branches.find((branch) => String(branch.id) === String(branchId))?.name || "");
  }

  const filteredBranches = useMemo(() => {
    const term = branchSearch.trim().toLocaleLowerCase("th");
    return branches.filter((branch) => {
      if (term && ![branch.code, branch.name].some((value) => String(value || "").toLocaleLowerCase("th").includes(term))) return false;
      if (branchTypeFilter && String(branch.branch_type || "branch") !== branchTypeFilter) return false;
      if (branchStatusFilter === "active" && !branch.active) return false;
      if (branchStatusFilter === "inactive" && branch.active) return false;
      return true;
    });
  }, [branchSearch, branchStatusFilter, branchTypeFilter, branches]);
  const { rows: pagedBranches, pagination: branchPagination } = paginate(filteredBranches, branchPage, branchPageSize);

  const filteredUsers = useMemo(() => {
    const term = userSearch.trim().toLocaleLowerCase("th");
    return users.filter((user) => {
      if (term && ![user.name, user.email].some((value) => String(value || "").toLocaleLowerCase("th").includes(term))) return false;
      if (userRoleFilter && String(user.role_key) !== userRoleFilter) return false;
      if (userStatusFilter === "active" && !user.active) return false;
      if (userStatusFilter === "inactive" && user.active) return false;
      return true;
    });
  }, [userRoleFilter, userSearch, userStatusFilter, users]);
  const { rows: pagedUsers, pagination: userPagination } = paginate(filteredUsers, userPage, userPageSize);

  function openBranchCreate() {
    branchErrors.setErrors({});
    setNewBranchCode(generateReadableCode("BR"));
    setBranchFormResetKey((current) => current + 1);
    setBranchEditor({ mode: "create", record: null });
  }

  function openBranchEdit(branch: Option) {
    branchErrors.setErrors({});
    setBranchEditor({ mode: "edit", record: branch });
  }

  function openUserCreate() {
    setCreateRoleId("");
    setUserEditor({ mode: "create", record: null });
  }

  function openUserEdit(user: Option) {
    setCreateRoleId(String(roles.find((role) => String(role.role_key) === String(user.role_key))?.id || ""));
    setUserEditor({ mode: "edit", record: user });
  }

  // D11: permission catalog grouped by its dot-prefix (dashboard.*,
  // products.*, ...) for the role-permission checkbox editor below.
  const permissionGroups = useMemo(() => {
    const groups = new Map<string, Option[]>();
    for (const permission of permissions) {
      const prefix = String(permission.permission_key).split(".")[0];
      const list = groups.get(prefix) || [];
      list.push(permission);
      groups.set(prefix, list);
    }
    return Array.from(groups.entries());
  }, [permissions]);

  // D12: grouped by branch instead of one long list of every branch ×
  // document-type combination — plus a filter, since both branches and
  // document types only grow over time.
  const sequenceGroups = useMemo(() => {
    const term = sequenceSearch.trim().toLocaleLowerCase("th");
    const matches = (sequence: Option) =>
      !term ||
      [sequence.branch_name, sequence.branch_code, sequence.prefix, sequence.doc_type]
        .some((value) => String(value || "").toLocaleLowerCase("th").includes(term));
    const groups = new Map<string, { branchName: string; branchCode: string; items: Option[] }>();
    for (const sequence of sequences) {
      if (!matches(sequence)) continue;
      const key = String(sequence.branch_id);
      const group = groups.get(key) || { branchName: String(sequence.branch_name), branchCode: String(sequence.branch_code), items: [] };
      group.items.push(sequence);
      groups.set(key, group);
    }
    return Array.from(groups.entries());
  }, [sequenceSearch, sequences]);

  async function submitJSON(path: string, method: "POST" | "PUT", body: Record<string, unknown>) {
    try {
      const response = await proxyClient<{ message?: string }>(path, {
        method,
        body: JSON.stringify(body)
      });
      setMessage(response.message || "บันทึกแล้ว");
      startTransition(() => router.refresh());
      return true;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "บันทึกไม่สำเร็จ");
      return false;
    }
  }

  async function createBranch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const fields = {
      code: String(data.get("code") || ""),
      name: String(data.get("name") || ""),
      branch_type: String(data.get("branch_type") || ""),
      address: String(data.get("address") || "")
    };
    const result = branchErrors.validate(fields);
    if (!result.success) return;
    if (await submitJSON("/branches", "POST", {
      ...fields,
      parent_branch_id: data.get("parent_branch_id") || undefined,
      active: data.get("active") === "on",
      online_sales_enabled: data.get("online_sales_enabled") === "on"
    })) {
      form.reset();
      setNewBranchCode(generateReadableCode("BR"));
      setBranchFormResetKey((current) => current + 1);
      branchErrors.setErrors({});
      setBranchEditor(null);
    }
  }

  async function updateBranch(branchId: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const saved = await submitJSON(`/branches/${branchId}`, "PUT", {
      code: data.get("code"),
      name: data.get("name"),
      address: data.get("address"),
      branch_type: data.get("branch_type"),
      parent_branch_id: data.get("parent_branch_id") || undefined,
      active: data.get("active") === "on",
      online_sales_enabled: data.get("online_sales_enabled") === "on"
    });
    if (saved) setBranchEditor(null);
  }

  async function createUser(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    if (await submitJSON("/users", "POST", {
      full_name: data.get("full_name"),
      email: data.get("email"),
      password: data.get("password"),
      role_id: data.get("role_id"),
      branch_id: data.get("branch_id") || undefined,
      active: data.get("active") === "on"
    })) {
      form.reset();
      setCreateRoleId("");
      setUserEditor(null);
    }
  }

  async function updateUser(userId: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const saved = await submitJSON(`/users/${userId}`, "PUT", {
      full_name: data.get("full_name"),
      email: data.get("email"),
      role_id: data.get("role_id"),
      branch_id: data.get("branch_id") || undefined,
      active: data.get("active") === "on"
    });
    if (saved) setUserEditor(null);
  }

  async function resetPassword(userId: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    if (await submitJSON(`/users/${userId}/reset-password`, "POST", {
      password: data.get("password")
    })) form.reset();
  }

  async function updateSequence(sequence: Option, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    await submitJSON(
      `/branches/${String(sequence.branch_id)}/sequences/${String(sequence.doc_type)}`,
      "PUT",
      {
        prefix: data.get("prefix"),
        next_number: Number(data.get("next_number")),
        is_locked: data.get("is_locked") === "on"
      }
    );
  }

  // D11: replace a role's whole permission set from the checkbox editor.
  async function savePermissions(roleId: string, event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    await submitJSON(`/roles/${roleId}/permissions`, "PUT", {
      permissions: data.getAll("permissions")
    });
  }

  async function requestDelete(kind: "branch" | "user", item: Option) {
    const id = String(item.id);
    const path = kind === "branch" ? `/branches/${id}` : `/users/${id}`;
    try {
      const impact = await proxyClient<{
        confirmation: string;
        counts: Record<string, number>;
      }>(`${path}/deletion-impact`);
      setDeleteText("");
      setDeleteState({
        kind,
        id,
        label: kind === "branch" ? String(item.name) : String(item.email),
        confirmation: impact.confirmation,
        counts: impact.counts,
        path
      });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ตรวจสอบผลกระทบไม่สำเร็จ");
    }
  }

  async function confirmDelete() {
    if (!deleteState) return;
    try {
      const response = await proxyClient<{ message: string }>(deleteState.path, {
        method: "DELETE",
        body: JSON.stringify({ confirmation: deleteText })
      });
      setDeleteState(null);
      setBranchEditor(null);
      setUserEditor(null);
      setDeleteText("");
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "ลบข้อมูลไม่สำเร็จ");
    }
  }

  return (
    <div className="space-y-4">
      {message ? <p className="rounded-2xl border bg-white px-4 py-3 text-sm shadow-card">{message}</p> : null}
      <Tabs className="space-y-6" defaultValue={defaultTab}>
        <TabsList>
          <TabsTrigger value="branches">สาขา</TabsTrigger>
          <TabsTrigger value="users">ผู้ใช้</TabsTrigger>
          <TabsTrigger value="role-permissions">บทบาทและสิทธิ์</TabsTrigger>
          <TabsTrigger value="sequences">เลขที่เอกสาร</TabsTrigger>
          <TabsTrigger value="marketplace">ตลาดออนไลน์</TabsTrigger>
        </TabsList>

        <TabsContent className="space-y-6" value="branches">
          <SectionCard
            actions={
              <Button aria-label="เพิ่มสาขา" onClick={openBranchCreate} title="เพิ่มสาขา" type="button">
                <Plus className="h-4 w-4" />
              </Button>
            }
            description="การลบสาขาจะลบผู้ใช้ เอกสาร สต๊อก และการโอนที่เกี่ยวข้อง"
            title="รายการสาขา"
          >
            <div className="mb-4 flex flex-wrap items-end gap-3">
              <Field className="w-60" label="ค้นหา">
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                  <Input
                    aria-label="ค้นหาสาขา"
                    className="pl-9"
                    onChange={(event) => { setBranchSearch(event.target.value); setBranchPage(1); }}
                    placeholder="รหัสหรือชื่อสาขา"
                    value={branchSearch}
                  />
                </div>
              </Field>
              <Field className="w-44" label="ประเภทสถานที่">
                <Select
                  aria-label="กรองตามประเภทสถานที่"
                  onChange={(event) => { setBranchTypeFilter(event.target.value); setBranchPage(1); }}
                  value={branchTypeFilter}
                >
                  <option value="">ทุกประเภท</option>
                  <option value="branch">สาขาหน้าร้าน</option>
                  <option value="main_warehouse">คลังหลัก</option>
                </Select>
              </Field>
              <Field className="w-40" label="สถานะ">
                <Select
                  aria-label="กรองตามสถานะสาขา"
                  onChange={(event) => { setBranchStatusFilter(event.target.value); setBranchPage(1); }}
                  value={branchStatusFilter}
                >
                  <option value="">ทุกสถานะ</option>
                  <option value="active">เปิดใช้งาน</option>
                  <option value="inactive">ปิดใช้งาน</option>
                </Select>
              </Field>
            </div>

            <DataTable
              columns={[
                { key: "code", label: "รหัส", className: "whitespace-nowrap font-medium" },
                { key: "name", label: "ชื่อสาขา", className: "whitespace-nowrap" },
                { key: "branch_type", label: "ประเภท", className: "whitespace-nowrap", render: (row) => branchTypeLabel(row.branch_type) },
                {
                  key: "parent_branch_id",
                  label: "คลังหลักต้นสังกัด",
                  className: "whitespace-nowrap",
                  render: (row) => branchNameById(row.parent_branch_id) || <span className="text-muted-foreground">—</span>
                },
                {
                  key: "address",
                  label: "ที่อยู่",
                  className: "max-w-[18rem] truncate",
                  render: (row) => (row.address ? <span title={String(row.address)}>{String(row.address)}</span> : <span className="text-muted-foreground">—</span>)
                },
                {
                  key: "active",
                  label: "สถานะ",
                  className: "whitespace-nowrap",
                  // The old inline label read "เปิดใช้งาน · รับ/กระจายสินค้าเท่านั้น"
                  // and wrapped onto three lines. Split into a state marker plus
                  // a short capability chip, with the full wording on hover.
                  render: (row) => (
                    <span className="flex items-center gap-2" title={Boolean(row.sales_enabled) ? "ขายหน้าร้านได้" : "รับ/กระจายสินค้าเท่านั้น"}>
                      <StatusDot on={Boolean(row.active)} />
                      <Badge className={Boolean(row.sales_enabled) ? "bg-success-50 text-success-800" : "bg-muted text-muted-foreground"}>
                        {Boolean(row.sales_enabled) ? "ขายได้" : "คลัง"}
                      </Badge>
                    </span>
                  )
                },
                {
                  key: "online_sales_enabled",
                  label: "ออนไลน์",
                  className: "whitespace-nowrap text-center",
                  render: (row) =>
                    Boolean(row.online_sales_enabled) ? (
                      <Globe aria-label="เปิดขายออนไลน์" className="mx-auto h-4 w-4 text-success-700" />
                    ) : (
                      <Minus aria-label="ไม่เปิดขายออนไลน์" className="mx-auto h-4 w-4 text-muted-foreground" />
                    )
                }
              ]}
              emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
              rowActions={(row) => (
                <div className="flex justify-end gap-2">
                  <Button aria-label={`แก้ไขสาขา ${String(row.name)}`} onClick={() => openBranchEdit(row)} title="แก้ไข" type="button" variant="secondary">
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button aria-label={`ลบสาขา ${String(row.name)}`} onClick={() => void requestDelete("branch", row)} title="ลบ" type="button" variant="destructive">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              )}
              rows={pagedBranches}
            />

            <Pagination
              className="mt-4"
              onPageChange={setBranchPage}
              onPageSizeChange={(size) => { setBranchPageSize(size); setBranchPage(1); }}
              page={branchPagination.page}
              pageSize={branchPagination.page_size}
              total={branchPagination.total}
              totalPages={branchPagination.total_pages}
            />
          </SectionCard>
        </TabsContent>

        <TabsContent className="space-y-6" value="users">
          <SectionCard
            actions={
              <Button aria-label="เพิ่มผู้ใช้" onClick={openUserCreate} title="เพิ่มผู้ใช้" type="button">
                <Plus className="h-4 w-4" />
              </Button>
            }
            description="การลบถาวรจะลบธุรกรรมที่ผู้ใช้นั้นสร้างและคำนวณยอดสต๊อกใหม่"
            title="รายการผู้ใช้"
          >
            <div className="mb-4 flex flex-wrap items-end gap-3">
              <Field className="w-60" label="ค้นหา">
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
                  <Input
                    aria-label="ค้นหาผู้ใช้"
                    className="pl-9"
                    onChange={(event) => { setUserSearch(event.target.value); setUserPage(1); }}
                    placeholder="ชื่อหรืออีเมล"
                    value={userSearch}
                  />
                </div>
              </Field>
              <Field className="w-52" label="บทบาท">
                <Select
                  aria-label="กรองตามบทบาท"
                  onChange={(event) => { setUserRoleFilter(event.target.value); setUserPage(1); }}
                  value={userRoleFilter}
                >
                  <option value="">ทุกบทบาท</option>
                  {roles.map((role) => (
                    <option key={String(role.id)} value={String(role.role_key)}>{String(role.name)}</option>
                  ))}
                </Select>
              </Field>
              <Field className="w-40" label="สถานะ">
                <Select
                  aria-label="กรองตามสถานะผู้ใช้"
                  onChange={(event) => { setUserStatusFilter(event.target.value); setUserPage(1); }}
                  value={userStatusFilter}
                >
                  <option value="">ทุกสถานะ</option>
                  <option value="active">เปิดใช้งาน</option>
                  <option value="inactive">ปิดใช้งาน</option>
                </Select>
              </Field>
            </div>

            <DataTable
              columns={[
                { key: "name", label: "ชื่อ-นามสกุล", className: "whitespace-nowrap font-medium" },
                { key: "email", label: "อีเมล", className: "whitespace-nowrap" },
                {
                  key: "role_key",
                  label: "บทบาท",
                  className: "whitespace-nowrap",
                  render: (row) => String(roles.find((role) => String(role.role_key) === String(row.role_key))?.name || row.role_key || "—")
                },
                {
                  key: "branch_id",
                  label: "สาขา",
                  className: "whitespace-nowrap",
                  render: (row) => branchNameById(row.branch_id) || <span className="text-muted-foreground">ไม่ผูกสาขา</span>
                },
                {
                  key: "active",
                  label: "สถานะ",
                  className: "whitespace-nowrap",
                  render: (row) => <StatusDot on={Boolean(row.active)} />
                }
              ]}
              emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
              rowActions={(row) => (
                <div className="flex justify-end gap-2">
                  <Button aria-label={`แก้ไขผู้ใช้ ${String(row.email)}`} onClick={() => openUserEdit(row)} title="แก้ไข" type="button" variant="secondary">
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    aria-label={`ลบผู้ใช้ ${String(row.email)}`}
                    disabled={String(row.id) === currentUserId}
                    onClick={() => void requestDelete("user", row)}
                    title={String(row.id) === currentUserId ? "ลบบัญชีที่กำลังใช้งานอยู่ไม่ได้" : "ลบ"}
                    type="button"
                    variant="destructive"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              )}
              rows={pagedUsers}
            />

            <Pagination
              className="mt-4"
              onPageChange={setUserPage}
              onPageSizeChange={(size) => { setUserPageSize(size); setUserPage(1); }}
              page={userPagination.page}
              pageSize={userPagination.page_size}
              total={userPagination.total}
              totalPages={userPagination.total_pages}
            />
          </SectionCard>
        </TabsContent>

        <TabsContent className="space-y-6" value="role-permissions">
          <SectionCard
            description="บทบาทหลักของระบบ (ผู้ดูแลระบบ, พนักงานขายหน้าร้าน) ล็อกไว้แก้ไขไม่ได้ เพื่อไม่ให้ปรับสิทธิ์จนเข้าระบบไม่ได้ — บทบาทอื่นปรับสิทธิ์ได้ตามต้องการ"
            title="บทบาทและสิทธิ์"
          >
            <div className="space-y-3">
              {roles.map((role) => {
                const roleId = String(role.id);
                const isSystem = Boolean(role.is_system);
                const rolePermissionKeys = new Set(
                  Array.isArray(role.permissions) ? role.permissions.map((key) => String(key)) : []
                );
                return (
                  <details className="overflow-hidden rounded-2xl border" key={roleId} open={!isSystem}>
                    <summary className="cursor-pointer bg-surface-warm px-4 py-3 text-sm font-bold hover:bg-muted">
                      {String(role.name)}{" "}
                      <span className="font-normal text-muted-foreground">
                        {isSystem
                          ? "บทบาทหลักของระบบ · แก้ไขสิทธิ์ไม่ได้"
                          : `${rolePermissionKeys.size.toLocaleString("th-TH")} สิทธิ์`}
                      </span>
                    </summary>
                    <div className="p-4">
                      {isSystem ? (
                        <p className="text-sm text-muted-foreground">
                          {String(role.role_key) === "super_admin"
                            ? "เข้าถึงได้ทุกอย่างในระบบยกเว้นหน้าจอขายหน้าร้าน"
                            : "สิทธิ์ผูกกับหน้าจอขายหน้าร้านโดยเฉพาะ"}
                        </p>
                      ) : (
                        <form className="space-y-4" onSubmit={(event) => void savePermissions(roleId, event)}>
                          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                            {permissionGroups.map(([prefix, items]) => (
                              <div className="space-y-2 rounded-xl border bg-white p-3" key={prefix}>
                                <p className="text-xs font-bold uppercase text-muted-foreground">{prefix}</p>
                                {items.map((permission) => (
                                  <label
                                    className="flex items-start gap-2 text-sm"
                                    key={String(permission.permission_key)}
                                    title={String(permission.description)}
                                  >
                                    <Checkbox
                                      className="mt-0.5"
                                      defaultChecked={rolePermissionKeys.has(String(permission.permission_key))}
                                      name="permissions"
                                      value={String(permission.permission_key)}
                                    />
                                    <span>{String(permission.name)}</span>
                                  </label>
                                ))}
                              </div>
                            ))}
                          </div>
                          <Button type="submit">บันทึกสิทธิ์</Button>
                        </form>
                      )}
                    </div>
                  </details>
                );
              })}
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent value="sequences">
          <SectionCard title="เลขที่เอกสาร" description="ปลดล็อกก่อนแก้ prefix หรือเลขถัดไป">
            {/* D12: grouped by branch (collapsible) + filterable, instead of
                every branch × document-type combination stacked in one long
                list — see sequenceGroups above. */}
            <div className="relative mb-4">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input aria-label="ค้นหาสาขาหรือคำนำหน้าเอกสาร" className="pl-9" onChange={(event) => setSequenceSearch(event.target.value)} placeholder="ค้นหาสาขาหรือคำนำหน้าเอกสาร" value={sequenceSearch} />
            </div>
            <div className="space-y-3">
              {sequenceGroups.map(([branchId, group]) => (
                <details className="overflow-hidden rounded-2xl border" key={branchId} open={Boolean(sequenceSearch.trim())}>
                  <summary className="cursor-pointer bg-surface-warm px-4 py-3 text-sm font-bold hover:bg-muted">
                    {group.branchName} <span className="font-normal text-muted-foreground">{group.branchCode} · {group.items.length.toLocaleString("th-TH")} เลขที่เอกสาร</span>
                  </summary>
                  <div className="space-y-3 p-3">
                    {group.items.map((sequence) => (
                      <form className="grid gap-4 rounded-2xl bg-muted p-4 md:grid-cols-[1fr_1fr_1fr_auto]" key={String(sequence.id)} onSubmit={(event) => void updateSequence(sequence, event)}>
                        <Field label="คำนำหน้าเอกสาร (Prefix)" hint={String(sequence.doc_type) === "invoice" ? "ใบขาย" : "ใบเสนอราคา"}>
                          <Input aria-label={`คำนำหน้า ${String(sequence.doc_type)} ${String(sequence.branch_code)}`} defaultValue={String(sequence.prefix)} maxLength={10} name="prefix" required />
                        </Field>
                        <Field label="หมายเลขลำดับถัดไป" hint="ระบบเพิ่มให้อัตโนมัติ">
                          <Input aria-label={`เลขถัดไป ${String(sequence.doc_type)} ${String(sequence.branch_code)}`} defaultValue={String(sequence.next_number)} min="1" name="next_number" required type="number" />
                        </Field>
                        <Field label="สถานะการแก้ไข" hint="ล็อกเพื่อป้องกันเลขเปลี่ยน">
                          <CheckboxField aria-label={`ล็อกเลขที่ ${String(sequence.doc_type)} ${String(sequence.branch_code)}`} defaultChecked={Boolean(sequence.is_locked)} label="ล็อกเลขที่เอกสาร" name="is_locked" />
                        </Field>
                        <Button className="self-end" type="submit">บันทึก</Button>
                        <p className="text-sm text-muted-foreground md:col-span-4">ตัวอย่างเลขถัดไป <strong>{String(sequence.example_number)}</strong></p>
                      </form>
                    ))}
                  </div>
                </details>
              ))}
              {sequenceGroups.length === 0 ? <EmptyState description="ไม่พบสาขาหรือคำนำหน้าเอกสารที่ค้นหา" /> : null}
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent className="space-y-6" value="marketplace">
          <MarketplaceConsole branches={branches} defaultBranchId={String(branches[0]?.id || "")} providers={providers} />
          <SectionCard title="คำสั่งซื้อจากตลาดออนไลน์">
            <DataTable columns={[
              { key: "provider_name", label: "ผู้ให้บริการ" },
              { key: "external_order_id", label: "เลขคำสั่งซื้อ" },
              { key: "customer_name", label: "ลูกค้า" },
              { key: "status", label: "สถานะ" },
              { key: "order_total", label: "ยอดรวม", type: "currency" }
            ]} rows={marketplaceOrders} />
          </SectionCard>
        </TabsContent>

      </Tabs>

      {/* Branch create/edit — one dialog, keyed on the record so switching
          rows remounts the uncontrolled inputs with the new defaults. */}
      <Dialog onOpenChange={(open) => !open && setBranchEditor(null)} open={Boolean(branchEditor)}>
        <DialogContent className="max-w-3xl">
          <DialogHeader
            description={branchEditor?.mode === "create" ? "ระบบจะสร้างชุดเลขที่ใบขายและใบเสนอราคาให้อัตโนมัติ" : "แก้ไขข้อมูลสาขาและสิทธิ์การขาย"}
            title={branchEditor?.mode === "create" ? "เพิ่มสาขา" : `แก้ไขสาขา ${String(branchEditor?.record?.name || "")}`}
          />
          {branchEditor ? (
            <form
              className="grid gap-3 md:grid-cols-2"
              key={String(branchEditor.record?.id || "new")}
              onSubmit={(event) =>
                branchEditor.mode === "create"
                  ? void createBranch(event)
                  : void updateBranch(String(branchEditor.record?.id), event)
              }
            >
              <Field error={branchErrors.errors.code} hint={branchEditor.mode === "create" ? "ระบบสร้างให้ แก้ไขได้" : undefined} label="รหัสสาขา">
                {branchEditor.mode === "create" ? (
                  <div className="flex gap-1.5">
                    <Input
                      name="code"
                      onChange={(event) => { setNewBranchCode(event.target.value.toUpperCase()); branchErrors.clearError("code"); }}
                      placeholder="รหัสสาขา"
                      value={newBranchCode}
                    />
                    <Button
                      aria-label="สุ่มรหัสใหม่"
                      className="shrink-0 px-2.5"
                      onClick={() => setNewBranchCode(generateReadableCode("BR"))}
                      title="สุ่มรหัสใหม่"
                      type="button"
                      variant="secondary"
                    >
                      <RefreshCw className="h-4 w-4" />
                    </Button>
                  </div>
                ) : (
                  <Input defaultValue={String(branchEditor.record?.code || "")} maxLength={20} name="code" onChange={() => branchErrors.clearError("code")} required />
                )}
              </Field>
              <Field error={branchErrors.errors.name} label="ชื่อสาขา">
                <Input defaultValue={String(branchEditor.record?.name || "")} maxLength={120} name="name" onChange={() => branchErrors.clearError("name")} placeholder="เช่น สาขาสุขุมวิท" required />
              </Field>
              <Field error={branchErrors.errors.branch_type} label="ประเภทสถานที่">
                <Select aria-label="ประเภทสถานที่" defaultValue={String(branchEditor.record?.branch_type || "branch")} name="branch_type" onChange={() => branchErrors.clearError("branch_type")}>
                  <option value="branch">สาขาหน้าร้าน</option>
                  <option value="main_warehouse">คลังหลัก</option>
                </Select>
              </Field>
              <Field hint="ไม่บังคับ" label="คลังหลักต้นสังกัด">
                <Select aria-label="คลังหลักต้นสังกัด" defaultValue={String(branchEditor.record?.parent_branch_id || "")} name="parent_branch_id">
                  <option value="">ไม่ระบุคลังหลัก</option>
                  {branches
                    .filter((parent) => String(parent.branch_type) === "main_warehouse" && String(parent.id) !== String(branchEditor.record?.id || ""))
                    .map((parent) => <option key={String(parent.id)} value={String(parent.id)}>{String(parent.name)}</option>)}
                </Select>
              </Field>
              <Field className="md:col-span-2" error={branchErrors.errors.address} hint="ไม่บังคับ" label="ที่อยู่">
                <AutoResizeTextarea defaultValue={String(branchEditor.record?.address || "")} maxLength={200} name="address" onChange={() => branchErrors.clearError("address")} placeholder="ที่อยู่สำหรับเอกสาร" />
              </Field>
              <label className="flex h-11 items-center gap-2 rounded-xl border px-3 text-sm">
                <Switch defaultChecked={branchEditor.mode === "create" ? true : Boolean(branchEditor.record?.active)} key={`active-${branchFormResetKey}`} name="active" />
                เปิดใช้งาน
              </label>
              <label className="flex h-11 items-center gap-2 rounded-xl border px-3 text-sm">
                <Switch defaultChecked={branchEditor.mode === "edit" && Boolean(branchEditor.record?.online_sales_enabled)} key={`online-${branchFormResetKey}`} name="online_sales_enabled" />
                เปิดขายออนไลน์
              </label>
              <div className="flex justify-end gap-2 md:col-span-2">
                <Button onClick={() => setBranchEditor(null)} type="button" variant="secondary">ยกเลิก</Button>
                <Button type="submit">{branchEditor.mode === "create" ? "เพิ่มสาขา" : "บันทึก"}</Button>
              </div>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>

      {/* User create/edit. Editing also carries the reset-password form, which
          posts to its own endpoint and so stays a separate <form>. */}
      <Dialog onOpenChange={(open) => !open && setUserEditor(null)} open={Boolean(userEditor)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader
            description="เลือกบทบาท — ระบบจะบังคับหรือปิดช่องสาขาให้อัตโนมัติตามบทบาทที่เลือก"
            title={userEditor?.mode === "create" ? "เพิ่มผู้ใช้" : `แก้ไขผู้ใช้ ${String(userEditor?.record?.email || "")}`}
          />
          {userEditor ? (
            <div className="space-y-4">
              <form
                className="grid gap-3 md:grid-cols-2"
                key={String(userEditor.record?.id || "new")}
                onSubmit={(event) =>
                  userEditor.mode === "create"
                    ? void createUser(event)
                    : void updateUser(String(userEditor.record?.id), event)
                }
              >
                <Field label="ชื่อ-นามสกุล">
                  <Input defaultValue={String(userEditor.record?.name || "")} name="full_name" placeholder="ชื่อ-นามสกุล" required />
                </Field>
                <Field label="อีเมล">
                  <Input defaultValue={String(userEditor.record?.email || "")} name="email" placeholder="อีเมล" required type="email" />
                </Field>
                {userEditor.mode === "create" ? (
                  <Field label="รหัสผ่าน" hint="อย่างน้อย 8 ตัว">
                    <Input minLength={8} name="password" placeholder="รหัสผ่านอย่างน้อย 8 ตัว" required type="password" />
                  </Field>
                ) : null}
                <Field label="บทบาท">
                  <Select aria-label="บทบาทผู้ใช้" name="role_id" onChange={(event) => setCreateRoleId(event.target.value)} required value={createRoleId}>
                    <option value="">เลือกบทบาท</option>
                    {roles.map((role) => <option key={String(role.id)} value={String(role.id)}>{String(role.name)}</option>)}
                  </Select>
                </Field>
                <Field label="สาขา">
                  <Select aria-label="สาขาผู้ใช้" defaultValue={String(userEditor.record?.branch_id || "")} disabled={roleFor(createRoleId)?.scope === "global"} name="branch_id">
                    <option value="">{roleFor(createRoleId)?.scope === "branch" ? "เลือกสาขา" : "ไม่ผูกสาขา"}</option>
                    {branchOptionsFor(createRoleId).map((branch) => <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>)}
                  </Select>
                </Field>
                <CheckboxField
                  className="self-end"
                  defaultChecked={userEditor.mode === "create" ? true : Boolean(userEditor.record?.active)}
                  label="เปิดใช้งาน"
                  name="active"
                />
                <div className="flex justify-end gap-2 md:col-span-2">
                  <Button onClick={() => setUserEditor(null)} type="button" variant="secondary">ยกเลิก</Button>
                  <Button type="submit">{userEditor.mode === "create" ? "เพิ่มผู้ใช้" : "บันทึกผู้ใช้"}</Button>
                </div>
              </form>
              {userEditor.mode === "edit" ? (
                <form className="flex gap-2 border-t pt-4" onSubmit={(event) => void resetPassword(String(userEditor.record?.id), event)}>
                  <Input aria-label="รหัสผ่านใหม่" minLength={8} name="password" placeholder="รหัสผ่านใหม่" required type="password" />
                  <Button className="shrink-0" type="submit" variant="secondary">ตั้งรหัสผ่านใหม่</Button>
                </form>
              ) : null}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(deleteState)}>
        <DialogContent>
          <DialogHeader title={`ลบ${deleteState?.kind === "branch" ? "สาขา" : "ผู้ใช้"} ${deleteState?.label || ""}`} description="ข้อมูลทั้งหมดด้านล่างจะถูกลบถาวรและไม่สามารถย้อนกลับได้" />
          <div className="space-y-4">
            <div className="rounded-2xl bg-destructive/10 p-4 text-sm">
              {Object.entries(deleteState?.counts || {}).map(([key, count]) => <div className="flex justify-between py-1" key={key}><span>{key}</span><strong>{count}</strong></div>)}
            </div>
            <p className="text-sm">พิมพ์ <strong>{deleteState?.confirmation}</strong> เพื่อยืนยัน</p>
            <Input aria-label="ข้อความยืนยันการลบ" onChange={(event) => setDeleteText(event.target.value)} value={deleteText} />
            <div className="flex justify-end gap-2">
              <Button onClick={() => setDeleteState(null)} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={deleteText !== deleteState?.confirmation} onClick={() => void confirmDelete()} type="button" variant="destructive">ลบถาวร</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
