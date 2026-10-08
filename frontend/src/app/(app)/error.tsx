"use client";

import { useRouter } from "next/navigation";
import { startTransition, useEffect } from "react";

import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/ui/state-block";

/**
 * A page whose data could not be loaded. It renders inside the shell, so the
 * menu still works; "ลองใหม่" re-runs the page's server fetches.
 */
export default function AppError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const router = useRouter();
  useEffect(() => {
    console.error(error);
  }, [error]);
  return (
    <ErrorState
      action={
        <Button
          onClick={() => startTransition(() => {
            router.refresh();
            reset();
          })}
          variant="secondary"
        >
          ลองใหม่
        </Button>
      }
      className="min-h-[50vh]"
      description={`ระบบหลังบ้านไม่ตอบสนองหรือเกิดข้อผิดพลาดระหว่างโหลด ลองใหม่อีกครั้ง หากยังไม่ได้ให้แจ้งผู้ดูแลระบบ${error.digest ? ` (รหัสอ้างอิง ${error.digest})` : ""}`}
      title="โหลดหน้านี้ไม่สำเร็จ"
    />
  );
}
