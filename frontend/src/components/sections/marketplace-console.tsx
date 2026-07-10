"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { SectionCard } from "@/components/sections/common";
import { Button, Checkbox, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;

export function MarketplaceConsole({
  providers,
  branches,
  defaultBranchId
}: {
  providers: Option[];
  branches: Option[];
  defaultBranchId?: string;
}) {
  const router = useRouter();
  const [providerId, setProviderId] = useState(String(providers[0]?.id || ""));
  const [branchId, setBranchId] = useState(defaultBranchId || "");
  const [connectionName, setConnectionName] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [webhookEnabled, setWebhookEnabled] = useState(true);
  const [message, setMessage] = useState("");

  async function save() {
    try {
      await proxyClient("/marketplace/connections", {
        method: "POST",
        body: JSON.stringify({
          provider_id: providerId,
          branch_id: branchId,
          connection_name: connectionName,
          credentials: { api_key: apiKey },
          settings: { webhook_enabled: webhookEnabled },
          status: "configured"
        })
      });
      setMessage("Marketplace connection saved");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Save failed");
    }
  }

  return (
    <SectionCard title="Marketplace Connection" description="v1 stores credentials, settings, and branch mapping without a live connector.">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Select value={providerId} onChange={(event) => setProviderId(event.target.value)}>
          {providers.map((provider) => (
            <option key={String(provider.id)} value={String(provider.id)}>
              {String(provider.name)}
            </option>
          ))}
        </Select>
        <Select value={branchId} onChange={(event) => setBranchId(event.target.value)}>
          <option value="">Select branch</option>
          {branches.map((branch) => (
            <option key={String(branch.id)} value={String(branch.id)}>
              {String(branch.name)}
            </option>
          ))}
        </Select>
        <Input placeholder="Connection name" value={connectionName} onChange={(event) => setConnectionName(event.target.value)} />
        <Input placeholder="API key" value={apiKey} onChange={(event) => setApiKey(event.target.value)} />
      </div>
      <label className="mt-4 flex items-center gap-3 rounded-md border border-black/10 bg-[#f5f5f4] px-4 py-3 text-sm">
        <Checkbox checked={webhookEnabled} onChange={(event) => setWebhookEnabled(event.target.checked)} />
        Enable webhook inbox
      </label>
      <div className="mt-4 flex items-center gap-3">
        <Button onClick={save} type="button">
          Save Connection
        </Button>
        {message ? <p className="text-sm text-black/70">{message}</p> : null}
      </div>
    </SectionCard>
  );
}
