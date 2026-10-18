"use client";

import { useEffect, useState } from "react";

import { SectionCard } from "@/components/sections/common";
import { Button, CheckboxField, FeedbackNotice, LoadingState } from "@/components/ui/primitives";
import type { Feedback } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type SalesSettings = { prices_include_vat: boolean; vat_rate: number };

/** Shop-wide pricing: whether the prices the shop types already include VAT. */
export function SalesSettingsCard() {
  const [settings, setSettings] = useState<SalesSettings | null>(null);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);

  useEffect(() => {
    void proxyClient<SalesSettings>("/sales-settings")
      .then(setSettings)
      .catch((error) => setFeedback({ tone: "error", text: error instanceof Error ? error.message : "โหลดการตั้งค่าไม่สำเร็จ" }));
  }, []);

  async function save() {
    if (!settings) return;
    setBusy(true);
    setFeedback(null);
    try {
      await proxyClient("/sales-settings", { method: "PUT", body: JSON.stringify(settings) });
      setFeedback({ tone: "success", text: "บันทึกการตั้งค่าการขายแล้ว มีผลกับบิลถัดไปทันที" });
    } catch (error) {
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : "บันทึกไม่สำเร็จ" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <SectionCard
      description="ใช้กับทุกสาขา ราคาในรายการสินค้า ราคาตามระดับ ราคาส่ง และส่วนลดที่พนักงานกรอก จะอ่านตามการตั้งค่านี้"
      title="ราคาขายและภาษีมูลค่าเพิ่ม"
    >
      {!settings ? (
        feedback ? <FeedbackNotice feedback={feedback} /> : <LoadingState compact label="กำลังโหลด" />
      ) : (
        <div className="space-y-4">
          <CheckboxField
            checked={settings.prices_include_vat}
            label={`ราคาขายรวม VAT ${settings.vat_rate}% แล้ว (ลูกค้าจ่ายเท่าราคาที่ตั้ง ระบบแยก VAT ออกให้)`}
            onChange={(event) => setSettings({ ...settings, prices_include_vat: event.target.checked })}
          />
          <p className="text-sm text-muted-foreground">
            {settings.prices_include_vat
              ? `ตัวอย่าง: สินค้าราคา 107 บาท ลูกค้าจ่าย 107 บาท ใบเสร็จแสดงมูลค่าก่อนภาษี 100 บาท + VAT 7 บาท`
              : `ตัวอย่าง: สินค้าราคา 100 บาท ลูกค้าจ่าย 107 บาท (บวก VAT ${settings.vat_rate}% เพิ่ม)`}
          </p>
          <FeedbackNotice feedback={feedback} />
          <div className="flex justify-end">
            <Button loading={busy} onClick={() => void save()} type="button">บันทึก</Button>
          </div>
        </div>
      )}
    </SectionCard>
  );
}
