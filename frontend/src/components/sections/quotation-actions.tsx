"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

export function ConvertQuotationButton({ id }: { id: string }) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);

  async function handleClick() {
    setLoading(true);
    try {
      await proxyClient(`/quotations/${id}/convert`, { method: "POST" });
      startTransition(() => router.refresh());
    } finally {
      setLoading(false);
    }
  }

  return (
    <Button onClick={handleClick} type="button" variant="secondary">
      {loading ? "Converting..." : "Convert"}
    </Button>
  );
}
