# Mobile UX implementation and validation

The Admin and POS interfaces now share mobile sizing, viewport-safe overlays, and accessible navigation. Business calculations, permissions, API contracts, and stock operations are unchanged.

## Navigation and information layout

- Admin: the header opens a scrollable drawer containing the complete permission-filtered navigation tree. Groups expand without navigating. Current pages, Pro labels, account controls, theme switching, and logout remain accessible. Desktop keeps its sidebar, including its collapsed mode.
- POS: sales, parked bills, and history remain in a compact bottom bar; the menu drawer exposes all other permitted screens. The existing tablet/desktop navigation remains at 640px and above. Safe-area padding protects the bottom controls.
- Mobile form controls have at least 44px height; editable text stays at 16px to avoid iOS focus zoom. Smaller card padding, fluid filters, wrapping toolbars, and a compact pager free up content space. Tabs can grow when their labels wrap.
- Dashboard: filters can be expanded on phones while day navigation remains visible. Shared card padding and nested branch spacing are reduced; zero-valued figures retain readable contrast.
- Product catalog, categories, suppliers, and purchase orders use labeled record cards below 640px, retaining every value and action in the same DOM. Comparison/report tables retain horizontal scrolling within their containers.
- Inventory: the mobile stock selector has a bounded scroll area so product details are reachable without passing through every stock row. Audit fields wrap long labels and values.
- POS products use compact thumbnail rows on phones. The cart uses a modal focus boundary on smaller screens and the existing inline panel on wide tills. Quantities, discounts, tax details, and checkout remain available; short carts use only the space they need.
- Payment uses the shared scrollable dialog. Close controls stay accessible, focus returns to the opener, and the VisualViewport API accounts for the software keyboard. Product-search results stay inside mobile forms for native touch scrolling; desktop results retain a positioned popup.

## Results

Validated on 15 September 2026:

- Production build, ESLint, TypeScript, and whitespace checks passed.
- All 10 mobile browser scenarios passed (8 in the full run, then the 2 form scenarios rerun after adjusting their empty-list assertion).
- All 5 selected existing navigation/POS regression tests passed.
- No document overflow in the 638 route/width/browser checks.

Review screenshots: [Admin menu](mobile-ux/navigation-320.png), [dashboard](mobile-ux/dashboard-390.png), [dark dashboard](mobile-ux/dashboard-dark-390.png), [catalog](mobile-ux/product-catalog-390.png), [purchase orders](mobile-ux/purchase-orders-390.png), [POS](mobile-ux/pos-390.png), [short checkout dialog](mobile-ux/checkout-320.png).

## Coverage

Browser automation uses the local seeded Go/PostgreSQL backend and a production Next.js build. The two browser projects are Chromium and WebKit with touch enabled.

| Area | Coverage |
| --- | --- |
| Phone widths | 320, 360, 375, 390, 414, 430px |
| Boundary/tablet/desktop widths | 639, 640, 768, 1024, 1440px |
| Short dialogs | 320 × 480px; forms and payments also checked at 568px height |
| Admin routes | All 20 permitted navigation destinations plus `/global-reports` |
| POS routes | All 8 permitted navigation destinations |
| Navigation | All groups and child links visible, expansion without navigation, route selection, Escape dismissal and focus return |
| Admin interactions | Dashboard filtering/day changes, bounded selects, card actions, product/category/supplier/purchase-order forms, scrolling to final actions and closing |
| POS interactions | Lot selection, quantity changes, discounts, retained cart, cash/transfer/mixed payment previews, simulated receipt, new-sale reset, stock-request product search and quantities |
| Existing regression coverage | Admin/POS desktop menus and navigation, desktop POS payment/tax behavior, mobile shell |

The route sweeps check document overflow across 638 route/width/browser combinations. Screenshots and interactive checks cover selected light/dark layouts, drawer navigation, record cards, and short payment dialogs.

The payment tests use real backend previews and intercept **only final checkout**, so the tests do not create invoices or reduce stock. Create/edit dialogs and the stock-request form are inspected without submitting writes.

## Reproduce

Start the existing local backend/database, then run from `frontend`:

```sh
npm run lint
npx tsc --noEmit
npm run build:check
NEXT_DIST_DIR=.next-build BACKEND_INTERNAL_URL=http://localhost:8080 npm run start
```

In a second terminal:

```sh
npx playwright install chromium webkit
E2E_RUN=1 E2E_SKIP_WEBSERVER=1 npx playwright test --config=playwright.mobile.config.ts
```

`E2E_BASE_URL` can point the test run at another local frontend port. Screenshots and failure traces are written under `frontend/test-results`.

## Practical limits

This is browser-emulated validation, not a physical iPhone/Android or human usability study. Hardware keyboards, real software-keyboard transitions, screen readers, and device safe-area behavior still warrant a device smoke test. Viewport bounds and reduced-height states are covered automatically.

The local backend has no configured product-image storage, so existing image requests return 503 during this review. Product selection, stock data, and layout checks remain functional; successful photo loading was not verified in this environment. Pro destinations were checked in their existing gated state. The supplier list was empty in this dataset; its empty state and create form were verified.
