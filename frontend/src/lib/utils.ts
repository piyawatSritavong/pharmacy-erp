import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...parts: ClassValue[]) {
  return twMerge(clsx(parts));
}

export function currency(value: number) {
  return new Intl.NumberFormat("th-TH", {
    style: "currency",
    currency: "THB",
    minimumFractionDigits: 2
  }).format(value ?? 0);
}

export function dateTime(value: string) {
  return new Intl.DateTimeFormat("th-TH", {
    dateStyle: "medium",
    timeStyle: "short",
    // Explicit, not inherited from the runtime's OS timezone: the Docker
    // server and each user's browser can otherwise sit in different zones,
    // so the same instant renders as different text server-side vs
    // client-side and React throws a hydration mismatch (seen live on
    // /sales-management, error #418) the moment a table has a real row.
    // Pinning it here is correct regardless of where either side runs,
    // since every user of this app is in Thailand.
    timeZone: "Asia/Bangkok"
  }).format(new Date(value));
}

export function generateReadableCode(prefix: string) {
  const date = new Date().toISOString().slice(0, 10).replaceAll("-", "");
  const random = crypto.randomUUID().replaceAll("-", "").slice(0, 6).toUpperCase();
  return `${prefix.toUpperCase()}-${date}-${random}`;
}
