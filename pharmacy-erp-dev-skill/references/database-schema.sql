-- Pharmacy ERP Database Schema Extensions
-- For dual inventory (real + ghost stock), government purchases, and invoice management

-- ============================================================================
-- INVENTORY SYSTEM: Real vs Ghost Stock
-- ============================================================================

CREATE TABLE inventory_real (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),
  product_id UUID NOT NULL REFERENCES products(id),
  quantity_in_stock INT NOT NULL DEFAULT 0,
  reorder_level INT,
  reorder_quantity INT,
  cost_per_unit DECIMAL(10, 2),
  last_received_date TIMESTAMP,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),

  CONSTRAINT unique_real_stock UNIQUE(branch_id, product_id)
);

CREATE TABLE inventory_ghost (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),
  product_id UUID NOT NULL REFERENCES products(id),
  quantity_in_stock INT NOT NULL DEFAULT 0,
  payment_method TEXT, -- "cash_only", etc.
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),

  CONSTRAINT unique_ghost_stock UNIQUE(branch_id, product_id)
);

-- ============================================================================
-- STOCK TRANSFERS: Between Real and Ghost
-- ============================================================================

CREATE TABLE stock_transfers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),
  product_id UUID NOT NULL REFERENCES products(id),

  from_type TEXT NOT NULL, -- 'real' or 'ghost'
  to_type TEXT NOT NULL,   -- 'real' or 'ghost'
  quantity INT NOT NULL,

  reason TEXT, -- 'cash_sale', 'customer_return', 'reconciliation', etc.
  transfer_status TEXT DEFAULT 'completed', -- 'pending', 'completed', 'rejected'

  initiated_by UUID NOT NULL REFERENCES app_user(id),
  initiated_at TIMESTAMP DEFAULT NOW(),
  completed_at TIMESTAMP,

  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- PRICING: Dynamic prices for cash sales
-- ============================================================================

CREATE TABLE price_adjustments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id UUID NOT NULL,
  product_id UUID NOT NULL REFERENCES products(id),

  base_price DECIMAL(10, 2) NOT NULL,
  adjusted_price DECIMAL(10, 2) NOT NULL,
  adjustment_reason TEXT, -- 'cash_discount', 'bulk_order', 'promotional', etc.

  adjusted_by UUID NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMP DEFAULT NOW(),

  INDEX idx_invoice_price (invoice_id)
);

CREATE TABLE branch_pricing_tiers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),
  product_id UUID NOT NULL REFERENCES products(id),

  tier_name TEXT, -- 'standard', 'cash_discount', 'bulk', etc.
  price DECIMAL(10, 2) NOT NULL,
  min_quantity INT, -- minimum order quantity for this tier

  effective_from TIMESTAMP DEFAULT NOW(),
  effective_to TIMESTAMP,
  is_active BOOLEAN DEFAULT TRUE,

  created_at TIMESTAMP DEFAULT NOW(),

  CONSTRAINT unique_branch_product_tier UNIQUE(branch_id, product_id, tier_name)
);

-- ============================================================================
-- GOVERNMENT PURCHASES (รพสต. Mode)
-- ============================================================================

CREATE TABLE government_entities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_name TEXT NOT NULL,
  entity_code TEXT, -- government agency code
  contact_person TEXT,
  contact_phone TEXT,
  contact_email TEXT,
  billing_address TEXT,
  tax_id TEXT,

  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE government_quotations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  government_id UUID NOT NULL REFERENCES government_entities(id),
  branch_id UUID NOT NULL REFERENCES branches(id),

  -- What they requested
  requested_products JSONB NOT NULL, -- [{"product_id": "...", "quantity": 100}, ...]

  -- What we propose (may differ for pricing optimization)
  proposed_products JSONB NOT NULL, -- [{"product_id": "...", "quantity": 100, "price": 50}, ...]

  quotation_status TEXT DEFAULT 'draft', -- 'draft', 'sent', 'accepted', 'rejected', 'ordered'
  quotation_date TIMESTAMP DEFAULT NOW(),
  expiration_date TIMESTAMP,

  notes TEXT,
  created_by UUID NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE government_purchase_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  quotation_id UUID NOT NULL REFERENCES government_quotations(id),
  government_id UUID NOT NULL REFERENCES government_entities(id),

  po_number TEXT NOT NULL, -- government PO reference
  po_date TIMESTAMP,

  order_status TEXT DEFAULT 'received', -- 'received', 'acknowledged', 'fulfilled', 'invoiced'

  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- INVOICE SYSTEM: Simplified + Full with Sequence Locking
-- ============================================================================

CREATE TABLE invoice_sequences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),

  prefix TEXT NOT NULL, -- 'INV', 'QT', 'PO', etc.
  sequence_type TEXT, -- 'invoice', 'quotation', 'po', 'receipt'

  current_number INT NOT NULL DEFAULT 1,
  last_issued_date TIMESTAMP,

  format_pattern TEXT, -- 'PREFIXYYYYMMDDNNNNN' or custom format
  reset_frequency TEXT, -- 'daily', 'monthly', 'annual', 'none'

  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),

  CONSTRAINT unique_sequence UNIQUE(branch_id, prefix, sequence_type)
);

CREATE TABLE invoices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),

  -- Invoice identification
  invoice_number TEXT NOT NULL UNIQUE, -- e.g., "INV20250711000001"
  invoice_date TIMESTAMP DEFAULT NOW(),
  invoice_type TEXT, -- 'standard', 'simplified', 'government', 'government_simplified'

  -- Customer info
  customer_id UUID,
  customer_name TEXT,
  customer_tax_id TEXT,
  customer_address TEXT,

  -- Government-specific
  government_id UUID REFERENCES government_entities(id),
  po_reference TEXT,

  -- Financial data
  subtotal DECIMAL(12, 2) NOT NULL,
  vat_amount DECIMAL(12, 2) DEFAULT 0,
  vat_percentage INT DEFAULT 7,
  total_amount DECIMAL(12, 2) NOT NULL,

  -- Payment & Status
  payment_status TEXT DEFAULT 'unpaid', -- 'unpaid', 'partial', 'paid', 'cancelled'
  payment_method TEXT, -- 'cash', 'check', 'bank_transfer', 'credit'
  invoice_status TEXT DEFAULT 'locked', -- 'draft', 'locked', 'cancelled', 'voided'

  -- Audit
  created_by UUID NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMP DEFAULT NOW(),
  printed_at TIMESTAMP,
  reprinted_at TIMESTAMP,
  locked_at TIMESTAMP,

  updated_at TIMESTAMP DEFAULT NOW(),

  INDEX idx_invoice_number (invoice_number),
  INDEX idx_invoice_status (invoice_status),
  INDEX idx_invoice_date (invoice_date)
);

CREATE TABLE invoice_line_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  product_id UUID NOT NULL REFERENCES products(id),

  product_name TEXT NOT NULL,
  product_sku TEXT,

  quantity INT NOT NULL,
  unit_price DECIMAL(10, 2) NOT NULL,
  line_total DECIMAL(12, 2) NOT NULL, -- quantity * unit_price

  -- If price was overridden
  base_price DECIMAL(10, 2),
  price_override_reason TEXT,

  created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE invoice_payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id UUID NOT NULL REFERENCES invoices(id),

  payment_date TIMESTAMP DEFAULT NOW(),
  payment_amount DECIMAL(12, 2) NOT NULL,
  payment_method TEXT, -- 'cash', 'check', 'bank_transfer', etc.

  reference_number TEXT, -- check number, bank transfer reference, etc.
  notes TEXT,

  recorded_by UUID NOT NULL REFERENCES app_user(id),
  created_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- AUDIT LOGS: All financial transactions
-- ============================================================================

CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  entity_type TEXT NOT NULL, -- 'invoice', 'stock_transfer', 'payment', 'price_adjustment'
  entity_id UUID NOT NULL,

  action TEXT NOT NULL, -- 'created', 'updated', 'locked', 'voided', 'printed'

  user_id UUID NOT NULL REFERENCES app_user(id),
  user_role TEXT,

  changes JSONB, -- what changed: {field: {old_value, new_value}}
  reason TEXT,

  ip_address INET,
  user_agent TEXT,

  created_at TIMESTAMP DEFAULT NOW(),

  INDEX idx_entity (entity_type, entity_id),
  INDEX idx_user (user_id),
  INDEX idx_action (action),
  INDEX idx_created_at (created_at)
);

-- ============================================================================
-- INDEXES for Performance
-- ============================================================================

CREATE INDEX idx_inventory_real_branch_product ON inventory_real(branch_id, product_id);
CREATE INDEX idx_inventory_ghost_branch_product ON inventory_ghost(branch_id, product_id);
CREATE INDEX idx_stock_transfers_branch ON stock_transfers(branch_id, created_at DESC);
CREATE INDEX idx_stock_transfers_status ON stock_transfers(transfer_status);
CREATE INDEX idx_price_adjustments_invoice ON price_adjustments(invoice_id);
CREATE INDEX idx_branch_pricing_branch ON branch_pricing_tiers(branch_id, is_active);
CREATE INDEX idx_gov_quotations_status ON government_quotations(quotation_status);
CREATE INDEX idx_gov_po_status ON government_purchase_orders(order_status);
CREATE INDEX idx_invoices_date_branch ON invoices(branch_id, invoice_date DESC);
CREATE INDEX idx_invoice_status_date ON invoices(invoice_status, invoice_date DESC);
