import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";

// ============================================================================
// VALIDATION SCHEMA
// ============================================================================

const transferStockSchema = z.object({
  productId: z.string().uuid("Invalid product ID"),
  fromType: z.enum(["real", "ghost"], {
    errorMap: () => ({ message: "Select source inventory (Real or Ghost)" }),
  }),
  toType: z.enum(["real", "ghost"], {
    errorMap: () => ({ message: "Select destination inventory (Real or Ghost)" }),
  }),
  quantity: z
    .number()
    .min(1, "Quantity must be at least 1")
    .max(10000, "Quantity seems too high"),
  reason: z.string().min(5, "Please provide a reason for the transfer"),
});

type TransferStockFormData = z.infer<typeof transferStockSchema>;

// ============================================================================
// COMPONENT
// ============================================================================

interface TransferStockDialogProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  branchId: string;
  products: Array<{ id: string; name: string; sku: string }>;
  currentInventory: {
    [key: string]: { real: number; ghost: number };
  };
  onTransferSuccess?: () => void;
}

export function TransferStockDialog({
  isOpen,
  onOpenChange,
  branchId,
  products,
  currentInventory,
  onTransferSuccess,
}: TransferStockDialogProps) {
  const [isLoading, setIsLoading] = useState(false);
  const [selectedProduct, setSelectedProduct] = useState<string>("");
  const [fromType, setFromType] = useState<"real" | "ghost" | "">("");

  const form = useForm<TransferStockFormData>({
    resolver: zodResolver(transferStockSchema),
    defaultValues: {
      productId: "",
      fromType: undefined,
      toType: undefined,
      quantity: 1,
      reason: "",
    },
  });

  const getAvailableStock = () => {
    if (!selectedProduct || !fromType) return 0;
    return currentInventory[selectedProduct]?.[fromType as "real" | "ghost"] ?? 0;
  };

  const handleSubmit = async (data: TransferStockFormData) => {
    setIsLoading(true);
    try {
      const response = await fetch(`/api/inventory/transfer`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          branch_id: branchId,
          product_id: data.productId,
          from_type: data.fromType,
          to_type: data.toType,
          quantity: data.quantity,
          reason: data.reason,
        }),
      });

      if (!response.ok) {
        const error = await response.json();
        form.setError("reason", {
          message: error.error || "Transfer failed",
        });
        return;
      }

      // Success!
      onOpenChange(false);
      onTransferSuccess?.();
      form.reset();
    } catch (error) {
      form.setError("reason", {
        message: error instanceof Error ? error.message : "Network error",
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Transfer Stock (Real ↔ Ghost)</DialogTitle>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            {/* Product Selection */}
            <FormField
              control={form.control}
              name="productId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Product</FormLabel>
                  <FormControl>
                    <Select
                      value={field.value}
                      onValueChange={(value) => {
                        field.onChange(value);
                        setSelectedProduct(value);
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select product" />
                      </SelectTrigger>
                      <SelectContent>
                        {products.map((product) => (
                          <SelectItem key={product.id} value={product.id}>
                            {product.name} ({product.sku})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* From Type */}
            <FormField
              control={form.control}
              name="fromType"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>From (Source)</FormLabel>
                  <FormControl>
                    <Select
                      value={field.value || ""}
                      onValueChange={(value) => {
                        field.onChange(value as "real" | "ghost");
                        setFromType(value as "real" | "ghost");
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select source" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="real">
                          Real Stock ({getAvailableStock()})
                        </SelectItem>
                        <SelectItem value="ghost">
                          Ghost Stock ({getAvailableStock()})
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* To Type */}
            <FormField
              control={form.control}
              name="toType"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>To (Destination)</FormLabel>
                  <FormControl>
                    <Select value={field.value || ""} onValueChange={field.onChange}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select destination" />
                      </SelectTrigger>
                      <SelectContent>
                        {fromType && (
                          <SelectItem value={fromType === "real" ? "ghost" : "real"}>
                            {fromType === "real" ? "Ghost Stock" : "Real Stock"}
                          </SelectItem>
                        )}
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Quantity */}
            <FormField
              control={form.control}
              name="quantity"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Quantity (Available: {getAvailableStock()})
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      min={1}
                      max={getAvailableStock()}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value, 10))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Reason */}
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Reason for Transfer</FormLabel>
                  <FormControl>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger>
                        <SelectValue placeholder="Select reason" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="cash_sale">
                          Cash Sale (Real → Ghost)
                        </SelectItem>
                        <SelectItem value="payment_received">
                          Payment Received (Ghost → Real)
                        </SelectItem>
                        <SelectItem value="reconciliation">
                          Inventory Reconciliation
                        </SelectItem>
                        <SelectItem value="adjustment">
                          Stock Adjustment
                        </SelectItem>
                        <SelectItem value="write_off">Write-off</SelectItem>
                        <SelectItem value="other">Other</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Submit & Cancel */}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isLoading}>
                {isLoading ? "Transferring..." : "Transfer Stock"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// ============================================================================
// USAGE EXAMPLE IN A PARENT COMPONENT
// ============================================================================

/*
import { useState } from "react";
import { TransferStockDialog } from "@/components/inventory/transfer-stock.component";

export function InventoryPage() {
  const [transferDialogOpen, setTransferDialogOpen] = useState(false);

  const currentInventory = {
    "product-123": { real: 100, ghost: 50 },
    "product-456": { real: 200, ghost: 0 },
  };

  const products = [
    { id: "product-123", name: "Paracetamol 500mg", sku: "PARA-500" },
    { id: "product-456", name: "Aspirin 100mg", sku: "ASP-100" },
  ];

  return (
    <>
      <button onClick={() => setTransferDialogOpen(true)}>
        Transfer Stock
      </button>

      <TransferStockDialog
        isOpen={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        branchId="branch-001"
        products={products}
        currentInventory={currentInventory}
        onTransferSuccess={() => {
          // Refresh inventory data
          console.log("Transfer successful!");
        }}
      />
    </>
  );
}
*/
