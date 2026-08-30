"use client";

import { LogOut } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { cn } from "@/lib/utils";
import { proxyClient } from "@/services/api";

export function LogoutButton({ compact = false }: { compact?: boolean }) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function logout() {
    setLoading(true);
    try {
      await proxyClient("/auth/logout", { method: "POST" });
    } finally {
      router.replace("/login");
      router.refresh();
      setLoading(false);
    }
  }

  return (
    <button
      aria-label="ออกจากระบบ"
      className={cn(
        "inline-flex items-center justify-center gap-2 rounded-full border border-border bg-white px-4 py-2 text-sm font-semibold text-foreground transition hover:border-primary hover:bg-primary hover:text-primary-foreground disabled:opacity-50",
        compact && "h-10 w-10 px-0"
      )}
      disabled={loading}
      onClick={() => void logout()}
      type="button"
    >
      <LogOut className="h-4 w-4" />
      {compact ? null : loading ? "กำลังออก..." : "ออกจากระบบ"}
    </button>
  );
}
