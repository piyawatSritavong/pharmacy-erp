import Link from "next/link";

export default function NotFoundPage() {
  return (
    <main className="grid min-h-screen place-items-center bg-background p-6">
      <section className="w-full max-w-lg rounded-[2rem] border bg-white p-10 text-center shadow-card">
        <p className="text-sm font-bold text-primary">404</p>
        <h1 className="mt-3 text-3xl font-bold">ไม่พบหน้าที่ต้องการ</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          หน้านี้ถูกย้าย ถูกลบ หรือคุณไม่มีลิงก์ที่ถูกต้อง
        </p>
        <Link className="mt-6 inline-flex rounded-full bg-primary px-5 py-3 text-sm font-bold text-white" href="/">
          กลับหน้าหลัก
        </Link>
      </section>
    </main>
  );
}
