import type { ReactNode } from "react";

/**
 * Every printed document shares this one layout. A receipt, a tax invoice and
 * a purchase order differ only in their words: the title, who the document is
 * for, the item lines and the totals. Layout changes are made here, once, and
 * every document follows; a document never builds its own page.
 *
 * Top to bottom: the issuer and the title with the document number; the
 * other party beside the document's details; the numbered item table; notes
 * beside the totals; signature lines.
 */

export type DocumentFact = { label: string; value: ReactNode };

export type DocumentColumn = {
  key: string;
  label: string;
  /** A second line under the heading, e.g. "รวม VAT". */
  hint?: string;
  align?: "left" | "right";
};

export type DocumentTotal = { label: string; value: ReactNode; strong?: boolean };

export type DocumentNote = { label: string; body: ReactNode };

export type BusinessDocumentProps = {
  title: string;
  /** A second title line, e.g. the English name or the tax-invoice kind. */
  subtitle?: string;
  number: string;
  issuer: { name: string; lines: string[] };
  party: { label: string; name: string; lines: string[] };
  facts: DocumentFact[];
  columns: DocumentColumn[];
  rows: Array<Record<string, ReactNode>>;
  emptyText?: string;
  totals: DocumentTotal[];
  notes?: DocumentNote[];
  signatures?: string[];
  /** A warning printed above everything, e.g. a cancelled document. */
  banner?: ReactNode;
  /** Screen-only controls such as the print button; never printed. */
  actions?: ReactNode;
};

export function BusinessDocument({
  title,
  subtitle,
  number,
  issuer,
  party,
  facts,
  columns,
  rows,
  emptyText = "เอกสารนี้ไม่มีรายการ",
  totals,
  notes = [],
  signatures = [],
  banner,
  actions,
}: BusinessDocumentProps) {
  return (
    <article className="bg-white text-[13px] leading-relaxed text-black">
      {banner ? <div className="mb-5 rounded-md border-2 border-black px-4 py-3">{banner}</div> : null}

      <header className="flex items-start justify-between gap-6 border-b-2 border-black pb-4">
        <div className="min-w-0">
          <p className="text-lg font-bold">{issuer.name}</p>
          {issuer.lines.filter(Boolean).map((line) => (
            <p className="text-neutral-700" key={line}>{line}</p>
          ))}
        </div>
        <div className="shrink-0 text-right">
          <h1 className="text-2xl font-bold">{title}</h1>
          {subtitle ? <p className="text-neutral-600">{subtitle}</p> : null}
          <p className="mt-1 font-semibold">เลขที่ {number}</p>
          {actions ? <div className="mt-2 print:hidden">{actions}</div> : null}
        </div>
      </header>

      <section className="mt-4 grid grid-cols-2 gap-6">
        <div className="min-w-0">
          <p className="font-bold">{party.label}</p>
          <p className="mt-1 font-semibold">{party.name || "-"}</p>
          {party.lines.filter(Boolean).map((line) => (
            <p className="text-neutral-700" key={line}>{line}</p>
          ))}
        </div>
        <dl className="grid content-start gap-1">
          {facts.map((fact) => (
            <div className="flex justify-between gap-4" key={fact.label}>
              <dt className="text-neutral-600">{fact.label}</dt>
              <dd className="text-right">{fact.value}</dd>
            </div>
          ))}
        </dl>
      </section>

      <table className="mt-5 w-full border-collapse">
        <thead>
          <tr className="border-y-2 border-black text-left">
            <th className="w-8 py-2 pr-2">#</th>
            {columns.map((column) => (
              <th className={`py-2 pr-2 last:pr-0 ${column.align === "right" ? "text-right" : ""}`} key={column.key}>
                {column.label}
                {column.hint ? <span className="block text-[11px] font-normal text-neutral-600">{column.hint}</span> : null}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td className="py-5 text-center text-neutral-600" colSpan={columns.length + 1}>{emptyText}</td>
            </tr>
          ) : null}
          {rows.map((row, index) => (
            <tr className="border-b border-neutral-300 align-top" key={index}>
              <td className="py-2 pr-2">{index + 1}</td>
              {columns.map((column) => (
                <td className={`py-2 pr-2 last:pr-0 ${column.align === "right" ? "text-right tabular-nums" : ""}`} key={column.key}>
                  {row[column.key] ?? "-"}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>

      <section className="mt-5 grid grid-cols-[1fr_minmax(0,18rem)] gap-6">
        <div className="grid content-start gap-3">
          {notes.map((note) => (
            <div key={note.label}>
              <p className="font-bold">{note.label}</p>
              <div className="whitespace-pre-wrap">{note.body}</div>
            </div>
          ))}
        </div>
        <div className="grid content-start gap-1">
          {totals.map((total) => (
            <p
              className={`flex justify-between gap-4 tabular-nums ${total.strong ? "mt-1 border-t-2 border-black pt-2 text-base font-bold" : ""}`}
              key={total.label}
            >
              <span>{total.label}</span>
              <span>{total.value}</span>
            </p>
          ))}
        </div>
      </section>

      {signatures.length ? (
        <footer className="mt-14 grid gap-10 text-center" style={{ gridTemplateColumns: `repeat(${signatures.length}, minmax(0, 1fr))` }}>
          {signatures.map((label) => (
            <div key={label}>
              <p className="border-t border-black pt-2">{label}</p>
              <p className="mt-6 border-t border-black pt-2">วันที่</p>
            </div>
          ))}
        </footer>
      ) : null}
    </article>
  );
}
