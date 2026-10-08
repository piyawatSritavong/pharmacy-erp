/** An invoice number that doesn't exist, or that this account can't see. */
export default function PrintNotFound() {
  return (
    <main className="mx-auto max-w-xl px-8 py-16 text-center text-black">
      <h1 className="text-xl font-bold">ไม่พบเอกสาร</h1>
      <p className="mt-2 text-sm text-neutral-600">เอกสารนี้อาจถูกลบ หรือลิงก์ไม่ถูกต้อง</p>
    </main>
  );
}
