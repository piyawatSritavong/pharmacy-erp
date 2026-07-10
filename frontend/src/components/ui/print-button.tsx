"use client";

export function PrintButton() {
  return (
    <button
      className="rounded-md bg-black px-4 py-2 text-sm text-white print:hidden"
      onClick={() => window.print()}
      type="button"
    >
      Print
    </button>
  );
}
