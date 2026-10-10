import type { ReactNode } from "react";
import { LockKeyhole } from "lucide-react";

import { PageIntro } from "@/components/sections/common";
import { PRO_GATE_ENABLED } from "@/lib/pro-access";

/** A neutral skeleton to sit behind the blur, so a locked page still reads as a
 *  real screen without running its real (and now pointless) queries. */
export function LockedPreview({ title, description }: { title: string; description: string }) {
  return (
    <div className="space-y-6">
      <PageIntro description={description} title={title} />
      <div className="rounded-2xl border bg-card p-6">
        <p className="text-sm text-muted-foreground">งานส่วนนี้อยู่ในแผนพัฒนาลำดับถัดไป ยังไม่พร้อมทำรายการ</p>
      </div>
    </div>
  );
}

/**
 * A feature that isn't switched on for this shop: the real page renders
 * behind, blurred and inert, with a notice centred over it. The sidebar sits
 * outside this wrapper, so the user can still move to a menu they do have.
 *
 * The copy says only what is true. It used to quote a monthly price and a free
 * trial and offer a "subscribe" button whose toast promised a call back — but
 * billing was never wired up and nothing was sent anywhere.
 */
export function ProGate({ feature, children }: { feature: string; children: ReactNode }) {
  if (!PRO_GATE_ENABLED) return <>{children}</>;
  return (
    <div className="relative min-h-[70vh]">
      <div aria-hidden className="pointer-events-none select-none blur-[6px]">
        {children}
      </div>
      <div className="absolute inset-0 flex items-start justify-center bg-background/50 px-4 pt-16 sm:pt-24">
        <div className="w-full max-w-md rounded-2xl border bg-card p-6 text-center shadow-2xl sm:p-8">
          <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl bg-primary/10 text-primary">
            <LockKeyhole className="h-7 w-7" />
          </span>
          <h2 className="mt-5 text-xl font-semibold tracking-tight sm:text-2xl">{feature} ยังไม่เปิดใช้งาน</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            ฟีเจอร์นี้ยังไม่เปิดในระบบของร้าน หากต้องการใช้งาน ติดต่อผู้ดูแลระบบ
          </p>
        </div>
      </div>
    </div>
  );
}
