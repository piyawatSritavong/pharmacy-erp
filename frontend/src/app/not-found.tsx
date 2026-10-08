import Link from "next/link";

import { buttonVariants } from "@/components/ui/button";

export default function NotFoundPage() {
  return (
    <main className="grid min-h-screen place-items-center bg-background p-6">
      <section className="w-full max-w-lg rounded-2xl border bg-card p-10 text-center shadow-card">
        <p className="text-sm font-bold text-primary">404</p>
        <h1 className="mt-3 text-3xl font-bold">ไม่พบหน้าที่ต้องการ</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          หน้านี้ถูกย้าย ถูกลบ หรือคุณไม่มีลิงก์ที่ถูกต้อง
        </p>
        <Link className={buttonVariants({ className: "mt-6" })} href="/">
          กลับหน้าหลัก
        </Link>
      </section>
    </main>
  );
}
