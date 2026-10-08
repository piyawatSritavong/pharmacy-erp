import "./globals.css";

import type { Metadata, Viewport } from "next";
import { Noto_Sans_Thai } from "next/font/google";
import type { ReactNode } from "react";

import { Toaster } from "@/components/ui/sonner";
import { ThemeScript } from "@/components/ui/theme-toggle";

const notoSansThai = Noto_Sans_Thai({
  subsets: ["thai", "latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-sans",
  display: "swap"
});

const DESCRIPTION = "ระบบขายหน้าร้านและบริหารสต๊อกสำหรับเครือร้านขายยาหลายสาขา";

export const metadata: Metadata = {
  // Each page sets its own title; the tab then reads "ใบสั่งซื้อเข้า | PharmaPOS".
  title: { default: "PharmaPOS | ระบบบริหารร้านขายยา", template: "%s | PharmaPOS" },
  description: DESCRIPTION,
  applicationName: "PharmaPOS",
  // An internal back office: never in a search index, even if a URL leaks.
  robots: { index: false, follow: false },
  // Enough for a link pasted into LINE to show a proper card.
  openGraph: { title: "PharmaPOS", description: DESCRIPTION, locale: "th_TH", type: "website" },
  // Served from public/ rather than app/ metadata routes, so nothing here
  // joins the prerendered route list.
  icons: { icon: [{ url: "/icon.svg", type: "image/svg+xml" }], apple: "/apple-touch-icon.png" },
  manifest: "/manifest.webmanifest"
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  // Lets env(safe-area-inset-*) take effect, so the till's bottom bar and
  // dialog footers clear the home indicator on notched phones.
  viewportFit: "cover",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#344ABF" },
    { media: "(prefers-color-scheme: dark)", color: "#14171f" }
  ]
};

export default function RootLayout({
  children
}: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="th" className={notoSansThai.variable} suppressHydrationWarning>
      <head>
        <ThemeScript />
      </head>
      <body className="font-sans">
        {children}
        <Toaster />
      </body>
    </html>
  );
}
