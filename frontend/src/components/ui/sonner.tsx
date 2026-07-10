"use client";

import { Toaster as SonnerToaster } from "sonner";

export function Toaster() {
  return (
    <SonnerToaster
      closeButton
      position="top-right"
      richColors={false}
      theme="light"
      toastOptions={{
        className: "border border-black/10 bg-white text-black shadow-lg"
      }}
    />
  );
}
