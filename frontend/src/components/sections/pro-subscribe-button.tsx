"use client";

import { Sparkles } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/primitives";

/** The upgrade call-to-action. Billing is not wired up, so it acknowledges the
 *  intent rather than pretending to charge a card. */
export function ProSubscribeButton() {
  return (
    <Button
      className="w-full"
      onClick={() => toast.success("รับทราบความสนใจแล้ว", { description: "ทีมงานจะติดต่อกลับเพื่อเปิดใช้งาน PharmaPOS Pro" })}
      type="button"
    >
      <Sparkles className="h-4 w-4" />
      สมัคร Pro รายเดือน
    </Button>
  );
}
