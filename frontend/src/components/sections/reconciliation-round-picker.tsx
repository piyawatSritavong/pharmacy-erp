"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Search, X } from "lucide-react";

import { Input } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

const PAGE_SIZE = 20;

type Round = {
  id: string;
  reconciliation_number: string;
  period_start: string;
  period_end: string;
  final_revenue: number;
};

/**
 * Picks a closed round to scope the dashboard to. Rounds accumulate one per
 * branch per close, so a year of trading is hundreds of them: the list opens on
 * the twenty most recent and fetches twenty more when the operator scrolls to
 * the end, and typing filters on the server rather than on what happens to be
 * loaded.
 */
export function ReconciliationRoundPicker({
  value,
  label,
  onChange
}: {
  value: string;
  label: string;
  onChange: (id: string, round?: Round) => void;
}) {
  const [query, setQuery] = useState(label);
  const [open, setOpen] = useState(false);
  const [rounds, setRounds] = useState<Round[]>([]);
  const [loading, setLoading] = useState(false);
  const [exhausted, setExhausted] = useState(false);
  const listRef = useRef<HTMLDivElement | null>(null);
  // The search the loaded page belongs to, so a result arriving late for an
  // abandoned query cannot overwrite the current one.
  const activeSearch = useRef("");

  useEffect(() => {
    setQuery(label);
  }, [label]);

  const load = useCallback(async (search: string, offset: number) => {
    setLoading(true);
    activeSearch.current = search;
    try {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
      if (search) params.set("search", search);
      const result = await proxyClient<{ items: Round[] }>(`/accounting/month-end/reconciliations?${params.toString()}`);
      if (activeSearch.current !== search) return;
      const items = result.items || [];
      setRounds((previous) => (offset === 0 ? items : [...previous, ...items]));
      setExhausted(items.length < PAGE_SIZE);
    } catch {
      setExhausted(true);
    } finally {
      setLoading(false);
    }
  }, []);

  // Typing re-queries from the top, debounced so each keystroke is not a request.
  useEffect(() => {
    if (!open) return;
    const timer = setTimeout(() => {
      const search = query === label ? "" : query.trim();
      void load(search, 0);
    }, 220);
    return () => clearTimeout(timer);
  }, [label, load, open, query]);

  function onScroll() {
    const list = listRef.current;
    if (!list || loading || exhausted) return;
    if (list.scrollTop + list.clientHeight >= list.scrollHeight - 24) {
      void load(activeSearch.current, rounds.length);
    }
  }

  function choose(round: Round) {
    onChange(round.id, round);
    setQuery(round.reconciliation_number);
    setOpen(false);
  }

  function clear() {
    onChange("");
    setQuery("");
    setRounds([]);
    setOpen(false);
  }

  return (
    <div className="relative">
      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        aria-label="ค้นหารอบสรุปสิ้นเดือน"
        className="pl-9 pr-9"
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        placeholder="ทุกรอบ"
        value={query}
      />
      {value || query ? (
        <button
          aria-label="ล้างรอบที่เลือก"
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-1 text-muted-foreground transition hover:bg-muted"
          onClick={clear}
          type="button"
        >
          <X className="h-4 w-4" />
        </button>
      ) : null}

      {open ? (
        <>
          {/* Click-away without a portal: the overlay sits behind the list. */}
          <button aria-hidden className="fixed inset-0 z-30 cursor-default" onClick={() => setOpen(false)} tabIndex={-1} type="button" />
          <div
            className="absolute left-0 right-0 z-40 mt-1 max-h-64 overflow-y-auto rounded-xl border bg-card p-1 shadow-card"
            onScroll={onScroll}
            ref={listRef}
          >
            {rounds.map((round) => (
              <button
                className={`block w-full rounded-lg px-3 py-2 text-left text-sm transition hover:bg-muted ${round.id === value ? "bg-muted" : ""}`}
                key={round.id}
                onClick={() => choose(round)}
                type="button"
              >
                <span className="block font-medium">{round.reconciliation_number}</span>
                <span className="block text-xs text-muted-foreground">
                  {round.period_start} – {round.period_end}
                </span>
              </button>
            ))}
            {loading ? <p className="px-3 py-2 text-sm text-muted-foreground">กำลังโหลด…</p> : null}
            {!loading && rounds.length === 0 ? (
              <p className="px-3 py-2 text-sm text-muted-foreground">ไม่พบรอบที่ตรงกับคำค้น</p>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}
