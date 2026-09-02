"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, ImagePlus, Images, Package } from "lucide-react";

import {
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  Notice,
} from "@/components/ui/primitives";
import { cn } from "@/lib/utils";
import { proxyClient } from "@/services/api";

type GalleryImage = {
  id: string;
  alt_text: string;
  is_primary: boolean;
  source_branch_code: string;
  source_name: string;
  branch_scoped?: boolean;
  url: string;
};

/**
 * D2's "จัดการสต๊อก" detail panel: one large square product image; if more
 * than one exists, a swipeable gallery (drag or the arrow buttons) instead
 * of the small thumbnail + "ดูรูปทั้งหมด" dialog used elsewhere.
 */
export function ProductImageSwiper({
  productId,
  productName,
  imageCount,
  available,
  className,
  branchId,
  onImageAdded,
}: {
  productId: string;
  productName: string;
  imageCount: number;
  available: boolean;
  className?: string;
  /** Set on the stock pages: shows this branch's own images alongside the
   *  catalog's, and enables the "เพิ่มรูปใหม่" upload. */
  branchId?: string;
  onImageAdded?: () => void;
}) {
  const [items, setItems] = useState<GalleryImage[] | null>(null);
  const [index, setIndex] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const dragStartX = useRef<number | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const listPath = branchId
    ? `/products/${productId}/images?branch_id=${encodeURIComponent(branchId)}`
    : `/products/${productId}/images`;

  const reload = useCallback(() => {
    proxyClient<{ items: GalleryImage[] }>(listPath)
      .then((response) => setItems(response.items))
      .catch(() => setItems([]));
  }, [listPath]);

  useEffect(() => {
    setItems(null);
    setIndex(0);
    // With a branch we always fetch: the branch may have added images even
    // when the catalog itself has one or none.
    if (branchId || imageCount > 1) reload();
  }, [branchId, imageCount, productId, reload]);

  async function upload(file: File) {
    if (!branchId) return;
    setUploading(true);
    setUploadError("");
    try {
      const body = new FormData();
      body.append("file", file);
      body.append("branch_id", branchId);
      await proxyClient(`/products/${productId}/image`, { method: "POST", body });
      reload();
      onImageAdded?.();
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : "อัปโหลดรูปไม่สำเร็จ");
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  const gallery = items && items.length > 1 ? items : null;
  const hasAnyImage = available || (items?.length || 0) > 0;
  const currentSrc = gallery
    ? `/api/backend${gallery[index]?.url}`
    : items?.length === 1
      ? `/api/backend${items[0].url}`
      : `/api/backend/products/${productId}/image`;

  function go(delta: number) {
    if (!gallery) return;
    setIndex((current) => (current + delta + gallery.length) % gallery.length);
  }

  const uploader = branchId ? (
    <div className="mt-3 space-y-2">
      <input
        accept="image/*"
        aria-label="เลือกไฟล์รูปสำหรับสาขานี้"
        className="hidden"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) void upload(file);
        }}
        ref={fileRef}
        type="file"
      />
      <Button
        className="w-full"
        disabled={uploading}
        onClick={() => fileRef.current?.click()}
        type="button"
        variant="secondary"
      >
        <ImagePlus className="h-4 w-4" />
        {uploading ? "กำลังอัปโหลด..." : "เพิ่มรูปใหม่"}
      </Button>
      <p className="text-center text-xs text-muted-foreground">
        รูปที่เพิ่มจะแสดงเฉพาะสาขานี้ ไม่กระทบรูปหลักใน รายการสินค้า
      </p>
      {uploadError ? <Notice tone="error">{uploadError}</Notice> : null}
    </div>
  ) : null;

  if (!hasAnyImage) {
    return (
      <div className={cn("w-full max-w-xs", className)}>
        <div className="grid aspect-square w-full place-items-center rounded-2xl border bg-gradient-to-br from-orange-50 to-amber-100">
          <Package aria-label={`ยังไม่มีรูปสำหรับ ${productName}`} className="h-16 w-16 text-primary/30" />
        </div>
        {uploader}
      </div>
    );
  }

  return (
    <div className={cn("w-full max-w-xs", className)} data-testid="product-image-swiper">
      <div className="relative aspect-square w-full touch-pan-y select-none overflow-hidden rounded-2xl border bg-surface-warm">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          alt={gallery?.[index]?.alt_text || productName}
          className="h-full w-full object-contain"
          onPointerDown={(event) => {
            dragStartX.current = event.clientX;
          }}
          onPointerUp={(event) => {
            if (dragStartX.current === null || !gallery) return;
            const delta = event.clientX - dragStartX.current;
            dragStartX.current = null;
            if (Math.abs(delta) > 40) go(delta > 0 ? -1 : 1);
          }}
          src={currentSrc}
        />
        {gallery?.[index]?.branch_scoped ? (
          <span className="absolute left-2 top-2 rounded-full bg-info-50 px-2.5 py-1 text-xs font-semibold text-info-800">
            รูปของสาขา
          </span>
        ) : null}
        {gallery ? (
          <>
            <button
              aria-label="รูปก่อนหน้า"
              className="absolute left-2 top-1/2 grid h-8 w-8 -translate-y-1/2 place-items-center rounded-full bg-white/90 shadow-card hover:bg-white"
              onClick={() => go(-1)}
              type="button"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <button
              aria-label="รูปถัดไป"
              className="absolute right-2 top-1/2 grid h-8 w-8 -translate-y-1/2 place-items-center rounded-full bg-white/90 shadow-card hover:bg-white"
              onClick={() => go(1)}
              type="button"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </>
        ) : null}
      </div>
      {gallery ? (
        <div className="mt-2 flex justify-center gap-1.5">
          {gallery.map((image, i) => (
            <button
              aria-label={`รูปที่ ${i + 1}`}
              className={cn("h-1.5 w-1.5 rounded-full", i === index ? "bg-primary" : "bg-muted")}
              key={image.id}
              onClick={() => setIndex(i)}
              type="button"
            />
          ))}
        </div>
      ) : null}
      {uploader}
    </div>
  );
}

export function ProductImageGallery({
  productId,
  productName,
  imageCount,
}: {
  productId: string;
  productName: string;
  imageCount: number;
}) {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<GalleryImage[]>([]);
  const [loading, setLoading] = useState(false);

  async function changeOpen(next: boolean) {
    setOpen(next);
    if (!next || items.length) return;
    setLoading(true);
    try {
      const response = await proxyClient<{ items: GalleryImage[] }>(
        `/products/${productId}/images`,
      );
      setItems(response.items);
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog onOpenChange={(next) => void changeOpen(next)} open={open}>
      <Button onClick={() => void changeOpen(true)} type="button" variant="secondary">
        <Images className="h-4 w-4" />
        ดูรูปทั้งหมด ({imageCount})
      </Button>
      <DialogContent className="max-w-5xl">
        <DialogHeader title={`รูปสินค้า · ${productName}`} />
        {loading ? <p className="p-8 text-center text-muted-foreground">กำลังโหลดรูป...</p> : null}
        <div className="grid max-h-[70vh] gap-4 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3">
          {items.map((item) => (
            <figure className="overflow-hidden rounded-2xl border bg-white" key={item.id}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                alt={item.alt_text || productName}
                className="aspect-square w-full object-contain bg-surface-warm"
                loading="lazy"
                src={`/api/backend${item.url}`}
              />
              <figcaption className="p-3 text-xs text-muted-foreground">
                {item.is_primary ? <strong className="mr-2 text-primary">รูปหลัก</strong> : null}
                {item.source_branch_code || "รูปที่อัปโหลด"}
                {item.source_name ? ` · ${item.source_name}` : ""}
              </figcaption>
            </figure>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
