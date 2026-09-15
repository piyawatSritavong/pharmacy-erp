"use client";

import { useLayoutEffect, useState, type RefObject } from "react";

/** Keep anchored results inside the visible viewport, including when the
 * virtual keyboard changes its size. Prefer below; flip when above has room. */
export function usePopoverPosition(open: boolean, anchor: RefObject<HTMLElement>, height = 288) {
  const [position, setPosition] = useState<{ top: number; left: number; width: number; maxHeight: number; above: boolean } | null>(null);
  useLayoutEffect(() => {
    if (!open) return;
    const update = () => {
      const rect = anchor.current?.getBoundingClientRect();
      if (!rect) return;
      const viewport = window.visualViewport;
      const top = (viewport?.offsetTop || 0) + 8;
      const bottom = (viewport?.offsetTop || 0) + (viewport?.height || window.innerHeight) - 8;
      const below = Math.max(0, bottom - rect.bottom - 8);
      const aboveSpace = Math.max(0, rect.top - top - 8);
      const above = below < height && aboveSpace > below;
      const maxHeight = Math.min(height, above ? aboveSpace : below);
      const left = (viewport?.offsetLeft || 0) + 8;
      const width = Math.min(rect.width, (viewport?.width || window.innerWidth) - 16);
      setPosition({
        top: above ? Math.max(top, rect.top - maxHeight - 8) : Math.max(top, rect.bottom + 8),
        left: Math.max(left, Math.min(rect.left, left + (viewport?.width || window.innerWidth) - width - 16)),
        width, maxHeight, above
      });
    };
    update();
    window.addEventListener("scroll", update, true);
    window.addEventListener("resize", update);
    window.visualViewport?.addEventListener("resize", update);
    window.visualViewport?.addEventListener("scroll", update);
    return () => {
      window.removeEventListener("scroll", update, true);
      window.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("scroll", update);
    };
  }, [open, anchor, height]);
  return position;
}
