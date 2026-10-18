import type { ReactNode } from "react";
import { LockKeyhole } from "lucide-react";

import { PageIntro } from "@/components/sections/common";
import { isProGated } from "@/lib/pro-access";

/** A neutral skeleton to sit behind the blur, so a locked page still reads as a
 *  real screen without running its real (and now pointless) queries. */
export function LockedPreview({ title, description, notice = false }: { title: string; description: string; notice?: boolean }) {
  return (
    <div className="space-y-6">
      <PageIntro description={description} title={title} />
      {notice ? (
        <div className="rounded-2xl border bg-card p-6">
          <p className="text-sm text-muted-foreground">งานส่วนนี้อยู่ในแผนพัฒนาลำดับถัดไป ยังไม่พร้อมทำรายการ</p>
        </div>
      ) : (
        <>
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
        </>
      )}
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
export function ProGate({ feature, href, children }: { feature: string; href?: string; children: ReactNode }) {
  if (!isProGated(href)) return <>{children}</>;
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
          <p className="mt-5 text-xs font-semibold uppercase tracking-wide text-primary">PharmaPOS Pro</p>
          <h2 className="mt-1 text-xl font-semibold tracking-tight sm:text-2xl">{feature} อยู่ในแพ็กเกจ Pro</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            ฟีเจอร์นี้เปิดใช้ได้เฉพาะแพ็กเกจ Pro หากต้องการใช้งาน ติดต่อผู้ดูแลระบบ
          </p>
        </div>
      </div>
    </div>
  );
}
