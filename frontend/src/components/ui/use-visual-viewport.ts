"use client";

import { useEffect } from "react";

/** dvh alone does not account for the on-screen keyboard in every browser. */
export function useVisualViewport() {
  useEffect(() => {
    const viewport = window.visualViewport;
    if (!viewport) return;
    const style = document.documentElement.style;
    const update = () => {
      style.setProperty("--app-viewport-height", `${viewport.height}px`);
      style.setProperty("--app-viewport-top", `${viewport.offsetTop}px`);
    };
    update();
    viewport.addEventListener("resize", update);
    viewport.addEventListener("scroll", update);
    return () => {
      viewport.removeEventListener("resize", update);
      viewport.removeEventListener("scroll", update);
      style.removeProperty("--app-viewport-height");
      style.removeProperty("--app-viewport-top");
    };
  }, []);
}
