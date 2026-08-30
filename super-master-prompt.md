```
# MASTER PROMPT: Month-End Reconciliation System with Full Audit Trail

## CONTEXT
You are working on an existing Go + Next.js ERP system. The system has:
- Multi-branch inventory (Real Stock + Ghost Stock)
- Role-based access: Superadmin, Central Admin, Branch Admin, POS
- Invoice management with soft-delete capability
- Month-end reconciliation process

## OBJECTIVE
Implement/modify the month-end reconciliation system to match the following specification exactly. After implementation, you MUST run self-verification and provide proof that all requirements are met.

---

## PART 1: DATA MODEL REQUIREMENTS

### 1.1 Stock Structure
- **Real Stock**: Visible to all roles. Tracks physical inventory.
- **Ghost Stock**: **Hidden from all non-Superadmin roles**. Tracks "off-book" inventory.
- Each branch (including Warehouse) has BOTH Real and Ghost quantities.
- Ghost Stock is NOT displayed in any Admin/PO view.

### 1.2 Invoice Structure
Each invoice must store:
- `invoice_number` (displayed to users)
- `original_number` (original sequence before renumbering) - **NEW FIELD**
- `created_at` (actual creation timestamp - NEVER modified)
- `payment_method`: cash / transfer / mixed
- `request_full_tax_invoice`: boolean
- `deleted_at` (soft-delete timestamp)
- `hidden_by` (user ID who performed soft-delete)

### 1.3 New Tables
Create these tables if they don't exist:

**reconciliation_logs**:
```
- id (UUID)
- branch_id (UUID)
- period (string, e.g., "2026-01")
- invoice_id (UUID, reference to soft-deleted invoice)
- original_invoice_number (string)
- new_invoice_number (string, NULL if deleted)
- real_stock_deducted (int)
- ghost_stock_deducted (int)
- adjustment_reason (string: "RETURN_TO_WAREHOUSE" | "RENUMBER_ONLY")
- performed_by (UUID, Superadmin)
- created_at (timestamp)
```

**stock_adjustment_notes**:
```
- id (UUID)
- branch_id (UUID)
- product_id (UUID)
- quantity (int, negative for deduction)
- reason (string)
- reference_invoice_id (UUID, NULL if not applicable)
- created_by (UUID, Superadmin)
- created_at (timestamp)
```

---

## PART 2: MONTH-END RECONCILIATION PROCESS (Superadmin Only)

### Step A: Filtering
- Select all invoices for the chosen branch(es) where:
  - `payment_method = 'cash'`
  - `request_full_tax_invoice = false`
  - `deleted_at IS NULL` (not already processed)

### Step B: Ghost Stock Deduction
- For each selected invoice:
  - Deduct the quantity from the branch's **Ghost Stock**
  - Create entry in `reconciliation_logs` with `ghost_stock_deducted = quantity`
- **IMPORTANT**: Admin users MUST NOT see this deduction anywhere.

### Step C: Soft Delete Invoices
- Mark each selected invoice as `deleted_at = NOW()`
- Set `hidden_by = Superadmin ID`
- **CRITICAL**: Do NOT hard-delete. Keep records for Superadmin audit.

### Step D: Re-number Remaining Invoices (WITH Timestamp Protection)
- Fetch all remaining invoices for the branch (where `deleted_at IS NULL`)
- Order by `created_at` (original chronological order)
- Assign new sequential `invoice_number` values starting from 1
- **RULE**: Invoices with `request_full_tax_invoice = true` MUST keep their original `invoice_number` position (they become anchors)
- For each renumbered invoice, update:
  - `invoice_number` (new sequential value)
  - `original_number` (preserve original value - **DO NOT MODIFY**)
- **CRITICAL**: DO NOT modify `created_at` timestamp. Keep original creation time.
- Log all renumbering in `reconciliation_logs` with `new_invoice_number`

#### Example (for branch MES):
```
Before:
INV-001 (created 09:00, tax=true)  -> Keep as INV-001
INV-002 (created 09:30, cash, deleted) -> REMOVED
INV-003 (created 10:00, tax=false) -> Become INV-002
INV-004 (created 10:30, tax=true)  -> Keep as INV-003 (anchor)

After:
INV-001 (created 09:00, tax=true)  -> original=001, display=001
INV-002 (created 10:00, tax=false) -> original=003, display=002
INV-003 (created 10:30, tax=true)  -> original=004, display=003
```

### Step E: Real Stock Correction (The "Return to Warehouse" Fix)
To make physical stock match the system for Branch Admins:
- For each soft-deleted invoice, create a **"Return to Warehouse"** adjustment:
  - Create record in `stock_adjustment_notes`:
    - `branch_id` = the branch where sale occurred
    - `quantity` = -1 (deduction from branch Real Stock)
    - `reason` = "PRODUCT_RETURN_TO_WAREHOUSE"
    - `reference_invoice_id` = soft-deleted invoice ID
  - Deduct from branch's **Real Stock** (so branch stock aligns with physical inventory)
- For the Warehouse:
  - Add +1 to Warehouse's **Real Stock**
  - **BUT** immediately deduct -1 from Warehouse's **Ghost Stock** (so warehouse Real Stock returns to original value)
- **Net effect**: Branch Real Stock decreases, Warehouse Ghost Stock decreases, Warehouse Real Stock unchanged

### Step F: Audit Trail (Superadmin Only)
- All steps (A-E) must be wrapped in a single database transaction
- All actions must be recorded in `reconciliation_logs`
- Superadmin must have a dedicated **"Reconciliation Report"** menu showing:
  - Before/After invoice list
  - Ghost stock deductions
  - Real stock adjustments
  - Original vs new invoice numbers
  - Deleted invoices list (with original numbers)

---

## PART 3: VISIBILITY RULES (Fixing Vulnerabilities)

### 3.1 Admin Views (Branch/Central)
- **Invoice list**: Show ONLY `deleted_at IS NULL` invoices
- **Invoice numbers**: Show only the `invoice_number` field (renumbered version)
- **Timestamp**: Show **DATE ONLY** (not time) to prevent timestamp mismatch suspicion
- **Stock view**: Show ONLY `Real Stock` (never show Ghost Stock)
- **Reconciliation menu**: Completely hidden (no access to reconciliation logs)

### 3.2 POS Views
- Same as Admin, but restricted to their own branch only
- Cannot see total stock quantities (only product names)

### 3.3 Superadmin Views
- See everything (including `deleted_at` and Ghost Stock)
- See both `invoice_number` and `original_number` in audit view
- See full timestamps (for audit purposes)
- Have access to Reconciliation Report menu

---

## PART 4: FIX FOR "NO PHYSICAL RETURN" VULNERABILITY

### Fix Implementation:
- When creating the "Return to Warehouse" adjustment (Step E):
  - The adjustment reason must be set to `"PRODUCT_RETURN_TO_WAREHOUSE"`
  - **Do NOT** create any "Customer Return Receipt" documents
  - The system must treat this as an **internal inventory transfer**, not a customer return
- Branch Admin sees: "Product X returned to Warehouse" without any customer reference
- Warehouse Admin sees: "Product X received from Branch" (as a normal transfer)
- **Physical inventory**: The returned item physically exists at the Warehouse (you have real stock there)
- **Explanation (if audited)**: "Branch requested a different model, so we swapped inventory. The old model was sent back to the warehouse for redistribution."

### Code Implementation:
- In `stock_adjustment_notes`, reason field must accept `"PRODUCT_RETURN_TO_WAREHOUSE"`
- The API should NOT create any customer-facing documents (no Debit Note, no Return Receipt)
- This is purely an internal inventory transfer with a stock reason code

---

## PART 5: FIX FOR "TIMESTAMP MISMATCH" VULNERABILITY

### Fix Implementation:
- **NEVER modify** `created_at` or `updated_at` timestamps during renumbering
- When renumbering (Step D):
  - Only update `invoice_number` and `original_number` fields
  - Keep original `created_at` values
- For Admin views (non-Superadmin):
  - Display **only the date portion** (YYYY-MM-DD) in invoice lists
  - **Do NOT display** the time (HH:MM:SS) to prevent users from noticing chronological inconsistencies

### Code Implementation:
- API response for Admin roles should format `created_at` as `"2026-01-15"` only
- API response for Superadmin should show full timestamp `"2026-01-15 14:30:00"`

---

## PART 6: FIX FOR "GHOST STOCK AS ANOTHER BRANCH" VULNERABILITY

### Fix Implementation:
- Ghost Stock is stored per branch (not as a separate branch entity)
- Warehouse has both Real and Ghost counts in the same table
- When audited, you can explain Ghost Stock as:
  - "Stock allocated for special projects (government contracts)"
  - "Stock held for future replenishment"
  - "Stock in transit between warehouses"
- To support this, add a `stock_type` field to `stock_adjustment_notes`:
  - `"REAL"` for visible stock adjustments
  - `"GHOST"` for hidden stock adjustments
- The Warehouse Admin should see `stock_type = "REAL"` adjustments only
- Superadmin can see both

### Code Implementation:
- Add `stock_type` VARCHAR(10) to `stock_adjustment_notes` table
- Set `stock_type = "REAL"` for all adjustments visible to Admin
- Set `stock_type = "GHOST"` for all adjustments hidden from Admin
- API filtering: if role != Superadmin, exclude `stock_type = "GHOST"` records

---

## PART 7: TESTING & VALIDATION REQUIREMENTS

After implementation, you MUST run these tests and **PROVIDE PROOF** that each passes:

### Test 1: Complete Month-End Flow (Branch A)
Scenario:
- WH: Real=100, Ghost=20
- Branch A: Real=5, Ghost=0
- Sales: 3 invoices (1 cash-only without full tax invoice)
  - INV-001: Bed, 1pc, Transfer
  - INV-002: Bed, 1pc, Cash (no tax invoice)
  - INV-003: Bed, 1pc, Cash+Transfer

**Expected After Reconciliation**:
| Item | Before | After |
|------|--------|-------|
| Branch A Real Stock | 5 | 2 (5 - 2 normal sales - 1 return) |
| Branch A Ghost Stock | 0 | -1 (deducted from INV-002) |
| Warehouse Real Stock | 100 | 100 (101 - 1 ghost) |
| Warehouse Ghost Stock | 20 | 19 (20 - 1) |
| Invoices visible to Admin | 3 | 2 (INV-002 hidden) |
| Invoice numbers visible | 001, 002, 003 | 001, 002 (renumbered) |
| Timestamp mismatch? | - | N/A (dates shown, times hidden) |

**Provide evidence**: Screenshot or API response showing:
- Branch Admin sees only 2 invoices, numbers 001 and 002
- Branch Admin sees Real Stock = 2
- Branch Admin sees returned item in adjustment notes with reason "PRODUCT_RETURN_TO_WAREHOUSE"
- Warehouse Admin sees Real Stock = 100 (unchanged)
- Superadmin sees all 3 invoices in audit log with original numbers

### Test 2: Timestamp Protection
- Verify `created_at` timestamps were NOT modified for renumbered invoices
- Superadmin API must return full timestamp
- Admin API must return only date (YYYY-MM-DD)

### Test 3: Permission Enforcement
- Central Admin tries to view Ghost Stock → should get 403 Forbidden
- Branch Admin tries to view Reconciliation menu → should not exist
- POS user tries to access Superadmin API → 403 Forbidden

### Test 4: Physical Stock Correction (Return to Warehouse)
- Verify the "Return to Warehouse" adjustment exists in `stock_adjustment_notes`
- Verify reason = `"PRODUCT_RETURN_TO_WAREHOUSE"`
- Verify Branch Real Stock decreased by 1
- Verify Warehouse Real Stock remained the same (because Ghost compensated)

### Test 5: Ghost Stock Depletion Tracking
- After running the process 3 times (monthly), Ghost Stock in WH should decrease each time
- Superadmin must be able to view remaining Ghost Stock
- System must display remaining Ghost Stock in Superadmin dashboard

---

## PART 8: DELIVERABLES

You must deliver:
1. All database migration scripts
2. Updated Backend APIs (Go)
3. Updated Frontend pages (Next.js)
4. API test results (curl or Postman output) proving all Test requirements above
5. A **README.md** documenting how to run the month-end reconciliation process

---

## FINAL INSTRUCTION

**After implementing and testing, you MUST provide a summary report showing that ALL Test cases (1-5) have been executed and passed. Include evidence (API responses, database queries, screenshots in text format) that confirm the system behaves exactly as specified in this prompt.**

Do not add explanations about why something is done. Just implement the code and prove it works.
```