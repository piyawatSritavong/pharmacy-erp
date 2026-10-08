import Link from "next/link";
import { FileQuestion } from "lucide-react";

import { buttonVariants } from "@/components/ui/button";

/** A record or page that isn't there — kept inside the shell, menu and all. */
export default function AppNotFound() {
  return (
    <div className="grid min-h-[50vh] place-items-center gap-2 p-10 text-center">
      <FileQuestion aria-hidden className="h-9 w-9 text-muted-foreground/60" />
      <p className="text-sm font-semibold">ไม่พบข้อมูลที่ต้องการ</p>
      <p className="max-w-md text-xs text-muted-foreground">รายการนี้อาจถูกลบ หรือคุณไม่มีสิทธิ์เข้าถึง</p>
      <Link className={buttonVariants({ variant: "secondary" })} href="/">กลับหน้าหลัก</Link>
    </div>
  );
}
