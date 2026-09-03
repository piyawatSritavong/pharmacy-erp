import { PageIntro } from "@/components/sections/common";
import { TransferHistoryTable } from "@/components/sections/transfer-history-table";
import { TransfersWorkspace } from "@/components/sections/transfers-workspace";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getTransfers, requireSession } from "@/services/erp";

export default async function TransfersPage() {
  const session = await requireSession();
  requirePermission(session, ["transfer.approve"]);
  // Only the superadmin deals in Ghost Stock. For everyone else a transfer is
  // always real stock, so they get no bucket to choose and no bucket column.
  const canUseGhost = session.user.role_key === "super_admin";
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
        canUseGhost={canUseGhost}
        historySlot={<TransferHistoryTable branches={branches.items} canUseGhost={canUseGhost} transfers={transfers.items} />}
        products={products.items}
        transfers={transfers.items}
      />
    </div>
  );
}
