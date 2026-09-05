import type { ReactNode } from "react";

/**
 * A tax invoice is ink on paper. Whatever theme the operator has the app set to,
 * the document itself stays black on white — both on screen, so what they see is
 * what will come out of the printer, and in the PDF a customer receives.
 */
export default function PrintLayout({ children }: { children: ReactNode }) {
  return <div className="theme-print-sheet">{children}</div>;
}
