"use client";

import { useEffect, useState } from "react";
import { Toaster as SonnerToaster } from "sonner";

/** Toasts follow the app theme (the .dark class ThemeToggle sets), not a fixed light skin. */
export function Toaster() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  useEffect(() => {
    const root = document.documentElement;
    const read = () => setTheme(root.classList.contains("dark") ? "dark" : "light");
    read();
    const observer = new MutationObserver(read);
    observer.observe(root, { attributeFilter: ["class"], attributes: true });
    return () => observer.disconnect();
  }, []);
  return (
    <SonnerToaster
      closeButton
      position="top-right"
      richColors={false}
      theme={theme}
      toastOptions={{
        className: "border border-border bg-card text-foreground shadow-lg"
      }}
    />
  );
}
