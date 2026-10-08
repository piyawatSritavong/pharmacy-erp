import type { Metadata } from "next";
import type { ReactNode } from "react";

// The login page itself is a client component, which cannot export metadata.
export const metadata: Metadata = { title: "เข้าสู่ระบบ" };

export default function LoginLayout({ children }: { children: ReactNode }) {
  return children;
}
