---
name: pharmacy-erp-development
description: Build production-ready Pharmacy ERP features with dual inventory (real + ghost stock), role-based UI separation, government purchase mode, and invoice management. Use when implementing inventory management, stock transfers, pricing strategies, government purchases (รพสต.), invoice generation with locked numbering, or integrating DockBill patterns into pharmacy-erp-main. Covers backend schema design (Go), database migrations, API endpoint generation, frontend components, and multi-domain role-based UI separation.
---

# Pharmacy ERP Development Skill

Build production-ready pharmacy and medical equipment ERP systems with sophisticated inventory management, role-based access control, government procurement support, and invoice handling.

## Core Features & Capabilities

### 1. Dual Inventory System (Real + Ghost Stock)
- **Real Stock (Stock จริง)**: Physical inventory with full audit trail
- **Ghost Stock (Stock ผี)**: Virtual inventory for cash-only transactions
- **Bi-directional Transfer**: Move stock between real and ghost seamlessly
- **Example**: Buy 100 units → Real: 70, Ghost: 30

**Use this when:**
- Setting up initial inventory system
- Implementing stock transfer workflows
- Creating inventory reconciliation reports
- Handling cash-only sales separately from standard transactions

### 2. Role-Based UI Separation
- **Manager Portal** (Sub-domain/separate site): View real stock only
- **Admin Portal** (Full access): View both real and ghost stock
- **Dynamic Role Assignment**: RBAC with permission inheritance

**Use this when:**
- Separating access by role (branch admin, superadmin, POS operator)
- Creating manager-facing dashboards (real stock only)
- Implementing audit logs for stock movements

### 3. Price Modification & Cash Sales
- Dynamic pricing for cash transactions
- Override base price with custom rates
- Track pricing changes in audit log
- Branch-level and system-level pricing tiers

**Use this when:**
- Implementing flexible pricing for cash sales
- Creating price adjustment workflows
- Setting up multi-branch pricing strategies

### 4. Government Purchase Mode (รพสต. Mode)
- Special B2B workflow for government hospitals/agencies
- Handle "intentional product mismatches" for pricing optimization
- Separate quotation + order confirmation flow
- Government-specific invoice requirements

**Use this when:**
- Processing large government contracts
- Handling government procurement quotes
- Managing government customer relationships

### 5. Invoice Management System
- **Simple Invoice**: Quick receipt format
- **Full Invoice**: Complete financial document with auto-locked sequence number
- **Invoice Sequence Locking**: Prevents edits that would break invoice numbering
- **Print/Reprint**: Multiple invoice versions and formats
- **Invoice Number Syncing**: Lock number to match system records

**Use this when:**
- Generating invoices (cash, credit, or government)
- Creating invoice print workflows
- Implementing audit-compliant invoice numbering

### 6. Backend-Centric Architecture
All critical operations (pricing, stock deduction, VAT, audit decisions) happen in backend (Go):
- No client-side persistence of financial data
- Immutable audit logs
- Transaction-safe operations
- Complete audit trail

**Use this when:**
- Building API endpoints
- Implementing payment matching
- Creating audit log handlers

---

## Project Structure

### pharmacy-erp-main (Base Project)
```
backend/        → Go + Echo framework
  ├── cmd/api
  ├── internal/
  │   ├── handlers/       (API endpoints)
  │   ├── services/       (business logic)
  │   ├── models/         (data structures)
  │   └── middleware/     (auth, validation)
  ├── migrations/         (SQL migrations)
  └── tests/
  
frontend/       → Next.js + Tailwind
  ├── src/app/           (Route groups: auth, operations, admin)
  ├── src/components/    (shadcn/ui + domain components)
  ├── src/services/      (API client)
  └── public/
  
deploy/         → Docker Compose
```

### DockBill Integration Pattern
```
DockBill Style:
  - Next.js App Router with route groups
  - Drizzle ORM + NextAuth.js
  - shadcn/ui + Radix components
  - Server Actions for mutations
  - Zod validation schemas

→ Apply patterns to pharmacy-erp frontend as needed
```

---

## Development Workflows

### Workflow 1: Add a New Inventory Feature

**Step 1: Database Schema** (Go Backend)
```
Analyze current schema (schema.sql):
  ├── products
  ├── branches
  ├── inventory_real
  ├── inventory_ghost
  ├── stock_transfers
  └── audit_logs
```

**Step 2: Create Migration** (Go)
```
backend/migrations/
  └── YYYYMMDDHHMMSS_add_feature.sql
```

**Step 3: Build API Endpoint** (Go)
```
internal/handlers/inventory.go
  └── HandleTransferStock()
  └── HandleAdjustInventory()
```

**Step 4: Create Frontend Component** (Next.js)
```
frontend/src/components/inventory/
  └── transfer-stock.component.tsx
  └── adjust-inventory.component.tsx
```

**Step 5: Implement Role-Based Access**
```
Manager sees only: real_stock, real_stock_changes
Admin sees both: real_stock + ghost_stock + transfers
```

### Workflow 2: Implement Government Purchase (รพสต. Mode)

**Step 1: Create Government Purchase Schema**
```sql
CREATE TABLE government_quotations (
  id UUID PRIMARY KEY,
  government_id UUID NOT NULL,
  requested_products JSON,  -- What they actually want
  proposed_products JSON,   -- What we suggest (for better pricing)
  quotation_status TEXT,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);
```

**Step 2: Build Government Quote API**
- POST /api/government-quotations (create quote)
- GET /api/government-quotations/:id (view)
- PUT /api/government-quotations/:id (accept/reject)

**Step 3: Create Government Invoice Variant**
- Use same invoice engine but different template
- Government-specific fields (PO number, agency code)
- Compliance metadata

**Step 4: Create Government Portal Components**
```typescript
// Components for government customers
- QuotationRequestForm
- GovernmentQuotationView
- GovernmentPurchaseOrder
- GovernmentInvoice
```

### Workflow 3: Implement Dual Portal Access

**Step 1: Configure Sub-domains**
```
Manager Portal:  manager.pharmacy.local → /branch-dashboard
Admin Portal:    admin.pharmacy.local   → /admin/dashboard
```

**Step 2: Role-Based Route Groups**
```
frontend/app/
  ├── (manager)/           ← subdomain: manager.*
  │   ├── layout.tsx
  │   ├── inventory/page.tsx  (real stock only)
  │   └── sales/page.tsx
  │
  └── (admin)/             ← subdomain: admin.*
      ├── layout.tsx
      ├── inventory/page.tsx  (real + ghost stock)
      ├── transfers/page.tsx
      └── reports/page.tsx
```

**Step 3: Data Filtering by Role**
```go
// Backend: Always apply role-based filtering
func GetInventory(userID string, role string) {
  if role == "manager" {
    return only_real_stock
  }
  if role == "admin" {
    return real_stock + ghost_stock
  }
}
```

### Workflow 4: Invoice Generation & Locking

**Step 1: Invoice Number Sequence**
```sql
CREATE TABLE invoice_sequences (
  id UUID PRIMARY KEY,
  branch_id UUID,
  prefix TEXT,           -- PREFIX (e.g., "INV")
  current_number INT,    -- 00001
  format TEXT,           -- PREFIXYYYYMMDDNNNNN
  created_at TIMESTAMP
);
```

**Step 2: Generate Invoice with Lock**
```go
// backend/internal/handlers/invoice.go
func GenerateInvoice(invoiceData) {
  // 1. Get next sequence number
  seq := getNextSequenceNumber(branch_id)
  
  // 2. Generate invoice number
  invoiceNum := formatInvoiceNumber(seq, prefix, date)
  
  // 3. Create invoice with LOCKED status
  invoice.Status = "locked"  // Can't edit without unlock permission
  
  // 4. Persist to DB with audit log
  saveInvoice(invoice)
  logAudit("invoice_created", invoiceData)
}
```

**Step 3: Frontend Invoice Rendering**
```typescript
// Generate print-ready invoice
- Simple format: Receipt-style (minimal data)
- Full format: Accounting-style (complete data, no edit)
- Lock enforcement: Check status before allowing edits
```

---

## Key Implementation Patterns

### Pattern 1: Audit-Safe Stock Transfers
```go
// All stock changes go through this handler
func TransferStock(source, destination, quantity, reason) {
  tx := db.BeginTx()
  
  // 1. Validate quantities
  validateStock(source, quantity)
  
  // 2. Update both accounts
  debit(source, quantity)
  credit(destination, quantity)
  
  // 3. Log the transfer
  logAudit(StockTransfer{
    from: source,
    to: destination,
    qty: quantity,
    reason: reason,
    timestamp: now(),
    user: currentUser(),
  })
  
  tx.Commit()
}
```

### Pattern 2: Role-Based Query Filtering
```go
// Backend: Centralized access control
func GetInventoryList(userID string, role string) []InventoryItem {
  query := db.Query("SELECT * FROM inventory")
  
  if role == "manager" {
    query.Where("inventory_type = ?", "real")
  }
  if role == "pos_operator" {
    query.Where("branch_id = ?", userBranch)
  }
  
  return query.All()
}
```

### Pattern 3: Price Override Tracking
```go
type PriceAdjustment struct {
  ID              uuid.UUID
  InvoiceID       uuid.UUID
  ProductID       uuid.UUID
  BasePrice       decimal.Decimal
  AdjustedPrice   decimal.Decimal
  AdjustmentReason string
  AdjustedBy      string
  CreatedAt       time.Time
}
```

---

## Integration Checklist

- [ ] **Database**: Merge DockBill schema patterns with pharmacy-erp
- [ ] **Auth**: Extend NextAuth to support multi-domain role separation
- [ ] **Inventory**: Implement real/ghost stock dual system
- [ ] **Pricing**: Add dynamic price modification with audit trail
- [ ] **Government Mode**: Create รพสต. purchase workflows
- [ ] **Invoices**: Implement sequence locking and multi-format generation
- [ ] **UI Separation**: Create manager vs admin portals
- [ ] **API**: Build all endpoints with backend-centric validation
- [ ] **Migrations**: Document all SQL changes
- [ ] **Tests**: Unit + integration tests for critical paths
- [ ] **Deployment**: Update docker-compose.yml for new services

---

## Commands Reference

### Backend (Go)
```bash
# Run backend server
go run ./cmd/api serve

# Run migrations
go run ./cmd/api migrate

# Generate seed data
go run ./cmd/api seed

# Run tests
go test ./...
```

### Frontend (Next.js)
```bash
# Install dependencies
npm install

# Run dev server
npm run dev

# Generate shadcn components
npx shadcn-ui@latest add [component-name]

# Run tests
npm run test

# Build for production
npm run build
```

### Docker
```bash
# Start full stack
docker compose up --build

# Rebuild after changes
docker compose up --build

# View logs
docker compose logs -f
```

### Database (Drizzle ORM style, if integrating DockBill)
```bash
# Generate migrations from schema
pnpm db:generate

# Run pending migrations
pnpm db:migrate

# Drop all (dev only)
pnpm db:drop
```

---

## Production Deployment Notes

1. **Environment Variables**: Ensure all `.env` values are set correctly in production
2. **Database**: Run migrations before deploying new code
3. **Audit Logs**: Enable audit logging for all financial transactions
4. **Invoice Numbering**: Lock invoice sequences to prevent gaps
5. **Role-Based Access**: Test manager portal can't access ghost stock
6. **Backup**: Implement daily database backups

---

## Common Tasks

### Task: Add a New Product Field
1. Update Go models in `backend/internal/models/`
2. Create migration: `backend/migrations/YYYYMMDDHHMMSS_add_field.sql`
3. Update API handler to return new field
4. Update Next.js component to display/edit field
5. Run tests to verify

### Task: Create Government Purchase Quote
1. Create `government_quotations` table (see schema above)
2. Build API endpoint: `POST /api/government-quotations`
3. Create React component: `GovernmentQuotationForm`
4. Add route: `/admin/government-quotes`
5. Test: Submit quote, verify data in DB

### Task: Fix Invoice Number Lock
1. Check `invoice_sequences` table for gaps
2. Verify `invoice.status = "locked"` enforcement
3. Test that locked invoices can't be edited
4. Review audit logs for unlock events

---

## Troubleshooting

**Issue: Stock numbers don't match between real and ghost**
→ Check stock_transfers audit log. Run reconciliation report.

**Issue: Manager portal shows ghost stock**
→ Verify backend role-based filtering. Check query filters in handler.

**Issue: Invoice numbers not sequential**
→ Check transaction isolation level. Verify sequence lock mechanism.

**Issue: Price changes not logged**
→ Enable audit logging middleware. Check if change went through backend API.

**Issue: Government mode not working**
→ Verify government_quotations table exists. Check role permissions for government endpoints.

