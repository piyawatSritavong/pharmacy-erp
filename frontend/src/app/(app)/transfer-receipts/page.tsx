import { PageIntro } from "@/components/sections/common";
import { TransferConsole } from "@/components/sections/transfer-console";
import { requirePermission } from "@/lib/rbac";
import { getBranches, getProducts, getTransfers, requireSession } from "@/services/erp";

export default async function TransferReceiptsPage() {
  const session = requirePermission(await requireSession(), ["transfer.receive"]);
  const [branches, products, transfers] = await Promise.all([
    getBranches(),
    getProducts(session.user.branch_id),
    getTransfers()
  ]);

  const inboundTransfers = transfers.items.filter(
    (item) =>
      String(item.destination_branch_id || "") === String(session.user.branch_id || "") &&
      String(item.status || "") === "in_transit"
  );

  return (
    <div className="space-y-6">
      <PageIntro
        title="รับโอนสินค้า"
        description="ตรวจรายการที่ผู้ดูแลส่งมา เปรียบเทียบจำนวนจริง และยืนยันรับเข้าสาขา"
      />
      <TransferConsole
        branches={branches.items}
        defaultBranchId={session.user.branch_id}
        mode="receipt"
        products={products.items}
        transfers={inboundTransfers}
      />
    </div>
  );
}
