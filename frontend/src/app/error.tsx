"use client";

import { useRouter } from "next/navigation";
import { startTransition, useEffect } from "react";

import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/ui/state-block";

/**
 * Failures above a page — chiefly the back-office layout's session check when
 * the API is down. Saying so beats the old behaviour of sending the user to
 * /login, where signing in again could only fail the same way.
 */
export default function RootError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const router = useRouter();
  useEffect(() => {
    console.error(error);
  }, [error]);
  return (
    <main className="grid min-h-screen place-items-center bg-background p-6">
      <ErrorState
        action={
          <Button
            onClick={() => startTransition(() => {
              router.refresh();
              reset();
            })}
          >
            ลองใหม่
          </Button>
        }
        className="w-full max-w-lg rounded-2xl border bg-card"
        description={`ระบบหลังบ้านไม่ตอบสนอง กรุณาลองใหม่อีกครั้งในอีกสักครู่${error.digest ? ` (รหัสอ้างอิง ${error.digest})` : ""}`}
        title="เชื่อมต่อระบบไม่สำเร็จ"
      />
    </main>
  );
}
