import { CircleAlert, Loader2 } from "lucide-react";

import { cn } from "@/lib/utils";

/**
 * The two validation states that sit alongside EmptyState (Global Rules:
 * every fetch must express has-data / no-data / loading / error). Defined
 * once here so a spinner and a failure read the same on every screen.
 */
export function LoadingState({
  className,
  label = "กำลังโหลดข้อมูล..."
}: {
  className?: string;
  label?: string;
}) {
  return (
    <div
      aria-busy="true"
      aria-live="polite"
      className={cn("grid place-items-center gap-2 p-10 text-center", className)}
      role="status"
    >
      <Loader2 className="h-8 w-8 animate-spin text-primary" />
      <p className="text-sm font-semibold text-muted-foreground">{label}</p>
    </div>
  );
}

export function ErrorState({
  className,
  description,
  title = "โหลดข้อมูลไม่สำเร็จ"
}: {
  className?: string;
  /** The actual failure reason — always show it; a bare title tells the user nothing. */
  description?: string;
  title?: string;
}) {
  return (
    <div className={cn("grid place-items-center gap-2 p-10 text-center", className)} role="alert">
      <CircleAlert className="h-8 w-8 text-error" />
      <p className="text-sm font-semibold">{title}</p>
      {description ? <p className="max-w-md text-xs text-muted-foreground">{description}</p> : null}
    </div>
  );
}

/**
 * Inline feedback banner for a completed action — one look for success,
 * error, warning, and neutral info across every console.
 */
export function Notice({
  children,
  className,
  tone = "info"
}: {
  children: React.ReactNode;
  className?: string;
  tone?: "info" | "success" | "warning" | "error";
}) {
  const tones = {
    info: "bg-info-50 text-info-800",
    success: "bg-success-50 text-success-800",
    warning: "bg-warning-50 text-warning-800",
    error: "bg-error-50 text-error"
  } as const;
  return (
    <p
      className={cn("rounded-xl px-4 py-2.5 text-sm", tones[tone], className)}
      role={tone === "error" ? "alert" : "status"}
    >
      {children}
    </p>
  );
}
