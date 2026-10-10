# V2 verification · 2026-10-09

Candidate: working tree based on `5e4ad86`, including earlier uncommitted V2 work. This is a verified development candidate, not a completed V2 release. No deployment, production migration/seed, commit or push was performed.

## Changes checked

- Migration 072 adds V2-only cancellation, refund/goods, shift variance/resolution and acknowledgement evidence, plus one open drawer per account.
- Sales cancellation preserves original invoices/payments, credits only the remaining uncredited lines, records the remaining refund liability and optionally returns actual received goods into quarantine.
- Partial cash/bank/equivalent-value goods refunds are atomic and idempotent. Goods preserve unit/price/VAT snapshots and consume FEFO stock/cost. Generic credit endpoints cannot bypass the cancellation refund queue.
- Pending refunds (including historical invoice dates) and unresolved variance carry into new shifts. Current warning acknowledgement is required before closing; changed warnings invalidate a stale acknowledgement. Only superadmin resolves variance in the pilot.
- Positive debt is counted per invoice for credit exposure; pending refund liabilities cannot reduce another invoice's exposure.
- Pro gate/badges temporarily disabled, sales menu links to owner-only V2; deferred pages show honest development status. V1 month-end runtime is untouched.
- Full quote line/customer/unit/discount/VAT/promotion revision, full promotion rule editor, shared line editor and multi-product shipment UI; explicit form-control labels and stale-detail-response guard.

## Actual results

| Check | Result and scope |
| --- | --- |
| `env -u TEST_DATABASE_URL go test ./...` | PASS, compile/non-DB regression; DB-dependent legacy tests skip, not a full V1 database regression |
| Go builds: `./cmd/api-v2` and `./cmd/api` | PASS; temporary binaries outside repository |
| Fresh migration + seed via `TestIntegrationHarness` | PASS, 001–072 on a newly created isolated PostgreSQL 17 DB |
| `go test -race -count=1 ./internal/v2` with isolated test DB | PASS; all prior workflows plus cancellation/refund/shift scenarios, concurrent refund spending, unpaid cancellation after partial credit and cheque guard |
| Focused ESLint, V2 UI + changed Pro/navigation + browser spec/config | PASS, zero lint warnings |
| `npm run build:check` | PASS, final V2 UI compiled/type checked in `.next-build` |
| Real-browser desktop 1280×900 | PASS, final expanded spec, 11.0 seconds |
| Real-browser tablet 820×1180 + mobile 390×844 | PASS, same final expanded spec, 34.1 seconds combined |
| Local backup/restore | PASS; isolated dump restored into a different test DB; complete-row hashes and counts match for 18 V1/V2 evidence tables |
| V1 transaction preservation | PASS; five seeded V1 stock/invoice/payment table fingerprints unchanged after all browser and integration work |
| `git diff --check` | PASS |

Browser checks actually submit invoice cancellation, goods refund, close/open with pending refund carry, multi-product shipment, promotion rule revision and quotation revision. They verify completed refunds, carried/resolved warning visibility, old quotation cancellation, no page errors and no document-level horizontal overflow at the checked screen sizes. They also check sales-menu access and the deferred government-page state.

The first browser startup used a command-rewriting path that rejected a valid server option; explicit `rtk proxy` fixed the local harness. A label locator mismatch led to explicit V2 control labelling. A new test referenced the wrong Go input type and was corrected before the final passing checks. Tests were repeated only after those failures or added coverage. Existing SWC fallback/Browserslist and standalone-server notices remain; no dependency upgrade was made.

## Environment and evidence

- PostgreSQL 17 bound to `127.0.0.1:55490`, source `pharmacy_v2_oct9_test`, restored `pharmacy_v2_restore_test`, `APP_ENV=test`.
- Temporary cluster/binaries/browser artifacts: `/tmp/pharmacy-v2-20261009.e7RGdp`. Fixture accounts/data are isolated. Session/signing credentials are not retained in these documents or Basic Memory.
- Browser used production build on loopback port 13184 with separate local V1 identity/catalog API on 18080 and V2 API on 18082; no production service was contacted.
- Source checksums: [VERIFICATION_2026-10-09.sha256](VERIFICATION_2026-10-09.sha256). Checks were on this candidate, not a tagged release.
- Servers and test database are stopped after verification; test-only data and dump remain in the temporary directory.

## Still not accepted

No customer/device UAT, real GitHub CI execution, representative production-size performance test, production recovery/RPO/RTO rehearsal, V1-data upgrade/cutover, or full existing V1 database/E2E regression. Cheque-linked invoice cancellation requires a separate bank reconciliation design. Company/legal print forms and VAT rounding remain candidates.

Month-end, V1 supplier return/cost adjustment/reversal, writer cutover and government/FDA forms remain explicitly deferred. The checks above do not mark all 71 acceptance items complete.
