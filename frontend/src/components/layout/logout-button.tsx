"use client";

import { LogOut } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { cn } from "@/lib/utils";
import { ProxyError, proxyClient } from "@/services/api";

export function LogoutButton({ compact = false }: { compact?: boolean }) {
  const [loading, setLoading] = useState(false);

  async function logout() {
    setLoading(true);
    try {
      await proxyClient("/auth/logout", { method: "POST" });
    } catch (caught) {
      // The navigation used to live in a `finally`, so it ran even when the
      // request never completed — showing a login screen over a session whose
      // cookie was still live and still worked on the next click. If the
      // request failed, nothing was cleared: say so and stay put.
      // A ProxyError carries what the server said, which is already Thai and
      // worth showing. Anything else is a network-level failure whose message
      // is a raw browser string ("Failed to fetch") — not something to put in
      // front of someone at a till.
      toast.error(caught instanceof ProxyError ? caught.message : "ออกจากระบบไม่สำเร็จ กรุณาลองใหม่");
      setLoading(false);
      return;
    }

    // Only now, and as a document load. The client Router Cache still holds
    // the authenticated payload of every page visited this session; a client
    // navigation would leave all of it in memory behind the login screen.
    window.location.replace("/login");
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
