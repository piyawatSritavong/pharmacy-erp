import "./globals.css";

import type { Metadata } from "next";
import type { ReactNode } from "react";

import { Toaster } from "@/components/ui/sonner";

export const metadata: Metadata = {
  title: "Pharmacy ERP",
  description: "Backend-centric pharmacy ERP thin client"
};

export default function RootLayout({
  children
}: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="th">
      <body>
        {children}
        <Toaster />
      </body>
    </html>
  );
}
