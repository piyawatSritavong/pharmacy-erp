"use client";

import { CreditCard, FilePlus2, Search } from "lucide-react";
import { startTransition, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import { DataTable, SectionCard } from "@/components/sections/common";
import { DocumentComposer } from "@/components/sections/document-composer";
import { ConvertQuotationButton, DeleteDocumentButton } from "@/components/sections/quotation-actions";
import { Button, Dialog, DialogContent, DialogHeader, Input, Pagination, Select, Tabs, TabsContent, TabsList, TabsTrigger, usePagedRows } from "@/components/ui/primitives";
import { Field } from "@/components/ui/field";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type DocumentKind = "quotation" | "invoice";

export function SalesDocumentsConsole({
  branches,
  products,
  quotations,
  invoices,
  governmentMode,
  canUseGhost = false,
  showFullTimestamp = false
}: {
  branches: Option[];
  products: Option[];
  quotations: Option[];
  invoices: Option[];
  governmentMode: boolean;
  canUseGhost?: boolean;
  showFullTimestamp?: boolean;
}) {
  const router = useRouter();
  const [createKind, setCreateKind] = useState<DocumentKind | null>(null);
  const [paymentInvoice, setPaymentInvoice] = useState<Option | null>(null);
  const [paymentType, setPaymentType] = useState<"cash" | "bank_transfer">("cash");
  const [paymentMessage, setPaymentMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const salesBranches = branches.filter((branch) => Boolean(branch.sales_enabled ?? true));
  // One filter bar drives both tabs — the two lists are the same documents at
  // different stages, and filtering them apart would be confusing.
  const [search, setSearch] = useState("");
  const [branchFilter, setBranchFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

  function matches(row: Option, statusKey: "status" | "payment_status") {
    const keyword = search.trim().toLocaleLowerCase("th");
    if (branchFilter && String(row.branch_id || row.branch_name) !== branchFilter) return false;
    if (statusFilter && String(row[statusKey]) !== statusFilter) return false;
    if (
      keyword &&
      ![row.quote_number, row.invoice_number, row.customer_name].some((value) =>
        String(value || "").toLocaleLowerCase("th").includes(keyword)
      )
    ) {
      return false;
    }
    return true;
  }

  const visibleQuotations = useMemo(
    () => quotations.filter((row) => matches(row, "status")),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [branchFilter, quotations, search, statusFilter]
  );
  const visibleInvoices = useMemo(
    () => invoices.filter((row) => matches(row, "payment_status")),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [branchFilter, invoices, search, statusFilter]
  );
  const quotationPage = usePagedRows(visibleQuotations);
  const invoicePage = usePagedRows(visibleInvoices);

  function onFilterChange(apply: () => void) {
    apply();
    quotationPage.resetPage();
    invoicePage.resetPage();
  }

  const filterBar = (
    <div className="mb-4 flex flex-wrap items-end gap-3">
      <Field className="w-full sm:w-64" label="ค้นหา">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-3 h-4 w-4 text-muted-foreground" />
          <Input
            aria-label="ค้นหาเอกสาร"
            className="pl-9"
            onChange={(event) => onFilterChange(() => setSearch(event.target.value))}
            placeholder="เลขที่เอกสาร หรือชื่อลูกค้า"
            value={search}
          />
        </div>
      </Field>
      <Field className="w-52" label="สาขา">
        <Select
          aria-label="กรองเอกสารตามสาขา"
          onChange={(event) => onFilterChange(() => setBranchFilter(event.target.value))}
          value={branchFilter}
        >
          <option value="">ทุกสาขา</option>
          {salesBranches.map((branch) => (
            <option key={String(branch.id)} value={String(branch.id)}>{String(branch.name)}</option>
          ))}
        </Select>
      </Field>
      <Field className="w-44" label="สถานะ">
        <Select
          aria-label="กรองเอกสารตามสถานะ"
          onChange={(event) => onFilterChange(() => setStatusFilter(event.target.value))}
          value={statusFilter}
        >
          <option value="">ทุกสถานะ</option>
          <option value="draft">ฉบับร่าง</option>
          <option value="converted">แปลงเป็นใบขายแล้ว</option>
          <option value="paid">ชำระแล้ว</option>
          <option value="unpaid">ค้างชำระ</option>
        </Select>
      </Field>
    </div>
  );

  async function collectPayment() {
    if (!paymentInvoice) return;
    setBusy(true);
    setPaymentMessage("");
    try {
      const response = await proxyClient<{ message: string }>(`/invoices/${String(paymentInvoice.id)}/pay`, {
        method: "POST",
        body: JSON.stringify({
          payment_type: paymentType,
          notes: governmentMode ? "รับชำระเอกสาร รพ.สต." : "รับชำระจากหน้าการขายและเอกสาร"
        })
      });
      setPaymentMessage(response.message);
      setPaymentInvoice(null);
      startTransition(() => router.refresh());
    } catch (caught) {
      setPaymentMessage(caught instanceof Error ? caught.message : "รับชำระไม่สำเร็จ");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Tabs className="space-y-6" defaultValue="quotation">
        <TabsList>
          <TabsTrigger value="quotation">ใบเสนอราคา</TabsTrigger>
          <TabsTrigger value="invoice">ใบขาย</TabsTrigger>
        </TabsList>
        <TabsContent value="quotation">
          <SectionCard
            title="ใบเสนอราคา"
            description={governmentMode ? "ใบเสนอราคาสำหรับหน่วยงานราชการ" : "ใบเสนอราคาทั่วไปที่ยังไม่แปลงสามารถออกเป็นใบขายได้"}
            actions={<Button onClick={() => setCreateKind("quotation")} type="button"><FilePlus2 className="h-4 w-4" />สร้างใบเสนอราคา</Button>}
          >
            {filterBar}
            <DataTable
              columns={[
                { key: "quote_number", label: "เลขที่เอกสาร" },
                { key: "customer_name", label: "ลูกค้า" },
                { key: "branch_name", label: "สาขา" },
                { key: "status", label: "สถานะ" },
                { key: "total_amount", label: "ยอดรวม", type: "currency" },
                { key: "created_at", label: "วันที่สร้าง", type: showFullTimestamp ? "datetime" : "date" }
              ]}
              rowActions={(row) => (
                <div className="flex justify-end gap-2">
                  {String(row.status) === "draft" ? <ConvertQuotationButton id={String(row.id)} /> : null}
                  <DeleteDocumentButton id={String(row.id)} kind="quotation" />
                </div>
              )}
              emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
              rows={quotationPage.pageRows}
            />
            <Pagination className="mt-4" {...quotationPage.pager} />
          </SectionCard>
        </TabsContent>
        <TabsContent value="invoice">
          <SectionCard
            title="ใบขาย"
            description={governmentMode ? "ใบขายสำหรับหน่วยงานราชการ" : "ใบขายทั่วไปและสถานะการรับชำระ"}
            actions={<Button onClick={() => setCreateKind("invoice")} type="button"><FilePlus2 className="h-4 w-4" />สร้างใบขาย</Button>}
          >
            {filterBar}
            <DataTable
              columns={[
                { key: "invoice_number", label: "เลขที่ใบขาย" },
                { key: "customer_name", label: "ลูกค้า" },
                { key: "branch_name", label: "สาขา" },
                { key: "payment_status", label: "การชำระเงิน" },
                { key: "total_amount", label: "ยอดรวม", type: "currency" },
                { key: "issued_at", label: "วันที่ออก", type: showFullTimestamp ? "datetime" : "date" }
              ]}
              rowActions={(row) => (
                <div className="flex flex-wrap justify-end gap-2">
                  {String(row.payment_status) === "unpaid" ? (
                    <Button onClick={() => { setPaymentInvoice(row); setPaymentMessage(""); }} type="button" variant="secondary">
                      <CreditCard className="h-4 w-4" />รับชำระ
                    </Button>
                  ) : null}
                  <a className="inline-flex rounded-full border px-3 py-2 text-sm font-semibold hover:bg-muted" href={`/print/invoices/${String(row.id)}`} rel="noreferrer" target="_blank">พิมพ์</a>
                  <DeleteDocumentButton id={String(row.id)} kind="invoice" />
                </div>
              )}
              emptyDescription="ลองปรับคำค้นหาหรือตัวกรอง"
              rows={invoicePage.pageRows}
            />
            <Pagination className="mt-4" {...invoicePage.pager} />
          </SectionCard>
        </TabsContent>
      </Tabs>

      <Dialog onOpenChange={(open) => { if (!open) setCreateKind(null); }} open={Boolean(createKind)}>
        <DialogContent className="max-w-6xl">
          <DialogHeader
            title={createKind === "invoice" ? "สร้างใบขาย" : "สร้างใบเสนอราคา"}
            description={governmentMode ? "เอกสารนี้จะถูกจัดเป็นงาน รพ.สต. โดยอัตโนมัติ" : "เอกสารทั่วไป ไม่รวมรายการงานราชการ"}
          />
          {createKind ? (
            <DocumentComposer
              branches={salesBranches}
              canUseGhost={canUseGhost}
              defaultBranchId={String(salesBranches[0]?.id || "")}
              description="ยอด ราคา ภาษี และสต๊อกตรวจสอบโดย backend"
              governmentMode={governmentMode}
              kind={createKind}
              onCreated={() => setCreateKind(null)}
              products={products}
              title={createKind === "invoice" ? "รายละเอียดใบขาย" : "รายละเอียดใบเสนอราคา"}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={(open) => { if (!open) setPaymentInvoice(null); }} open={Boolean(paymentInvoice)}>
        <DialogContent>
          <DialogHeader title={`รับชำระ ${String(paymentInvoice?.invoice_number || "")}`} description="ระบบจะรับชำระเต็มยอดของใบขาย" />
          <div className="space-y-4">
            <Select aria-label="ช่องทางรับชำระ" onChange={(event) => setPaymentType(event.target.value as typeof paymentType)} value={paymentType}>
              <option value="cash">เงินสด</option>
              <option value="bank_transfer">เงินโอน</option>
            </Select>
            {paymentMessage ? <p className="text-sm text-destructive">{paymentMessage}</p> : null}
            <div className="flex justify-end gap-2">
              <Button onClick={() => setPaymentInvoice(null)} type="button" variant="secondary">ยกเลิก</Button>
              <Button disabled={busy} onClick={() => void collectPayment()} type="button">
                {busy ? "กำลังบันทึก..." : "ยืนยันรับชำระ"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
