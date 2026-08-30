import { PageIntro } from "@/components/sections/common";
import { TransferHistoryTable } from "@/components/sections/transfer-history-table";
import { TransfersWorkspace } from "@/components/sections/transfers-workspace";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getStockTransferRequests, getTransfers, requireSession } from "@/services/erp";

export default async function TransfersPage({
  searchParams
}: {
  searchParams?: Promise<{ tab?: string | string[] }>;
}) {
  requirePermission(await requireSession(), ["transfer.approve"]);
  const params = await searchParams;
  const defaultTab = typeof params?.tab === "string" ? params.tab : "transfers";
  const [branches, products, transfers, requests] = await Promise.all([
    getBranches(),
    getProducts(undefined, { page: 1, pageSize: 200 }),
    getTransfers(),
    getStockTransferRequests()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="โอนสินค้า"
        description="โยกย้ายสินค้าระหว่างสาขาและตรวจคำขอเบิกสินค้าจากหน้าร้าน"
      />
      <TransfersWorkspace
        branches={branches.items}
        canUseGhost={false}
        defaultTab={defaultTab}
        historySlot={<TransferHistoryTable branches={branches.items} transfers={transfers.items} />}
        products={products.items}
        requests={requests.items}
        transfers={transfers.items}
      />
    </div>
  );
}
