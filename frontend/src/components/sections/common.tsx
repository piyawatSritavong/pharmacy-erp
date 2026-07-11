import type { ReactNode } from "react";

import {
  Badge,
  Card,
  CardBody,
  CardHeader,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/primitives";
import { buildQRCodeSVG } from "@/lib/qr";
import { cn, currency, dateTime } from "@/lib/utils";

export function PageIntro({
  eyebrow,
  title,
  description
}: {
  eyebrow: string;
  title: string;
  description: string;
}) {
  return (
    <div className="mb-6 space-y-1.5">
      <Badge className="bg-accent/10 text-accent">{eyebrow}</Badge>
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p>
    </div>
  );
}

export function MetricGrid({
  items
}: {
  items: Array<{ key: string; label: string; value: string | number }>;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
      {items.map((item) => (
        <Card key={item.key}>
          <CardBody className="space-y-1.5 p-5">
            <p className="text-xs font-medium text-muted-foreground">{item.label}</p>
            <p className="text-2xl font-semibold tracking-tight tabular-nums">
              {typeof item.value === "number" ? item.value.toLocaleString("th-TH") : item.value}
            </p>
          </CardBody>
        </Card>
      ))}
    </div>
  );
}

export function SectionCard({
  title,
  description,
  actions,
  children,
  className
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardHeader title={title} description={description} actions={actions} />
      <CardBody>{children}</CardBody>
    </Card>
  );
}

export function DataTable({
  columns,
  rows,
  rowActions
}: {
  columns: Array<{ key: string; label: string; type?: "currency" | "datetime" | "default" }>;
  rows: Array<Record<string, unknown>>;
  rowActions?: (row: Record<string, unknown>) => ReactNode;
}) {
  return (
    <TableContainer>
      <Table>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {columns.map((column) => (
              <TableHead key={column.key}>
                {column.label}
              </TableHead>
            ))}
            {rowActions ? <TableHead className="text-right">Actions</TableHead> : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row, rowIndex) => (
            <TableRow key={String(row.id || rowIndex)} className="text-sm text-foreground">
              {columns.map((column) => (
                <TableCell key={column.key}>
                  {renderCell(row[column.key], column.type)}
                </TableCell>
              ))}
              {rowActions ? <TableCell className="text-right">{rowActions(row)}</TableCell> : null}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

function renderCell(value: unknown, type?: "currency" | "datetime" | "default") {
  if (value === null || value === undefined || value === "") {
    return <span className="text-muted-foreground">-</span>;
  }
  if (type === "currency" && typeof value === "number") {
    return currency(value);
  }
  if (type === "datetime" && typeof value === "string") {
    return dateTime(value);
  }
  if (typeof value === "boolean") {
    return value ? "Yes" : "No";
  }
  return String(value);
}

export function InvoiceSummary({ summary }: { summary?: Record<string, unknown> }) {
  if (!summary) {
    return null;
  }

  return (
    <div className="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 sm:grid-cols-4">
      {[
        { key: "subtotal", label: "Subtotal" },
        { key: "tax_rate", label: "VAT %" },
        { key: "tax_amount", label: "VAT" },
        { key: "total_amount", label: "Total" }
      ].map((item) => (
        <div key={item.key}>
          <p className="text-xs font-medium text-muted-foreground">{item.label}</p>
          <p className="mt-2 text-lg font-semibold text-black">
            {typeof summary[item.key] === "number"
              ? item.key === "tax_rate"
                ? `${summary[item.key]}%`
                : currency(Number(summary[item.key]))
              : "-"}
          </p>
        </div>
      ))}
    </div>
  );
}

export function AuditTimeline({ items }: { items: Array<Record<string, unknown>> }) {
  return (
    <div className="space-y-3">
      {items.map((item, index) => (
        <div
          key={String(item.id || index)}
          className="rounded-lg border border-border bg-muted/50 p-4"
        >
          <div className="flex flex-wrap items-center gap-3">
            <Badge className="bg-white">{String(item.action || "audit")}</Badge>
            <p className="text-sm font-medium text-black">{String(item.entity_type || "entity")}</p>
            <p className="text-xs text-muted-foreground">{String(item.actor_name || "system")}</p>
          </div>
          <p className="mt-3 text-xs leading-6 text-muted-foreground">
            {String(item.after_data || "{}")}
          </p>
        </div>
      ))}
    </div>
  );
}

export function QRPanel({ code, title }: { code?: string; title: string }) {
  const qrMarkup = code ? buildQRCodeSVG(code) : null;
  return (
    <div className="rounded-lg border border-dashed border-border bg-white px-6 py-8 text-center">
      <p className="text-xs font-medium text-muted-foreground">{title}</p>
      {qrMarkup ? (
        <div
          aria-label={`QR ${code}`}
          className="mx-auto mt-4 h-40 w-40 rounded-lg border border-border bg-white p-2"
          dangerouslySetInnerHTML={{ __html: qrMarkup }}
        />
      ) : (
        <div className="mx-auto mt-4 grid h-40 w-40 place-items-center rounded-lg border border-border bg-muted/50 text-xs uppercase tracking-[0.3em] text-muted-foreground">
          No QR
        </div>
      )}
      <p className="mt-4 text-sm font-medium text-black">{code || "No code"}</p>
      <p className="mt-2 text-xs text-muted-foreground">Camera scan is available on the receipt screen, with manual code fallback.</p>
    </div>
  );
}

export function Grid({
  className,
  children
}: {
  className?: string;
  children: ReactNode;
}) {
  return <div className={cn("grid gap-6 xl:grid-cols-[1.2fr_0.8fr]", className)}>{children}</div>;
}
