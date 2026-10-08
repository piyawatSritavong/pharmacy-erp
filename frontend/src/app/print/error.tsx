"use client";

/** A document that failed to load — plain ink on paper, like the documents themselves. */
export default function PrintError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <main className="mx-auto max-w-xl px-8 py-16 text-center text-black">
      <h1 className="text-xl font-bold">โหลดเอกสารไม่สำเร็จ</h1>
      <p className="mt-2 text-sm text-neutral-600">ลองใหม่อีกครั้ง หากยังไม่ได้ให้แจ้งผู้ดูแลระบบ</p>
      <button className="mt-6 rounded-md border border-black px-4 py-2 text-sm print:hidden" onClick={reset} type="button">ลองใหม่</button>
    </main>
  );
}
