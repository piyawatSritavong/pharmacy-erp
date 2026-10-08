"use client";

import { Package } from "lucide-react";
import { useState } from "react";

import { cn } from "@/lib/utils";

export function ProductThumbnail({
  productId,
  name,
  available,
  imageCount = 0,
  className,
}: {
  productId: string;
  name: string;
  available: boolean;
  imageCount?: number;
  className?: string;
}) {
  // A photo that fails to load (storage down, file gone) shows the placeholder.
  const [failed, setFailed] = useState(false);
  return (
    <span
      className={cn(
        "relative grid shrink-0 place-items-center overflow-hidden rounded-xl bg-gradient-to-br from-placeholder-from to-placeholder-to",
        className,
      )}
    >
      {available && !failed ? (
        // The authenticated backend proxy serves the current primary gallery image.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          alt={name}
          className="h-full w-full object-cover"
          loading="lazy"
          onError={() => setFailed(true)}
          src={`/api/backend/products/${productId}/image`}
        />
      ) : (
        <Package aria-label={`ยังไม่มีรูปสำหรับ ${name}`} className="h-1/2 w-1/2 text-primary/30" />
      )}
      {imageCount > 1 ? (
        <span className="absolute bottom-1 right-1 rounded-full bg-black/70 px-1.5 py-0.5 text-2xs font-bold text-white">
          {imageCount} รูป
        </span>
      ) : null}
    </span>
  );
}
