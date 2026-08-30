"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";
import { PlugZap } from "lucide-react";

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
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);

  function currentPayload() {
    return {
      provider_id: providerId,
      branch_id: branchId,
      connection_name: connectionName,
      credentials: { api_key: apiKey },
      settings: { webhook_enabled: webhookEnabled },
      status: "configured"
    };
  }

  // D13: verify the connection is configured correctly before saving it.
  async function testConnection() {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await proxyClient<{ success: boolean; message: string }>("/marketplace/test-connection", {
        method: "POST",
        body: JSON.stringify(currentPayload())
      });
      setTestResult(result);
    } catch (caught) {
      setTestResult({ success: false, message: caught instanceof Error ? caught.message : "ทดสอบการเชื่อมต่อไม่สำเร็จ" });
    } finally {
      setTesting(false);
    }
  }

  async function save() {
    try {
      await proxyClient("/marketplace/connections", {
        method: "POST",
        body: JSON.stringify(currentPayload())
      });
      setMessage("บันทึกการเชื่อมต่อตลาดออนไลน์แล้ว");
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "บันทึกการเชื่อมต่อไม่สำเร็จ");
    }
  }

  return (
    <SectionCard title="การเชื่อมต่อตลาดออนไลน์" description="จัดเก็บข้อมูลเชื่อมต่อและสาขาที่รับคำสั่งซื้อ">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Select aria-label="ผู้ให้บริการตลาดออนไลน์" value={providerId} onChange={(event) => setProviderId(event.target.value)}>
          {providers.map((provider) => (
            <option key={String(provider.id)} value={String(provider.id)}>
              {String(provider.name)}
            </option>
          ))}
        </Select>
        <Select aria-label="สาขาสำหรับตลาดออนไลน์" value={branchId} onChange={(event) => setBranchId(event.target.value)}>
          <option value="">เลือกสาขา</option>
          {/* Part B, Rule 3 — คณาเภสัช is the only branch allowed to sell
              online; every other branch still runs normal in-store POS, so
              it's only excluded from *this* picker, not from selling. */}
          {branches.filter((branch) => Boolean(branch.online_sales_enabled)).map((branch) => (
            <option key={String(branch.id)} value={String(branch.id)}>
              {String(branch.name)}
            </option>
          ))}
        </Select>
        <Input placeholder="ชื่อการเชื่อมต่อ" value={connectionName} onChange={(event) => setConnectionName(event.target.value)} />
        <Input placeholder="คีย์ API" value={apiKey} onChange={(event) => setApiKey(event.target.value)} />
      </div>
      <label className="mt-4 flex items-center gap-3 rounded-md border border-border bg-muted/50 px-4 py-3 text-sm">
        <Checkbox checked={webhookEnabled} onChange={(event) => setWebhookEnabled(event.target.checked)} />
        เปิดรับคำสั่งซื้อผ่าน webhook
      </label>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Button onClick={save} type="button">
          บันทึกการเชื่อมต่อ
        </Button>
        <Button disabled={testing || !providerId || !branchId} onClick={() => void testConnection()} type="button" variant="secondary">
          <PlugZap className="h-4 w-4" />
          {testing ? "กำลังทดสอบ..." : "ทดสอบการเชื่อมต่อ"}
        </Button>
        {message ? <p className="text-sm text-muted-foreground">{message}</p> : null}
      </div>
      {testResult ? (
        <p className={`mt-3 rounded-xl px-4 py-3 text-sm ${testResult.success ? "bg-success-50 text-success-800" : "bg-error-50 text-error"}`} role="status">
          {testResult.message}
        </p>
      ) : null}
    </SectionCard>
  );
}
