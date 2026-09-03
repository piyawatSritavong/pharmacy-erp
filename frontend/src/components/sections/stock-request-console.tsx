"use client";

import { CheckCircle2, Clock3, PackagePlus, Plus, Search, Trash2, XCircle } from "lucide-react";
import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { DataTable, SectionCard, statusLabel } from "@/components/sections/common";
import { ProductSearchPicker } from "@/components/sections/product-search-picker";
import { Badge, Button, Dialog, DialogContent, DialogHeader, Input, Pagination, Select, usePagedRows } from "@/components/ui/primitives";
import { Field } from "@/components/ui/field";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type RequisitionLine = { key: string; productId: string; quantity: string };

function newLine(): RequisitionLine {
  return { key: Math.random().toString(36).slice(2), productId: "", quantity: "1" };
}

export function StockRequestConsole({
  products,
  requests,
  branches = [],
  mode,
  canUseGhost = false
}: {
  products: Option[];
  requests: Option[];
  branches?: Option[];
  mode: "pos" | "admin";
  canUseGhost?: boolean;
}) {
  const router = useRouter();
  // The requisition is a table of lines now, so a single trip to the counter
  // can ask for everything the shelf is short of at once.
  const [lines, setLines] = useState<RequisitionLine[]>([newLine()]);
  const [destBranchId, setDestBranchId] = useState("");
  const [sourceBranches, setSourceBranches] = useState<Record<string, string>>({});
  const [reviewBuckets, setReviewBuckets] = useState<Record<string, string>>({});
  const [busyId, setBusyId] = useState("");
  const [message, setMessage] = useState("");
  const [requestOpen, setRequestOpen] = useState(false);
  const [historySearch, setHistorySearch] = useState("");
  const [historyBranch, setHistoryBranch] = useState("");
  const [historyStatus, setHistoryStatus] = useState("");

  const activeProducts = useMemo(() => products.filter((product) => Boolean(product.active)), [products]);
  const sellingBranches = useMemo(
    () => branches.filter((branch) => Boolean(branch.active ?? true) && Boolean(branch.sales_enabled ?? true) && String(branch.branch_type) !== "main_warehouse"),
    [branches]
  );

  function updateLine(key: string, patch: Partial<RequisitionLine>) {
    setLines((current) => current.map((line) => (line.key === key ? { ...line, ...patch } : line)));
  }
  // Picking a product that another line already asks for collapses the two:
  // its quantity is added onto the existing line and this line drops away, so a
  // requisition never lists the same product twice.
  function selectProduct(key: string, productId: string) {
    setLines((current) => {
      if (!productId) return current.map((line) => (line.key === key ? { ...line, productId: "" } : line));
      const twin = current.find((line) => line.key !== key && line.productId === productId);
      if (!twin) return current.map((line) => (line.key === key ? { ...line, productId } : line));
      const addQty = Math.max(1, Number(current.find((line) => line.key === key)?.quantity) || 1);
      const merged = current
        .map((line) => (line.key === twin.key ? { ...line, quantity: String((Number(line.quantity) || 0) + addQty) } : line))
        .filter((line) => line.key !== key);
      return merged.length > 0 ? merged : [newLine()];
    });
  }
  function removeLine(key: string) {
    setLines((current) => (current.length > 1 ? current.filter((line) => line.key !== key) : current));
  }
  function resetDialog() {
    setLines([newLine()]);
    setDestBranchId("");
  }

  async function submitRequisition() {
    const valid = lines.filter((line) => line.productId && Number(line.quantity) > 0);
    if (valid.length === 0) {
      setMessage("เพิ่มสินค้าอย่างน้อยหนึ่งรายการ");
      return;
    }
    if (mode === "admin" && !destBranchId) {
      setMessage("เลือกสาขาที่ขอเบิก");
      return;
    }
    setBusyId("create");
    let created = 0;
    const failures: string[] = [];
    // One request per line: each product is reviewed and sourced on its own.
    for (const line of valid) {
      try {
        await proxyClient<{ message: string }>("/stock-transfer-requests", {
          method: "POST",
          body: JSON.stringify({
            product_id: line.productId,
            quantity: Number(line.quantity),
            ...(mode === "admin" ? { destination_branch_id: destBranchId } : {})
          })
        });
        created += 1;
      } catch (caught) {
        const name = String(activeProducts.find((product) => String(product.id) === line.productId)?.name || "สินค้า");
        failures.push(`${name}: ${caught instanceof Error ? caught.message : "ไม่สำเร็จ"}`);
      }
    }
    setBusyId("");
    if (created > 0) {
      setRequestOpen(false);
      resetDialog();
      startTransition(() => router.refresh());
    }
    setMessage(
      failures.length === 0
        ? `สร้างใบเบิก ${created.toLocaleString("th-TH")} รายการแล้ว`
        : `สร้างสำเร็จ ${created} รายการ · ไม่สำเร็จ ${failures.length}: ${failures.slice(0, 3).join(" | ")}`
    );
  }

  async function review(request: Option, decision: "approve" | "reject") {
    const id = String(request.id);
    setBusyId(id);
    try {
      const response = await proxyClient<{ message: string }>(`/stock-transfer-requests/${id}/review`, {
        method: "POST",
        body: JSON.stringify({
          decision,
          source_branch_id: sourceBranches[id] || "",
          stock_bucket: reviewBuckets[id] || "real"
        })
      });
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ตรวจสอบคำขอไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
  }

  const pending = requests.filter((request) => String(request.status) === "pending");
  const reviewed = requests.filter((request) => String(request.status) !== "pending");
  const historyBranches = useMemo(
    () => Array.from(new Set(reviewed.map((row) => String(row.destination_branch_name || "")).filter(Boolean))).sort(),
    [reviewed]
  );
  const visibleHistory = useMemo(() => {
    const keyword = historySearch.trim().toLocaleLowerCase("th");
    return reviewed.filter((row) => {
      if (historyBranch && String(row.destination_branch_name) !== historyBranch) return false;
      if (historyStatus && String(row.status) !== historyStatus) return false;
      if (keyword && ![row.product_name, row.transfer_code, row.source_branch_name].some((value) => String(value || "").toLocaleLowerCase("th").includes(keyword))) return false;
      return true;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [historyBranch, historySearch, historyStatus, reviewed]);
  const history = usePagedRows(visibleHistory);

  const requisitionDialog = (
    <Dialog onOpenChange={(open) => { setRequestOpen(open); if (!open) resetDialog(); }} open={requestOpen}>
      <DialogContent className="max-w-2xl">
        <DialogHeader
          title="สร้างใบเบิกสินค้า"
          description={mode === "admin" ? "เลือกสาขาที่ขอเบิก แล้วเพิ่มรายการสินค้าและจำนวน" : "เพิ่มรายการสินค้าและจำนวนที่ต้องการ ผู้ดูแลจะเลือกสาขาต้นทางและประเภทสต๊อกให้"}
        />
        {mode === "admin" ? (
          <Field className="mb-4" label="สาขาที่ขอเบิก (ปลายทาง)">
            <Select aria-label="สาขาที่ขอเบิก" onChange={(event) => setDestBranchId(event.target.value)} value={destBranchId}>
              <option value="">เลือกสาขา</option>
              {sellingBranches.map((branch) => <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>)}
            </Select>
          </Field>
        ) : null}
        <div className="overflow-hidden rounded-xl border">
          <table className="w-full text-sm">
            <thead className="bg-muted text-xs text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-left font-medium">สินค้า</th>
                <th className="w-32 px-3 py-2 text-left font-medium">จำนวน</th>
                <th className="w-12 px-3 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y">
              {lines.map((line) => (
                <tr key={line.key}>
                  <td className="px-3 py-2">
                    <ProductSearchPicker
                      ariaLabel="เลือกสินค้าที่ต้องการเบิก"
                      initialOptions={activeProducts}
                      onChange={(productId) => selectProduct(line.key, productId)}
                      value={line.productId}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <Input aria-label="จำนวนที่ต้องการ" min="1" onChange={(event) => updateLine(line.key, { quantity: event.target.value })} type="number" value={line.quantity} />
                  </td>
                  <td className="px-3 py-2 text-center">
                    <button aria-label="ลบรายการ" className="rounded p-1.5 text-muted-foreground transition hover:bg-red-50 hover:text-red-600 disabled:opacity-30" disabled={lines.length <= 1} onClick={() => removeLine(line.key)} type="button">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <Button className="mt-3" onClick={() => setLines((current) => [...current, newLine()])} type="button" variant="secondary">
          <Plus className="h-4 w-4" />เพิ่มรายการ
        </Button>
        <div className="mt-5 flex justify-end gap-2">
          <Button onClick={() => { setRequestOpen(false); resetDialog(); }} type="button" variant="ghost">ยกเลิก</Button>
          <Button disabled={busyId === "create"} onClick={() => void submitRequisition()} type="button">
            <PackagePlus className="h-4 w-4" />{busyId === "create" ? "กำลังส่ง..." : "ส่งใบเบิก"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );

  if (mode === "pos") {
    return (
      <div className="space-y-6">
        <SectionCard
          title="ใบเบิกสินค้าของสาขา"
          description="ขอเติมสต๊อกจากผู้ดูแล และติดตามสถานะใบโอนที่สร้างให้"
          actions={<Button onClick={() => setRequestOpen(true)} type="button"><PackagePlus className="h-4 w-4" />สร้างใบเบิกสินค้า</Button>}
        >
          <DataTable
            columns={[
              { key: "product_name", label: "สินค้า" },
              { key: "requested_quantity", label: "จำนวนที่ขอ" },
              { key: "status", label: "สถานะคำขอ" },
              { key: "created_at", label: "วันที่ส่ง", type: "datetime" }
            ]}
            rows={requests}
          />
          {message ? <p className="mt-4 rounded-xl bg-surface-warm p-3 text-sm">{message}</p> : null}
        </SectionCard>
        {requisitionDialog}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SectionCard
        title={`คำขอสินค้ารอตรวจสอบ (${pending.length.toLocaleString("th-TH")})`}
        description="เลือกสาขาต้นทางและสต๊อกที่จะส่ง ระบบจะสร้างใบโอนตามจำนวนที่สาขาขอ"
        actions={<Button onClick={() => setRequestOpen(true)} type="button"><PackagePlus className="h-4 w-4" />สร้างใบเบิกแทนสาขา</Button>}
      >
        <div className="space-y-4">
          {pending.map((request) => {
            const id = String(request.id);
            const destinationId = String(request.destination_branch_id);
            return (
              <article className="rounded-2xl border bg-white p-4" key={id}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h3 className="font-bold">{String(request.product_name)} · {String(request.sku)}</h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      ปลายทาง {String(request.destination_branch_name)} · ผู้ขอ {String(request.requested_by_name)} · จำนวน {Number(request.requested_quantity).toLocaleString("th-TH")}
                    </p>
                  </div>
                  <Badge><Clock3 className="mr-1 h-3 w-3" />{statusLabel(request.status)}</Badge>
                </div>
                <div className={`mt-4 grid gap-3 md:grid-cols-2 ${canUseGhost ? "xl:grid-cols-[minmax(0,1fr)_190px_auto_auto]" : "xl:grid-cols-[minmax(0,1fr)_auto_auto]"}`}>
                  <Select aria-label={`สาขาต้นทาง ${id}`} onChange={(event) => setSourceBranches((current) => ({ ...current, [id]: event.target.value }))} value={sourceBranches[id] || ""}>
                    <option value="">เลือกสาขาต้นทาง</option>
                    {branches.filter((branch) => String(branch.id) !== destinationId).map((branch) => (
                      <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
                    ))}
                  </Select>
                  {/* Only the superadmin sources from Ghost Stock; for everyone
                      else the answer is always real, so there is nothing to pick. */}
                  {canUseGhost ? (
                    <Select aria-label={`ประเภทสต๊อก ${id}`} onChange={(event) => setReviewBuckets((current) => ({ ...current, [id]: event.target.value }))} value={reviewBuckets[id] || "real"}>
                      <option value="real">สต๊อกจริง</option>
                      <option value="ghost">สต๊อกผี</option>
                    </Select>
                  ) : null}
                  <Button disabled={busyId === id || !sourceBranches[id]} onClick={() => void review(request, "approve")} type="button"><CheckCircle2 className="h-4 w-4" />สร้างใบโอน</Button>
                  <Button disabled={busyId === id} onClick={() => void review(request, "reject")} type="button" variant="secondary"><XCircle className="h-4 w-4" />ปฏิเสธ</Button>
                </div>
              </article>
            );
          })}
          {pending.length === 0 ? <p className="rounded-2xl border border-dashed p-8 text-center text-sm text-muted-foreground">ไม่มีคำขอที่รอตรวจสอบ</p> : null}
        </div>
        {message ? <p className="mt-4 rounded-xl bg-surface-warm p-3 text-sm">{message}</p> : null}
      </SectionCard>

      <SectionCard title="ประวัติคำขอ" description="คำขอที่สร้างใบโอนหรือปฏิเสธแล้ว">
        <div className="mb-4 flex flex-wrap items-end gap-3">
          <Field className="w-64" label="ค้นหา">
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
              <Input aria-label="ค้นหาประวัติคำขอ" className="pl-9" onChange={(event) => { setHistorySearch(event.target.value); history.resetPage(); }} placeholder="ชื่อสินค้า, เลขใบโอน, สาขาต้นทาง" value={historySearch} />
            </div>
          </Field>
          <Field className="w-52" label="สาขาปลายทาง">
            <Select aria-label="กรองประวัติคำขอตามสาขาปลายทาง" onChange={(event) => { setHistoryBranch(event.target.value); history.resetPage(); }} value={historyBranch}>
              <option value="">ทุกสาขา</option>
              {historyBranches.map((name) => <option key={name} value={name}>{name}</option>)}
            </Select>
          </Field>
          <Field className="w-44" label="สถานะคำขอ">
            <Select aria-label="กรองประวัติคำขอตามสถานะ" onChange={(event) => { setHistoryStatus(event.target.value); history.resetPage(); }} value={historyStatus}>
              <option value="">ทุกสถานะ</option>
              <option value="approved">อนุมัติแล้ว</option>
              <option value="rejected">ไม่อนุมัติ</option>
            </Select>
          </Field>
        </div>
        <DataTable
          columns={[
            { key: "destination_branch_name", label: "ปลายทาง" },
            { key: "product_name", label: "สินค้า" },
            { key: "requested_quantity", label: "จำนวน" },
            { key: "source_branch_name", label: "ต้นทาง" },
            // The stock bucket is superadmin-only detail; for everyone else a
            // requisition is always filled from real stock.
            ...(canUseGhost ? [{ key: "approved_stock_bucket", label: "สต๊อก" }] : []),
            { key: "transfer_code", label: "เลขใบโอน" },
            { key: "transfer_status", label: "สถานะใบโอน" },
            { key: "status", label: "สถานะคำขอ" },
            { key: "reviewed_at", label: "วันที่ตรวจสอบ", type: "datetime" }
          ]}
          emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
          rows={history.pageRows}
        />
        <Pagination className="mt-4" {...history.pager} />
      </SectionCard>
      {requisitionDialog}
    </div>
  );
}
