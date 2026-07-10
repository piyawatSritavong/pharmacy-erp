# Pharmacy ERP Development Quick Start

## Getting Started with Your Dual Inventory System

### 1. Database Setup (First Time Only)

```bash
# 1. Copy the schema file to your backend migrations
cp references/database-schema.sql backend/migrations/20250711000000_dual_inventory_system.sql

# 2. Run migrations
cd backend
go run ./cmd/api migrate

# 3. Verify tables created
psql -U pharmacy -d pharmacy_erp -c "\dt"
```

### 2. Backend: Add Handlers

```bash
# Copy the example handlers
cp references/inventory-handlers-example.go backend/internal/handlers/inventory.go

# Update your router to register the handlers
# In backend/cmd/api/main.go or wherever you setup routes:
```

```go
// Register inventory endpoints
api := e.Group("/api/inventory", middleware.JWTAuth())
api.POST("/transfer", h.HandleTransferStock)      // Transfer real <-> ghost
api.GET("/branches/:branch_id", h.HandleGetInventory)  // Get inventory by role
api.POST("/price-adjustment", h.HandleRecordPriceAdjustment)
```

### 3. Frontend: Add Components

```bash
# Copy React component for stock transfers
cp references/transfer-stock.component.tsx frontend/src/components/inventory/transfer-stock.component.tsx

# Install component in your page
```

```tsx
// In frontend/src/app/admin/inventory/page.tsx
import { TransferStockDialog } from "@/components/inventory/transfer-stock.component";

export default function InventoryPage() {
  const [transferOpen, setTransferOpen] = useState(false);

  return (
    <>
      <button onClick={() => setTransferOpen(true)}>
        Transfer Stock
      </button>

      <TransferStockDialog
        isOpen={transferOpen}
        onOpenChange={setTransferOpen}
        branchId={branchId}
        products={products}
        currentInventory={currentInventory}
        onTransferSuccess={() => refetchInventory()}
      />
    </>
  );
}
```

### 4. Test Locally

```bash
# Terminal 1: Backend
cd backend
go run ./cmd/api serve

# Terminal 2: Frontend
cd frontend
npm run dev

# Terminal 3: Test with curl
curl -X POST http://localhost:8000/api/inventory/transfer \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "branch_id": "branch-001",
    "product_id": "product-123",
    "from_type": "real",
    "to_type": "ghost",
    "quantity": 30,
    "reason": "cash_sale"
  }'
```

### 5. Verify in Database

```bash
# Check stock transfer was recorded
psql -U pharmacy -d pharmacy_erp -c "SELECT * FROM stock_transfers ORDER BY created_at DESC LIMIT 1;"

# Check audit log
psql -U pharmacy -d pharmacy_erp -c "SELECT * FROM audit_logs WHERE entity_type = 'stock_transfer' ORDER BY created_at DESC LIMIT 1;"

# Check inventory updates
psql -U pharmacy -d pharmacy_erp -c "SELECT * FROM inventory_real WHERE product_id = 'product-123';"
psql -U pharmacy -d pharmacy_erp -c "SELECT * FROM inventory_ghost WHERE product_id = 'product-123';"
```

---

## Common Development Tasks

### Task: Add Government Purchase Mode

```bash
# 1. Tables already in schema (government_quotations, government_purchase_orders)

# 2. Create handlers
# backend/internal/handlers/government.go
func (h *Handler) HandleCreateGovernmentQuotation(c echo.Context) error {
  // Implement quotation creation
}

# 3. Register route
api.POST("/government-quotations", h.HandleCreateGovernmentQuotation)

# 4. Create React component
# frontend/src/components/government/quotation-form.component.tsx

# 5. Add to admin portal
# frontend/src/app/admin/government-purchases/page.tsx
```

### Task: Implement Invoice Locking

```bash
# The schema already includes:
# - invoice_sequences: Auto-increment with locking
# - invoices.invoice_status: 'draft' → 'locked'

# Create handler:
func (h *Handler) HandleLockInvoice(c echo.Context) error {
  invoiceID := c.Param("invoice_id")
  
  // 1. Fetch invoice
  // 2. Verify it's ready to lock (all line items present)
  // 3. Get next invoice sequence number
  // 4. Format invoice number (PREFIXYYYYMMDDNNNNN)
  // 5. Update invoice status to 'locked'
  // 6. Log to audit_logs
}
```

### Task: Manager Portal (Real Stock Only)

```tsx
// Create separate route group
frontend/src/app/(manager)/

// In layout, apply role-based access check
export default async function ManagerLayout() {
  const session = await auth();
  if (session?.user?.role !== 'manager') {
    redirect('/unauthorized');
  }
  
  // Return layout with manager-specific sidebar
  return <ManagerSidebar>{children}</ManagerSidebar>;
}

// Backend automatically filters inventory:
// If user role = "manager" → only return real_stock
// If user role = "superadmin" → return real_stock + ghost_stock
```

---

## Troubleshooting

### Issue: "Transfer failed - Insufficient stock"
- Check `currentInventory` is being passed correctly to component
- Verify backend is returning accurate stock numbers
- Check audit logs: `SELECT * FROM audit_logs WHERE entity_type = 'stock_transfer' LIMIT 10;`

### Issue: "Inventory numbers don't match between real and ghost"
- Check for failed transactions (use transaction rollback)
- Verify inventory records exist in both tables
- Run reconciliation: Count all transfers and verify math

### Issue: Invoice number not showing up
- Check `invoice_sequences` table has entries for your branch
- Verify invoice.status is 'locked' (not 'draft')
- Check that you called `HandleLockInvoice` endpoint

---

## File Structure After Setup

```
pharmacy-erp-main/
├── backend/
│   ├── migrations/
│   │   └── 20250711000000_dual_inventory_system.sql  ← Added
│   └── internal/handlers/
│       ├── inventory.go  ← Added (transfer, get, price adjustment)
│       └── government.go ← Add later
│
├── frontend/
│   └── src/
│       ├── components/inventory/
│       │   └── transfer-stock.component.tsx  ← Added
│       └── app/
│           ├── (admin)/inventory/page.tsx
│           └── (manager)/inventory/page.tsx
│
└── pharmacy-erp-dev-skill/  ← This skill
    ├── SKILL.md
    ├── QUICKSTART.md
    └── references/
        ├── database-schema.sql
        ├── inventory-handlers-example.go
        └── transfer-stock.component.tsx
```

---

## Next Steps

1. **Test locally** → Run full stack with docker compose
2. **Implement invoice locking** → Follow SKILL.md Workflow 4
3. **Add government purchase mode** → Follow SKILL.md Workflow 2
4. **Create manager portal** → Follow SKILL.md Workflow 3
5. **Deploy to production** → Update docker-compose.yml, run migrations on prod DB

---

## Resources in This Skill

- **SKILL.md** → Full documentation and patterns
- **references/database-schema.sql** → All tables with comments
- **references/inventory-handlers-example.go** → Backend API implementation
- **references/transfer-stock.component.tsx** → Frontend React component with form validation

Use the skill whenever you need guidance on:
- Dual inventory implementation
- Stock transfer workflows
- Role-based access control
- Government purchase orders
- Invoice generation and locking
- Price modifications
- Audit logging
