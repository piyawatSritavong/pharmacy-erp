import type { ReactNode } from "react";
import { Check, Sparkles } from "lucide-react";

import { PageIntro } from "@/components/sections/common";
import { ProSubscribeButton } from "@/components/sections/pro-subscribe-button";

/** A neutral skeleton to sit behind the blur, so a locked page still reads as a
 *  real screen without running its real (and now pointless) queries. */
export function LockedPreview({ title, description }: { title: string; description: string }) {
  return (
    <div className="space-y-6">
      <PageIntro description={description} title={title} />
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <div className="h-24 rounded-2xl border bg-muted/40" key={index} />
        ))}
      </div>
      <div className="rounded-2xl border bg-card p-6">
        <div className="h-6 w-48 rounded bg-muted" />
        <div className="mt-4 space-y-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <div className="h-10 rounded-lg bg-muted/50" key={index} />
          ))}
        </div>
      </div>
    </div>
  );
}

const BENEFITS = [
  "เอกสารราชการ รพ.สต. และใบเสนอราคา/ใบขายเต็มรูปแบบ",
  "ส่งออกรายงาน อย. และเชื่อมต่อตลาดออนไลน์",
  "ผู้ใช้ไม่จำกัด พร้อมสิทธิ์ขั้นสูงและการสำรองข้อมูล",
  "ทีมดูแลลูกค้าตลอดการใช้งาน"
];

/**
 * A locked feature: the real page renders behind, blurred and inert, with an
 * upgrade card centred over it. The sidebar sits outside this wrapper, so the
 * user can still move to a menu they do have.
 */
export function ProGate({ feature, children }: { feature: string; children: ReactNode }) {
  return (
    <div className="relative min-h-[70vh]">
      <div aria-hidden className="pointer-events-none select-none blur-[6px]">
        {children}
      </div>
      <div className="absolute inset-0 flex items-start justify-center bg-background/50 px-4 pt-16 sm:pt-24">
        <div className="w-full max-w-md rounded-3xl border bg-card p-8 text-center shadow-2xl">
          <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl bg-primary/10 text-primary">
            <Sparkles className="h-7 w-7" />
          </span>
          <p className="mt-5 text-xs font-semibold uppercase tracking-wide text-primary">PharmaPOS Pro</p>
          <h2 className="mt-1 text-2xl font-bold tracking-tight">{feature} อยู่ในแพ็กเกจ Pro</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            อัปเกรดเพื่อปลดล็อก{feature} พร้อมความสามารถขั้นสูงทั้งหมด แล้วใช้งานได้ทันที
          </p>
          <ul className="mt-6 space-y-2.5 text-left">
            {BENEFITS.map((benefit) => (
              <li className="flex items-start gap-2.5 text-sm" key={benefit}>
                <Check className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                <span>{benefit}</span>
              </li>
            ))}
          </ul>
          <div className="mt-7 space-y-3">
            <ProSubscribeButton />
            <p className="text-xs text-muted-foreground">เริ่มต้น ฿990/เดือน · ยกเลิกได้ทุกเมื่อ · ทดลองใช้ฟรี 14 วัน</p>
          </div>
        </div>
      </div>
    </div>
  );
}
