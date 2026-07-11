"use client";

import { startTransition, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";

import { QRPanel, SectionCard } from "@/components/sections/common";
import { Button, Input, Select } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

type Option = Record<string, unknown>;
type BarcodeLike = { rawValue?: string };
type BarcodeDetectorLike = {
  detect: (source: HTMLVideoElement) => Promise<BarcodeLike[]>;
};
type BarcodeDetectorCtor = new (options?: { formats?: string[] }) => BarcodeDetectorLike;

function readBarcodeDetector(): BarcodeDetectorCtor | null {
  if (typeof window === "undefined") {
    return null;
  }
  const detector = (window as Window & { BarcodeDetector?: BarcodeDetectorCtor }).BarcodeDetector;
  return detector || null;
}

function QRScanner({
  onDetected
}: {
  onDetected: (code: string) => Promise<void>;
}) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const frameRef = useRef<number | null>(null);
  const [error, setError] = useState<string>("");
  const [active, setActive] = useState(false);

  useEffect(() => {
    return () => {
      stop();
    };
  }, []);

  function stop() {
    if (frameRef.current) {
      cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    }
    setActive(false);
  }

  async function tick(detector: BarcodeDetectorLike) {
    if (!videoRef.current) {
      return;
    }
    try {
      const results = await detector.detect(videoRef.current);
      const code = results.find((item) => item.rawValue)?.rawValue;
      if (code) {
        stop();
        await onDetected(code);
        return;
      }
    } catch {
      // Ignore frame read errors and continue polling.
    }
    frameRef.current = requestAnimationFrame(() => {
      void tick(detector);
    });
  }

  async function start() {
    const Detector = readBarcodeDetector();
    if (!Detector) {
      setError("Browser does not support camera QR scanning. Use manual transfer code below.");
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: { ideal: "environment" } },
        audio: false
      });
      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await videoRef.current.play();
      }
      setError("");
      setActive(true);
      const detector = new Detector({ formats: ["qr_code"] });
      await tick(detector);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to access camera");
    }
  }

  return (
    <div className="space-y-3 rounded-lg border border-border bg-muted/50 p-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={start} type="button">
          {active ? "Scanning..." : "Start Camera Scan"}
        </Button>
        {active ? (
          <Button onClick={stop} type="button" variant="secondary">
            Stop
          </Button>
        ) : null}
      </div>
      <video
        ref={videoRef}
        className="h-56 w-full rounded-lg border border-border bg-black object-cover"
        muted
        playsInline
      />
      {error ? <p className="text-sm text-muted-foreground">{error}</p> : null}
    </div>
  );
}

export function TransferConsole({
  branches,
  products,
  transfers = [],
  defaultBranchId,
  mode = "admin"
}: {
  branches: Option[];
  products: Option[];
  transfers?: Option[];
  defaultBranchId?: string;
  mode?: "admin" | "receipt" | "all";
}) {
  const router = useRouter();
  const [sourceBranchId, setSourceBranchId] = useState(defaultBranchId || "");
  const [destinationBranchId, setDestinationBranchId] = useState("");
  const [productId, setProductId] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [stockBucket, setStockBucket] = useState("real");
  const [transferId, setTransferId] = useState("");
  const [transferCode, setTransferCode] = useState("");
  const [requestNote, setRequestNote] = useState("");
  const [pickupName, setPickupName] = useState("");
  const [courierName, setCourierName] = useState("");
  const [dispatchPickupName, setDispatchPickupName] = useState("");
  const [dispatchCourierName, setDispatchCourierName] = useState("");
  const [message, setMessage] = useState("");

  const receivableTransfers = transfers.filter(
    (item) =>
      String(item.destination_branch_id || "") === String(defaultBranchId || "") &&
      String(item.status || "") === "in_transit"
  );
  const dispatchableTransfers = transfers.filter(
    (item) =>
      String(item.source_branch_id || "") === String(defaultBranchId || "") &&
      String(item.status || "") === "requested"
  );
  const featuredTransfer = receivableTransfers[0];

  async function requestTransfer() {
    try {
      const response = await proxyClient<{ message: string }>("/transfers", {
        method: "POST",
        body: JSON.stringify({
          source_branch_id: sourceBranchId,
          destination_branch_id: destinationBranchId,
          request_note: requestNote,
          pickup_name: pickupName,
          courier_name: courierName,
          items: [
            {
              product_id: productId,
              quantity: Number(quantity),
              stock_bucket: stockBucket
            }
          ]
        })
      });
      setMessage(response.message);
      startTransition(() => router.refresh());
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Transfer request failed");
    }
  }

  async function dispatchTransfer() {
    try {
      await proxyClient(`/transfers/${transferId}/dispatch`, {
        method: "POST",
        body: JSON.stringify({ pickup_name: dispatchPickupName, courier_name: dispatchCourierName })
      });
      startTransition(() => router.refresh());
      setMessage("Transfer dispatched");
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Dispatch failed");
    }
  }

  async function receiveTransfer(codeOverride?: string) {
    const resolvedCode = (codeOverride || transferCode).trim();
    try {
      await proxyClient("/transfers/receive-by-code", {
        method: "POST",
        body: JSON.stringify({ transfer_code: resolvedCode })
      });
      setTransferCode(resolvedCode);
      startTransition(() => router.refresh());
      setMessage("Transfer received");
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "Receive failed");
    }
  }

  if (mode === "receipt") {
    return (
      <div className="space-y-6">
        <SectionCard
          title="Goods Transfer Receipt"
          description="Receive stock by QR camera scan or manual transfer code. The backend validates branch scope and updates stock."
        >
          <div className="grid gap-6 lg:grid-cols-[0.9fr_1.1fr]">
            <QRPanel
              code={String(featuredTransfer?.qr_code || featuredTransfer?.transfer_code || "")}
              title="Transfer QR"
            />
            <div className="space-y-4">
              <QRScanner onDetected={receiveTransfer} />
              <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto]">
                <Input
                  aria-label="Transfer Code"
                  value={transferCode}
                  onChange={(event) => setTransferCode(event.target.value)}
                  placeholder="Transfer code"
                />
                <Button onClick={() => void receiveTransfer()} type="button">
                  Receive by Code
                </Button>
              </div>
              {message ? <p className="text-sm text-muted-foreground">{message}</p> : null}
            </div>
          </div>
        </SectionCard>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SectionCard title="Request Transfer" description="Request or create an inter-branch transfer with backend-owned stock validation.">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <Select aria-label="Source Branch" value={sourceBranchId} onChange={(event) => setSourceBranchId(event.target.value)}>
            <option value="">Source branch</option>
            {branches.map((branch) => (
              <option key={String(branch.id)} value={String(branch.id)}>
                {String(branch.name)}
              </option>
            ))}
          </Select>
          <Select
            aria-label="Destination Branch"
            value={destinationBranchId}
            onChange={(event) => setDestinationBranchId(event.target.value)}
          >
            <option value="">Destination branch</option>
            {branches.map((branch) => (
              <option key={String(branch.id)} value={String(branch.id)}>
                {String(branch.name)}
              </option>
            ))}
          </Select>
          <Select aria-label="Transfer Product" value={productId} onChange={(event) => setProductId(event.target.value)}>
            <option value="">Product</option>
            {products.map((product) => (
              <option key={String(product.id)} value={String(product.id)}>
                {String(product.name)}
              </option>
            ))}
          </Select>
          <Input aria-label="Transfer Quantity" value={quantity} onChange={(event) => setQuantity(event.target.value)} type="number" />
          <Select aria-label="Transfer Stock Bucket" value={stockBucket} onChange={(event) => setStockBucket(event.target.value)}>
            <option value="real">Real</option>
            <option value="ghost">Ghost</option>
          </Select>
          <Input aria-label="Transfer Note" placeholder="Request note" value={requestNote} onChange={(event) => setRequestNote(event.target.value)} />
          <Input aria-label="Pickup Name" placeholder="Pickup name" value={pickupName} onChange={(event) => setPickupName(event.target.value)} />
          <Input aria-label="Courier Name" placeholder="Courier name" value={courierName} onChange={(event) => setCourierName(event.target.value)} />
          <Button onClick={requestTransfer} type="button">
            Request Transfer
          </Button>
        </div>
      </SectionCard>

      <SectionCard title="Dispatch Queue" description="Dispatch transfers from the source branch.">
        <div className="grid gap-4 md:grid-cols-2">
          <Select aria-label="Transfer ID" value={transferId} onChange={(event) => setTransferId(event.target.value)}>
            <option value="">Select transfer</option>
            {dispatchableTransfers.map((transfer) => (
              <option key={String(transfer.id)} value={String(transfer.id)}>
                {String(transfer.transfer_code)} / {String(transfer.status)}
              </option>
            ))}
          </Select>
          <Input aria-label="Dispatch Pickup Name" placeholder="Pickup name" value={dispatchPickupName} onChange={(event) => setDispatchPickupName(event.target.value)} />
          <Input aria-label="Dispatch Courier Name" placeholder="Courier name" value={dispatchCourierName} onChange={(event) => setDispatchCourierName(event.target.value)} />
          <Button onClick={dispatchTransfer} type="button" variant="secondary">
            Dispatch
          </Button>
        </div>
        {message ? <p className="mt-4 text-sm text-muted-foreground">{message}</p> : null}
      </SectionCard>
    </div>
  );
}
