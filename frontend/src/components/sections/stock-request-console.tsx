"use client";

import { CheckCircle2, Clock3, PackagePlus, Search, XCircle } from "lucide-react";
import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { DataTable, SectionCard, statusLabel } from "@/components/sections/common";
import { Badge, Button, Dialog, DialogContent, DialogHeader, Input, Pagination, Select, usePagedRows } from "@/components/ui/primitives";
import { Field } from "@/components/ui/field";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

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
  const [productId, setProductId] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [sourceBranches, setSourceBranches] = useState<Record<string, string>>({});
  const [reviewBuckets, setReviewBuckets] = useState<Record<string, string>>({});
  const [busyId, setBusyId] = useState("");
  const [message, setMessage] = useState("");
  const [requestOpen, setRequestOpen] = useState(false);
  // ประวัติคำขอ filters — the pending queue above stays unfiltered, since it is
  // a to-do list you work through rather than an archive you search.
  const [historySearch, setHistorySearch] = useState("");
  const [historyBranch, setHistoryBranch] = useState("");
  const [historyStatus, setHistoryStatus] = useState("");

  async function createRequest() {
    setBusyId("create");
    try {
      const response = await proxyClient<{ message: string }>("/stock-transfer-requests", {
        method: "POST",
        body: JSON.stringify({ product_id: productId, quantity: Number(quantity) })
      });
      setMessage(response.message);
      setProductId("");
      setQuantity("1");
      setRequestOpen(false);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "ส่งคำขอโอนสินค้าไม่สำเร็จ");
    } finally {
      setBusyId("");
    }
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

  // Destination branches come from the reviewed rows themselves, so the filter
  // never offers a branch with no history behind it.
  const historyBranches = useMemo(
    () => Array.from(new Set(reviewed.map((row) => String(row.destination_branch_name || "")).filter(Boolean))).sort(),
    [reviewed]
  );
  const visibleHistory = useMemo(() => {
    const keyword = historySearch.trim().toLocaleLowerCase("th");
    return reviewed.filter((row) => {
      if (historyBranch && String(row.destination_branch_name) !== historyBranch) return false;
      if (historyStatus && String(row.status) !== historyStatus) return false;
      if (
        keyword &&
        ![row.product_name, row.transfer_code, row.source_branch_name].some((value) =>
          String(value || "").toLocaleLowerCase("th").includes(keyword)
        )
      ) {
        return false;
      }
      return true;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [historyBranch, historySearch, historyStatus, reviewed]);
  const history = usePagedRows(visibleHistory);

  if (mode === "pos") {
    return (
      <div className="space-y-6">
        <SectionCard
          title="สถานะคำขอสินค้า"
          description="ติดตามคำขอและใบโอนที่ผู้ดูแลสร้างให้"
          actions={
            <Button onClick={() => setRequestOpen(true)} type="button">
              <PackagePlus className="h-4 w-4" />
              สร้างใบเบิกสินค้า
            </Button>
          }
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

        <Dialog onOpenChange={setRequestOpen} open={requestOpen}>
          <DialogContent>
            <DialogHeader
              title="สร้างใบเบิกสินค้า"
              description="ระบุสินค้าและจำนวนที่ต้องการ ผู้ดูแลจะเลือกสาขาต้นทางและประเภทสต๊อกให้"
            />
            <div className="grid gap-4">
              <label className="space-y-2">
                <span className="text-sm font-medium">สินค้า</span>
                <Select aria-label="สินค้าที่ต้องการเบิก" onChange={(event) => setProductId(event.target.value)} value={productId}>
                  <option value="">เลือกสินค้า</option>
                  {products.filter((product) => Boolean(product.active)).map((product) => (
                    <option key={String(product.id)} value={String(product.id)}>
                      {String(product.name)} · {String(product.sku)}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="space-y-2">
                <span className="text-sm font-medium">จำนวนที่ต้องการ</span>
                <Input aria-label="จำนวนสินค้าที่ต้องการ" min="1" onChange={(event) => setQuantity(event.target.value)} type="number" value={quantity} />
              </label>
            </div>
            <div className="mt-5 flex justify-end gap-2">
              <Button onClick={() => setRequestOpen(false)} type="button" variant="ghost">ยกเลิก</Button>
              <Button disabled={!productId || Number(quantity) <= 0 || busyId === "create"} onClick={() => void createRequest()} type="button">
                <PackagePlus className="h-4 w-4" />
                {busyId === "create" ? "กำลังส่ง..." : "ส่งคำขอ"}
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SectionCard
        title={`คำขอสินค้ารอตรวจสอบ (${pending.length.toLocaleString("th-TH")})`}
        description="เลือกสาขาต้นทางและสต๊อกที่จะส่ง ระบบจะสร้างใบโอนตามจำนวนที่สาขาขอ"
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
                <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_190px_auto_auto]">
                  <Select
                    aria-label={`สาขาต้นทาง ${id}`}
                    onChange={(event) => setSourceBranches((current) => ({ ...current, [id]: event.target.value }))}
                    value={sourceBranches[id] || ""}
                  >
                    <option value="">เลือกสาขาต้นทาง</option>
                    {branches.filter((branch) => String(branch.id) !== destinationId).map((branch) => (
                      <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
                    ))}
                  </Select>
                  <Select
                    aria-label={`ประเภทสต๊อก ${id}`}
                    onChange={(event) => setReviewBuckets((current) => ({ ...current, [id]: event.target.value }))}
                    value={reviewBuckets[id] || "real"}
                  >
                    <option value="real">สต๊อกจริง</option>
                    {canUseGhost ? <option value="ghost">สต๊อกผี</option> : null}
                  </Select>
                  <Button disabled={busyId === id || !sourceBranches[id]} onClick={() => void review(request, "approve")} type="button">
                    <CheckCircle2 className="h-4 w-4" />สร้างใบโอน
                  </Button>
                  <Button disabled={busyId === id} onClick={() => void review(request, "reject")} type="button" variant="secondary">
                    <XCircle className="h-4 w-4" />ปฏิเสธ
                  </Button>
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
              <Input
                aria-label="ค้นหาประวัติคำขอ"
                className="pl-9"
                onChange={(event) => { setHistorySearch(event.target.value); history.resetPage(); }}
                placeholder="ชื่อสินค้า, เลขใบโอน, สาขาต้นทาง"
                value={historySearch}
              />
            </div>
          </Field>
          <Field className="w-52" label="สาขาปลายทาง">
            <Select
              aria-label="กรองประวัติคำขอตามสาขาปลายทาง"
              onChange={(event) => { setHistoryBranch(event.target.value); history.resetPage(); }}
              value={historyBranch}
            >
              <option value="">ทุกสาขา</option>
              {historyBranches.map((name) => <option key={name} value={name}>{name}</option>)}
            </Select>
          </Field>
          <Field className="w-44" label="สถานะคำขอ">
            <Select
              aria-label="กรองประวัติคำขอตามสถานะ"
              onChange={(event) => { setHistoryStatus(event.target.value); history.resetPage(); }}
              value={historyStatus}
            >
              <option value="">ทุกสถานะ</option>
              {/* Labels mirror statusLabel() in common.tsx, so the filter and
                  the สถานะคำขอ column never disagree on wording. */}
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
            { key: "approved_stock_bucket", label: "สต๊อก" },
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
    </div>
  );
}
