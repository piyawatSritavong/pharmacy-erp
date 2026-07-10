# Pharmacy ERP Monorepo

Backend-centric pharmacy and medical equipment ERP built as a 3-service monorepo.

## Stack

- Backend: Go 1.25 + Echo + PostgreSQL
- Frontend: Next.js App Router + Tailwind CSS + shared shadcn/Radix component base
- Auth: JWT in HttpOnly cookie
- Deploy: Docker Compose

## Structure

```text
/backend
  /cmd
  /configs
  /internal
  /migrations
  /tests
/frontend
  /src/app
  /src/components
  /src/services
/deploy
  docker-compose.yml
  backend.Dockerfile
  frontend.Dockerfile
```

## Backend-Centric Rules

- Frontend only submits data and renders backend responses.
- All pricing, VAT, stock deduction, sequence generation, payment matching, and audit decisions happen in Go.
- No Local Storage or client-side persistence is used for important data.
- Every mutation is intended to flow through the backend so audit logs remain complete.

## Seeded Accounts

- `superadmin@erp.local` / `DevPassword123!`
- `branchadmin@erp.local` / `DevPassword123!`
- `pos@erp.local` / `DevPassword123!`

Seed data also creates:

- 2 branches: `มนัสการแพทย์`, `คณาเภสัช`
- 3 roles and RBAC permissions
- Products, alias mapping, branch prices, real/ghost inventory
- Draft quotation, unpaid and paid invoices, daily POS sales data, pending check
- In-transit transfer for POS receipt and requested transfer for branch dispatch
- Provider-ready marketplace connection and order inbox

## Role Navigation

- `Super Admin`
  - `/dashboard`
  - `/inventory-management`
  - `/installments`
  - `/finance-central`
  - `/global-reports`
  - `/settings`
- `Branch Admin`
  - `/branch-dashboard`
  - `/branch-inventory`
  - `/sales-invoices`
  - `/installments`
  - `/local-finance`
- `Branch POS`
  - `/sales`
  - `/inventory-check`
  - `/installments`
  - `/transfer-receipts`
  - `/daily-sales`

## Added Flows

- Installment billing (ported from DockBill): split an unpaid invoice into monthly installments (`installment_plans` / `installment_payments`), collect per due date into `invoice_payments`, overdue tracking, and automatic invoice settlement when the final installment is paid
- 3-tier pricing (ported from DockBill): `cash` (base/branch price), `retail_price`, and `installment_price` per product; sales lines carry `price_tier` with precedence override > government alias > tier > branch/base
- Stock receiving (ported from DockBill): `POST /inventory/receive` books one shipment into real and ghost buckets in a single transaction with movements and audit
- Fixed branch-scoped document numbering with `PREFIXYYYYMMDDNNNNN`, non-reset running numbers, and lock semantics that block edits without blocking issuance
- Invoice print / reprint via `/print/invoices/:invoiceID`
- QR-based transfer receipt with local SVG QR rendering, browser camera scan, and manual transfer-code fallback
- Settings CRUD for branches, users, roles, permissions, and document sequences
- Marketplace config and audit logs moved under `Settings`
- Legacy `/transfers` route now redirects to `/branch-inventory?tab=transfers`
- Shared UI primitives now use Radix-backed `Select`, `Checkbox`, `Tabs`, `Dialog`, `Sheet`, and `DropdownMenu` wrappers while keeping the frontend as a thin API client

## Local Backend

```bash
cd backend
cp configs/app.example.env .env
go run ./cmd/api serve
```

Other backend commands:

```bash
go run ./cmd/api migrate
go run ./cmd/api seed
```

## Docker Compose

```bash
cd deploy
docker compose up --build
```

This starts PostgreSQL, the Go API, and the Next.js frontend. The backend auto-runs migrations and seed data before serving traffic.

## Tests

Backend unit + harness tests:

```bash
cd backend
go test ./...
```

Optional integration DB run:

```bash
TEST_DATABASE_URL=postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable go test ./tests -run TestIntegrationHarness
```

Frontend Playwright scaffold:

```bash
cd frontend
E2E_RUN=1 npm run e2e
```

## Notes

- Docker daemon was not available in the current implementation environment, so `docker compose up --build` was prepared but not executed here.
- The frontend is designed as a thin proxy/client around `/api/backend/*`, which forwards every request to the Go backend and stores JWT in an HttpOnly cookie.
