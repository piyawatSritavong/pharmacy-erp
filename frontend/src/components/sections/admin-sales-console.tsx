"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, MonitorSmartphone, Store } from "lucide-react";
import { toast } from "sonner";

import { PosWorkspace } from "@/components/sections/pos-workspace";
import { SectionCard } from "@/components/sections/common";
import { Field } from "@/components/ui/field";
import { Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type Branch = { id: string; name: string };
type SaleMode = "remote" | "hq_pickup";

function text(value: unknown) {
  return value == null ? "" : String(value);
}

const MODES: Array<{ id: SaleMode; title: string; detail: string; icon: typeof Store }> = [
  {
    id: "remote",
    title: "รีโมตหน้าร้าน",
    detail: "เปิดการขายแทนพนักงานสาขา ลูกค้าสแกนจ่ายที่เครื่อง POS ของสาขานั้น",
    icon: MonitorSmartphone
  },
  {
    id: "hq_pickup",
    title: "ขายผ่านสำนักงานใหญ่",
    detail: "รับชำระที่สำนักงานใหญ่ ลูกค้าไปรับสินค้าที่สาขาที่เลือก",
    icon: Store
  }
];

/**
 * ขายหน้าร้าน (สำนักงานใหญ่) — head office opens a sale in a branch's name.
 * Pick the branch first, then how the customer pays; the till itself is the
 * same POS cart, and the bill it writes is an ordinary bill of that branch.
 */
export function AdminSalesConsole({ branches, operatorName = "" }: { branches: Branch[]; operatorName?: string }) {
  const [branchId, setBranchId] = useState("");
  const [mode, setMode] = useState<SaleMode>("remote");
  const [products, setProducts] = useState<Option[]>([]);
  const [inventory, setInventory] = useState<Option[]>([]);
  const [loading, setLoading] = useState(false);
  const branchName = branches.find((branch) => branch.id === branchId)?.name || "";

  const loadBranchStock = useCallback(async (id: string) => {
    if (!id) {
      setProducts([]);
      setInventory([]);
      return;
    }
    setLoading(true);
    try {
      const query = `?branch_id=${encodeURIComponent(id)}`;
      const [productResponse, inventoryResponse] = await Promise.all([
        proxyClient<{ items: Option[] }>(`/products${query}&active=true&page=1&page_size=18`),
        proxyClient<{ items: Option[] }>(`/inventory${query}&page_size=500`)
      ]);
      setProducts(productResponse.items);
      setInventory(inventoryResponse.items);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "โหลดสินค้าและสต๊อกของสาขาไม่สำเร็จ");
      setProducts([]);
      setInventory([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadBranchStock(branchId);
  }, [branchId, loadBranchStock]);

  return (
    <div className="space-y-6">
      <SectionCard title="เลือกสาขาและวิธีขาย" description="ระบบจะออกบิลในนามสาขาที่เลือกและตัดสต๊อกของสาขานั้น">
        <div className="grid gap-5 lg:grid-cols-[minmax(0,280px)_1fr]">
          <Field label="สาขาที่จะเปิดขาย">
            <Select aria-label="เลือกสาขาที่จะเปิดขาย" onChange={(event) => setBranchId(event.target.value)} value={branchId}>
              <option value="">เลือกสาขา</option>
              {branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}
            </Select>
          </Field>
          <fieldset>
            <legend className="mb-2 text-sm font-medium">วิธีขาย</legend>
            <div className="grid gap-3 sm:grid-cols-2">
              {MODES.map((option) => {
                const Icon = option.icon;
                const active = mode === option.id;
                return (
                  <button
                    aria-pressed={active}
                    className={`flex items-start gap-3 rounded-xl border p-3 text-left transition ${active ? "border-primary bg-primary/5 ring-1 ring-primary" : "hover:border-primary/40 hover:bg-muted/50"}`}
                    key={option.id}
                    onClick={() => setMode(option.id)}
                    type="button"
                  >
                    <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${active ? "text-primary" : "text-muted-foreground"}`} />
                    <span>
                      <span className="block text-sm font-semibold">{option.title}</span>
                      <span className="mt-0.5 block text-xs text-muted-foreground">{option.detail}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </fieldset>
        </div>
      </SectionCard>

      {!branchId ? (
        <SectionCard title="ยังไม่ได้เลือกสาขา" description="เลือกสาขาด้านบนเพื่อเริ่มเปิดการขาย">
          <p className="py-8 text-center text-sm text-muted-foreground">เลือกสาขาที่จะเปิดขาย แล้วรายการสินค้าของสาขานั้นจะแสดงที่นี่</p>
        </SectionCard>
      ) : loading ? (
        <div className="flex items-center justify-center gap-2 py-16 text-muted-foreground"><Loader2 className="h-5 w-5 animate-spin" /> กำลังโหลดสินค้าของ {branchName}</div>
      ) : (
        <div className="rounded-xl border bg-primary/5 px-4 py-2.5 text-sm">
          กำลังเปิดขายในนาม <strong>{branchName}</strong> · {MODES.find((option) => option.id === mode)?.title}
        </div>
      )}

      {branchId && !loading ? (
        <PosWorkspace
          branchId={branchId}
          branchName={branchName}
          endpointBase="/admin/pos"
          inventory={inventory}
          key={`${branchId}:${mode}`}
          products={products}
          // รีโมตหน้าร้าน pushes the cart to the branch's own till, which takes
          // the money; ขายผ่านสำนักงานใหญ่ settles here as before.
          remoteBranchId={mode === "remote" ? branchId : ""}
        />
      ) : null}

      {/* The till's context bar, mirroring the POS footer: whose shop this bill
          is being written in, and who is writing it. Deliberately without the
          POS menu — this page already sits inside the back-office sidebar. */}
      {branchId && !loading ? (
        <footer className="sticky bottom-0 -mx-4 flex items-center gap-4 border-t bg-white/95 px-4 py-3 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-10 lg:px-10">
          <span className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl bg-foreground text-white">
            <Store className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <p className="truncate font-bold">PharmaPOS</p>
            <p className="truncate text-xs text-muted-foreground">
              กำลังขายในนาม {branchName} · {MODES.find((option) => option.id === mode)?.title}
            </p>
          </div>
          <p className="ml-auto shrink-0 text-right text-xs text-muted-foreground">{operatorName}</p>
        </footer>
      ) : null}
    </div>
  );
}
