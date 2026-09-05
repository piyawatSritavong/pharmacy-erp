"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";

export const THEME_STORAGE_KEY = "pharmacy-erp-theme";

/**
 * Applied before the first paint by ThemeScript and again by the toggle, so the
 * two never disagree about what "dark" means.
 */
function applyTheme(theme: "light" | "dark") {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

/**
 * Light/dark switch for the header. The choice is remembered per browser, which
 * is the right grain for this product: a till in a bright shop and an office
 * machine at night are different screens with different needs, and one operator
 * account may sit at both.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    // Read what ThemeScript already decided rather than deciding again.
    setTheme(document.documentElement.classList.contains("dark") ? "dark" : "light");
  }, []);

  function toggle() {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    applyTheme(next);
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, next);
    } catch {
      // Private browsing or blocked storage: the theme still applies for this
      // session, it just will not be remembered.
    }
  }

  return (
    <button
      aria-label={theme === "dark" ? "เปลี่ยนเป็นธีมสว่าง" : "เปลี่ยนเป็นธีมมืด"}
      className="inline-flex h-10 w-10 items-center justify-center rounded-xl border bg-card text-muted-foreground transition hover:bg-muted hover:text-foreground"
      onClick={toggle}
      title={theme === "dark" ? "ธีมสว่าง" : "ธีมมืด"}
      type="button"
    >
      {theme === "dark" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
    </button>
  );
}

/**
 * Runs before React hydrates so the page never paints light and then flips.
 * Falls back to the operating system's setting the first time, then follows
 * whatever the operator chose.
 */
export function ThemeScript() {
  const script = `(function(){try{var s=localStorage.getItem(${JSON.stringify(THEME_STORAGE_KEY)});var d=s?s==="dark":window.matchMedia("(prefers-color-scheme: dark)").matches;if(d)document.documentElement.classList.add("dark")}catch(e){}})()`;
  return <script dangerouslySetInnerHTML={{ __html: script }} suppressHydrationWarning />;
}
