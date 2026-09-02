import { PageIntro } from "@/components/sections/common";
import { TransferHistoryTable } from "@/components/sections/transfer-history-table";
import { TransfersWorkspace } from "@/components/sections/transfers-workspace";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getTransfers, requireSession } from "@/services/erp";

export default async function TransfersPage() {
  requirePermission(await requireSession(), ["transfer.approve"]);
  const [branches, products, transfers] = await Promise.all([
    getBranches(),
    getProducts(undefined, { page: 1, pageSize: 200 }),
    getTransfers()
  ]);

  return (
    <div className="space-y-6">
      <PageIntro
        title="โอนสินค้า"
        description="โยกย้ายสินค้าระหว่างสาขา และดูประวัติการโอน"
      />
      <TransfersWorkspace
        branches={branches.items}
        canUseGhost={false}
        historySlot={<TransferHistoryTable branches={branches.items} transfers={transfers.items} />}
        products={products.items}
        transfers={transfers.items}
      />
    </div>
  );
}
