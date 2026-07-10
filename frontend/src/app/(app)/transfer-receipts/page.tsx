import { DataTable, Grid, PageIntro, SectionCard } from "@/components/sections/common";
import { TransferConsole } from "@/components/sections/transfer-console";
import { requireRole } from "@/lib/rbac";
import { getBranches, getProducts, getTransfers, requireSession } from "@/services/erp";

export default async function TransferReceiptsPage() {
  const session = requireRole(await requireSession(), ["branch_pos"]);
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
        eyebrow="Receipt"
        title="Goods Transfer Receipt"
        description="Scan QR code with the camera or enter transfer code manually to receive stock into the current branch."
      />
      <Grid>
        <TransferConsole
          branches={branches.items}
          defaultBranchId={session.user.branch_id}
          mode="receipt"
          products={products.items}
          transfers={inboundTransfers}
        />
        <SectionCard title="Inbound Queue" description="Transfers waiting to be received into this branch">
          <DataTable
            columns={[
              { key: "transfer_code", label: "Transfer Code" },
              { key: "source_branch_name", label: "Source" },
              { key: "status", label: "Status" },
              { key: "requested_at", label: "Requested", type: "datetime" }
            ]}
            rows={inboundTransfers}
          />
        </SectionCard>
      </Grid>
    </div>
  );
}
