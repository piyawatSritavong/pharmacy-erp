"use client";

import { useMemo, useState } from "react";
import { AlertTriangle, ChevronDown, ChevronRight } from "lucide-react";

import { SectionCard } from "@/components/sections/common";
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@/components/ui/primitives";

type LowStockRow = {
  branch_code: string;
  branch_name: string;
  product_name: string;
  sku: string;
  qty_real: number;
  threshold: number;
};

function count(value: number) {
  return value.toLocaleString("th-TH");
}

/** A shortfall pill: how far below the reorder point this line sits. */
function ShortBadge({ qty, threshold }: { qty: number; threshold: number }) {
  const outOfStock = qty <= 0;
  return (
    <span className={`inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-semibold ${outOfStock ? "bg-red-100 text-red-700" : "bg-amber-100 text-amber-800"}`}>
      {outOfStock ? "หมด" : `เหลือ ${count(qty)}`} / จุดเตือน {count(threshold)}
    </span>
  );
}

/**
 * Low-stock alerts, two ways: one company-wide table (สำนักงานใหญ่) that rolls
 * every branch's shortfall of a product into a single row, and one collapsible
 * table per branch beneath it. The company view answers "what do we need to
 * buy?"; the per-branch views answer "who needs it, and how much?".
 */
export function LowStockTables({ rows }: { rows: LowStockRow[] }) {
  const company = useMemo(() => {
    const byProduct = new Map<string, { product_name: string; sku: string; totalQty: number; branches: number; minThreshold: number }>();
    for (const row of rows) {
      const entry = byProduct.get(row.sku) || { product_name: row.product_name, sku: row.sku, totalQty: 0, branches: 0, minThreshold: row.threshold };
      entry.totalQty += row.qty_real;
      entry.branches += 1;
      entry.minThreshold = Math.min(entry.minThreshold, row.threshold);
      byProduct.set(row.sku, entry);
    }
    return [...byProduct.values()].sort((a, b) => a.totalQty - b.totalQty || b.branches - a.branches);
  }, [rows]);

  const branches = useMemo(() => {
    const byBranch = new Map<string, { name: string; rows: LowStockRow[] }>();
    for (const row of rows) {
      const entry = byBranch.get(row.branch_code) || { name: row.branch_name, rows: [] };
      entry.rows.push(row);
      byBranch.set(row.branch_code, entry);
    }
    return [...byBranch.entries()].map(([code, value]) => ({ code, ...value })).sort((a, b) => b.rows.length - a.rows.length);
  }, [rows]);

  const [open, setOpen] = useState<Record<string, boolean>>({});

  if (rows.length === 0) {
    return (
      <SectionCard title="แจ้งเตือนสินค้าใกล้หมด" description="สินค้าที่ถึงหรือต่ำกว่าจุดแจ้งเตือนสต๊อก">
        <p className="py-8 text-center text-sm text-muted-foreground">ไม่มีสินค้าที่ถึงจุดแจ้งเตือนในขณะนี้</p>
      </SectionCard>
    );
  }

  return (
    <div className="space-y-4">
      <SectionCard
        title="สำนักงานใหญ่ · ทุกสาขารวมกัน"
        description={`${count(company.length)} รายการที่ต้องเติม รวมทุกสาขา`}
      >
        <TableContainer>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>สินค้า</TableHead>
                <TableHead>SKU</TableHead>
                <TableHead className="text-right">คงเหลือรวมทุกสาขา</TableHead>
                <TableHead className="text-right">สาขาที่ใกล้หมด</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {company.map((item) => (
                <TableRow key={item.sku}>
                  <TableCell className="font-medium">{item.product_name}</TableCell>
                  <TableCell className="whitespace-nowrap text-muted-foreground">{item.sku}</TableCell>
                  <TableCell className="text-right">
                    <span className={`font-semibold tabular-nums ${item.totalQty <= 0 ? "text-red-700" : ""}`}>{count(item.totalQty)}</span>
                  </TableCell>
                  <TableCell className="text-right">
                    <span className="inline-flex items-center gap-1 whitespace-nowrap text-sm text-amber-800"><AlertTriangle className="h-3.5 w-3.5" />{count(item.branches)} สาขา</span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </SectionCard>

      <SectionCard
        title="แยกตามสาขา"
        description="กดที่ชื่อสาขาเพื่อดูรายการสินค้าที่ใกล้หมดของสาขานั้น"
      >
        <div className="space-y-2">
          {branches.map((branch, index) => {
            const expanded = open[branch.code] ?? index === 0;
            return (
              <div className="overflow-hidden rounded-lg border" key={branch.code}>
                <button
                  aria-expanded={expanded}
                  className="flex w-full items-center gap-3 bg-muted/40 px-4 py-3 text-left transition hover:bg-muted"
                  onClick={() => setOpen((current) => ({ ...current, [branch.code]: !expanded }))}
                  type="button"
                >
                  {expanded ? <ChevronDown className="h-4 w-4 shrink-0" /> : <ChevronRight className="h-4 w-4 shrink-0" />}
                  <span className="flex-1 font-semibold">{branch.name}</span>
                  <span className="shrink-0 rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-semibold text-amber-800">{count(branch.rows.length)} รายการ</span>
                </button>
                {expanded ? (
                  <TableContainer>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>สินค้า</TableHead>
                          <TableHead>SKU</TableHead>
                          <TableHead className="text-right">สถานะ</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {branch.rows.map((row) => (
                          <TableRow key={`${branch.code}:${row.sku}`}>
                            <TableCell className="font-medium">{row.product_name}</TableCell>
                            <TableCell className="whitespace-nowrap text-muted-foreground">{row.sku}</TableCell>
                            <TableCell className="text-right"><ShortBadge qty={row.qty_real} threshold={row.threshold} /></TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                ) : null}
              </div>
            );
          })}
        </div>
      </SectionCard>
    </div>
  );
}
