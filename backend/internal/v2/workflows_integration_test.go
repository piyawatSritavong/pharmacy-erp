package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/database"
	appMiddleware "pharmacy-erp/backend/internal/http/middleware"
)

type pilotFixture struct {
	t                                                   *testing.T
	db                                                  *sql.DB
	handler                                             http.Handler
	token, branch, destination, product, unit, customer string
}

func fixture(t *testing.T, db *sql.DB, method string) *pilotFixture {
	t.Helper()
	f := &pilotFixture{t: t, db: db, branch: newID(), destination: newID(), product: newID(), unit: newID()}
	var user string
	var version int
	if err := db.QueryRow(`SELECT id::text,auth_version FROM users WHERE email='superadmin@erp.local'`).Scan(&user, &version); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO branches(id,code,name) VALUES($1,$2,'V2 verification'),($3,$4,'V2 destination')`, f.branch, "CHECK-"+f.branch, f.destination, "CHECK-"+f.destination)
	f.exec(`INSERT INTO products(id,sku,name,cost_price,base_selling_price,unit_name) VALUES($1,$2,'V2 fixture',1,100,'ชิ้น')`, f.product, "V2-"+f.product)
	f.exec(`INSERT INTO product_units(id,product_id,unit_name,conversion_qty,sort_order) VALUES($1,$2,'กล่อง',10,1)`, f.unit, f.product)
	secret := "isolated-V2-fixture-signing-key"
	f.handler = NewHandler(config.Config{JWTSecret: secret}, db)
	claims := appMiddleware.Claims{UserID: user, AuthVersion: version, StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	var err error
	f.token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{f.branch, f.destination} {
		f.post("/policy", map[string]any{"branch_id": b, "cost_method": method, "allow_branch_promotions": true, "max_discount_bps": 1000}, 200, "")
	}
	f.customer = strID(f.post("/customers", CustomerInput{BranchID: f.branch, Code: newID(), Type: "person", Name: "V2 customer", CreditLimitCents: 1000000, Active: true}, 200, ""))
	return f
}
func strID(m map[string]any) string { return m["id"].(string) }
func number(v any) int64 {
	n, err := v.(json.Number).Int64()
	if err != nil {
		panic(err)
	}
	return n
}
func (f *pilotFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *pilotFixture) scalar(q string, args ...any) int64 {
	f.t.Helper()
	var n int64
	if err := f.db.QueryRow(q, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}
func (f *pilotFixture) id(q string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.db.QueryRow(q, args...).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}
func (f *pilotFixture) request(method, path, body, key, token string, want int) map[string]any {
	f.t.Helper()
	req := httptest.NewRequest(method, "/api/v2"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res := httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != want {
		f.t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, res.Code, res.Body.String())
	}
	var out map[string]any
	d := json.NewDecoder(res.Body)
	d.UseNumber()
	if err := d.Decode(&out); err != nil {
		f.t.Fatal(err)
	}
	return out
}
func (f *pilotFixture) post(path string, input any, status int, key string) map[string]any {
	f.t.Helper()
	if key == "" {
		key = newID()
	}
	return f.request("POST", path, rawJSON(input), key, f.token, status)
}
func (f *pilotFixture) get(path string) map[string]any {
	f.t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return f.request("GET", path+sep+"branch_id="+f.branch, "", "", f.token, 200)
}
func (f *pilotFixture) receive(qty, cost int64, expiry string) string {
	f.t.Helper()
	out := f.post("/stock/receive", StockInput{BranchID: f.branch, Reference: newID(), Items: []StockLine{{ProductID: f.product, Quantity: qty, UnitCostCents: cost, LotNumber: newID(), ExpiresOn: expiry}}}, 200, "")
	return out["lot_ids"].([]any)[0].(string)
}
func (f *pilotFixture) document(qty int64) map[string]any {
	return f.post("/documents", DocumentInput{BranchID: f.branch, Kind: "ar_invoice", CustomerID: f.customer, Lines: []DocumentLine{{ProductID: f.product, Quantity: qty, VATBPS: 700}}}, 200, "")
}
func (f *pilotFixture) balance(id string) int64 {
	return f.scalar(`SELECT COALESCE(SUM(amount_cents),0) FROM v2_money_events WHERE document_id=$1`, id)
}
func (f *pilotFixture) stock(qty, value int64) {
	f.t.Helper()
	for _, q := range []string{`SELECT base_quantity FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2`, `SELECT COALESCE(SUM(remaining_quantity),0) FROM v2_lots WHERE branch_id=$1 AND product_id=$2`, `SELECT COALESCE(SUM(quantity_delta),0) FROM v2_stock_events WHERE branch_id=$1 AND product_id=$2`} {
		if got := f.scalar(q, f.branch, f.product); got != qty {
			f.t.Fatalf("quantity invariant: got %d want %d", got, qty)
		}
	}
	if got := f.scalar(`SELECT value_cents FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2`, f.branch, f.product); got != value {
		f.t.Fatalf("value got %d want %d", got, value)
	}
	if got := f.scalar(`SELECT COALESCE(SUM(value_delta_cents),0) FROM v2_stock_events WHERE branch_id=$1 AND product_id=$2`, f.branch, f.product); got != value {
		f.t.Fatalf("ledger value got %d want %d", got, value)
	}
}

func TestV2PilotWorkflows(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires explicitly isolated TEST_DATABASE_URL")
	}
	db, err := database.OpenTest(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// Snapshot V1 transaction rows after seed. V2 operations must leave them exact.
	original := map[string]string{}
	for _, table := range []string{"inventory", "inventory_movements", "invoices", "invoice_items", "invoice_payments"} {
		var hash string
		if err = db.QueryRow(`SELECT md5(COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id)::text,'')) FROM ` + table + ` x`).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		original[table] = hash
	}
	t.Cleanup(func() {
		for table, expected := range original {
			var got string
			if err := db.QueryRow(`SELECT md5(COALESCE(jsonb_agg(to_jsonb(x) ORDER BY id)::text,'')) FROM ` + table + ` x`).Scan(&got); err != nil {
				t.Error(err)
			} else if got != expected {
				t.Errorf("V1 %s changed", table)
			}
		}
	})

	t.Run("routes_auth_and_json", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		for _, path := range []string{"/health", "/catalog", "/customers", "/inventory", "/counts", "/returns", "/stock-report", "/reservations", "/documents", "/finance", "/statements", "/price-rules", "/promotions", "/jobs", "/expiry", "/drawers", "/ledger", "/shipments"} {
			f.get(path)
		}
		f.request("GET", "/health", "", "", "", 401)
		var user string
		var version int
		if err := db.QueryRow(`SELECT u.id::text,u.auth_version FROM users u JOIN roles r ON r.id=u.role_id WHERE u.active=TRUE AND r.role_key<>'super_admin' LIMIT 1`).Scan(&user, &version); err != nil {
			t.Fatal(err)
		}
		token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, appMiddleware.Claims{UserID: user, AuthVersion: version, StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()}}).SignedString([]byte("isolated-V2-fixture-signing-key"))
		f.request("GET", "/health", "", "", token, 403)
		f.request("POST", "/stock/receive", `{} {}`, newID(), f.token, 400)
	})
	t.Run("idempotency_unit_and_landed_cost", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		key := newID()
		in := StockInput{BranchID: f.branch, Reference: "rounding", HeaderDiscountCents: 3, Items: []StockLine{{ProductID: f.product, Quantity: 1, UnitCostCents: 1, LotNumber: "A"}, {ProductID: f.product, Quantity: 1, UnitCostCents: 1, LotNumber: "B"}, {ProductID: f.product, Quantity: 1, UnitCostCents: 1, LotNumber: "C"}}}
		f.post("/stock/receive", in, 200, key)
		f.post("/stock/receive", in, 200, key)
		f.stock(3, 0)
		in.HeaderDiscountCents = 2
		f.post("/stock/receive", in, 409, key)
		f.post("/stock/receive", StockInput{BranchID: f.branch, Reference: "units", ShippingCents: 7, Items: []StockLine{{ProductID: f.product, UnitID: f.unit, Quantity: 2, UnitCostCents: 1000, LotNumber: "box"}}}, 200, "")
		f.stock(23, 2007)
	})
	t.Run("physical_fefo_and_valuation", func(t *testing.T) {
		for _, method := range []string{"fifo", "moving_average"} {
			t.Run(method, func(t *testing.T) {
				f := fixture(t, db, method)
				f.receive(10, 100, "2099-12-31")
				early := f.receive(10, 200, "2098-12-31")
				out := f.post("/stock/issue", StockInput{BranchID: f.branch, Reference: "issue", Items: []StockLine{{ProductID: f.product, Quantity: 12}}}, 200, "")
				cost, remaining := int64(1400), int64(1600)
				if method == "moving_average" {
					cost, remaining = 1800, 1200
				}
				if number(out["cost_cents"]) != cost {
					t.Fatal("wrong valuation", out)
				}
				if f.scalar(`SELECT remaining_quantity FROM v2_lots WHERE id=$1`, early) != 0 {
					t.Fatal("earliest expiry not consumed")
				}
				f.stock(8, remaining)
			})
		}
	})
	t.Run("last_item_concurrent_reservation", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(1, 100, "")
		in := StockInput{BranchID: f.branch, Reference: "last item", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339), Items: []StockLine{{ProductID: f.product, Quantity: 1}}}
		start := make(chan struct{})
		results := make(chan int, 2)
		for range 2 {
			go func() {
				<-start
				req := httptest.NewRequest("POST", "/api/v2/reservations", strings.NewReader(rawJSON(in)))
				req.Header.Set("Authorization", "Bearer "+f.token)
				req.Header.Set("Idempotency-Key", newID())
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				f.handler.ServeHTTP(res, req)
				results <- res.Code
			}()
		}
		close(start)
		a, b := <-results, <-results
		if !((a == 200 && b == 409) || (a == 409 && b == 200)) {
			t.Fatalf("concurrency: %d %d", a, b)
		}
		reservation := f.id(`SELECT id::text FROM v2_reservations WHERE branch_id=$1`, f.branch)
		f.post("/stock/issue", StockInput{BranchID: f.branch, Reference: "reserved", Items: in.Items}, 409, "")
		f.post("/reservations/transition", ReservationInput{BranchID: f.branch, ReservationID: reservation, Action: "consume"}, 200, "")
		f.stock(0, 0)
		f.post("/reservations/transition", ReservationInput{BranchID: f.branch, ReservationID: reservation, Action: "release"}, 409, "")
	})
	t.Run("shipment_partial_receipts_and_quarantine_recall", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		root := f.receive(10, 100, "")
		sh := strID(f.post("/shipments", StockInput{BranchID: f.branch, DestinationBranchID: f.destination, Reference: "shipment", Items: []StockLine{{ProductID: f.product, Quantity: 4}}}, 200, ""))
		line := f.id(`SELECT id::text FROM v2_shipment_lines WHERE shipment_id=$1`, sh)
		in := ShipmentReceiptInput{BranchID: f.destination, ShipmentID: sh, Action: "receive", Lines: []ShipmentReceiptLine{{ID: line, BaseQuantity: 2}}}
		key := newID()
		f.post("/shipments/receive", in, 200, key)
		f.post("/shipments/receive", in, 200, key)
		f.post("/shipments/receive", in, 200, "")
		f.post("/shipments/receive", in, 409, "")
		f.stock(6, 600)
		if f.scalar(`SELECT base_quantity FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2`, f.destination, f.product) != 4 {
			t.Fatal("destination stock mismatch")
		}
		f.post("/lots/quarantine-part", LotSplitInput{BranchID: f.branch, LotID: root, BaseQuantity: 2, Reason: "inspect"}, 200, "")
		f.stock(6, 600)
		f.post("/recalls", LotStateInput{BranchID: f.branch, LotID: root, Reason: "supplier notice"}, 200, "")
		if f.scalar(`SELECT COUNT(*) FROM v2_lots WHERE product_id=$1 AND state<>'recalled'`, f.product) != 0 {
			t.Fatal("recall did not cover descendants")
		}
		f.get("/lots/" + root + "/trace")
	})
	t.Run("counts_adjust_once_and_reject_stale", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(5, 100, "")
		count := strID(f.post("/counts", CountInput{BranchID: f.branch, Reference: "count", ProductIDs: []string{f.product}}, 200, ""))
		sn := f.id(`SELECT id::text FROM v2_count_snapshots WHERE count_id=$1`, count)
		f.post("/counts/observe", CountInput{BranchID: f.branch, ID: count, Revision: 1, Lines: []CountLine{{SnapshotID: sn, CountedQuantity: 4, Reason: "missing"}}}, 200, "")
		f.post("/counts/transition", CountInput{BranchID: f.branch, ID: count, Revision: 2, Action: "confirm", Reason: "approved"}, 200, "")
		f.stock(4, 400)
		f.post("/counts/transition", CountInput{BranchID: f.branch, ID: count, Revision: 2, Action: "confirm", Reason: "approved"}, 409, "")
		count = strID(f.post("/counts", CountInput{BranchID: f.branch, Reference: "stale", ProductIDs: []string{f.product}}, 200, ""))
		sn = f.id(`SELECT id::text FROM v2_count_snapshots WHERE count_id=$1`, count)
		f.post("/counts/observe", CountInput{BranchID: f.branch, ID: count, Revision: 1, Lines: []CountLine{{SnapshotID: sn, CountedQuantity: 4}}}, 200, "")
		f.post("/stock/issue", StockInput{BranchID: f.branch, Reference: "after count", Items: []StockLine{{ProductID: f.product, Quantity: 1}}}, 200, "")
		f.post("/counts/transition", CountInput{BranchID: f.branch, ID: count, Revision: 2, Action: "confirm", Reason: "stale"}, 409, "")
		f.stock(3, 300)
	})
	t.Run("ar_allocations_cheque_cash_credit_and_return", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(20, 100, "")
		doc := strID(f.document(2))
		if f.balance(doc) != 21400 {
			t.Fatal("invoice total")
		}
		pay := strID(f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "bank_transfer", AmountCents: 5000, Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 3000}}}, 200, ""))
		f.post("/allocations", AllocationInput{BranchID: f.branch, PaymentID: pay, Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 2000}}}, 200, "")
		if f.balance(doc) != 16400 {
			t.Fatal("multiple allocations")
		}
		cheque := f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "cheque", AmountCents: 5000, ChequeNumber: newID(), Bank: "fixture bank", ChequeDueOn: today(), Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 5000}}}, 200, "")
		id := cheque["cheque_id"].(string)
		for i, action := range []string{"deposited", "cleared", "bounced"} {
			f.post("/cheques/transition", TransitionInput{BranchID: f.branch, ID: id, Revision: i + 1, Action: action, Reason: "bank proof"}, 200, "")
			expected := int64(16400)
			if action == "cleared" {
				expected = 11400
			}
			if f.balance(doc) != expected {
				t.Fatal("cheque debt state", action)
			}
		}
		f.post("/cheques/transition", TransitionInput{BranchID: f.branch, ID: id, Revision: 3, Action: "bounced", Reason: "duplicate"}, 409, "")
		drawer := strID(f.post("/drawers", DrawerInput{BranchID: f.branch, OpeningCents: 1000}, 200, ""))
		f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "cash", DrawerID: drawer, AmountCents: 1000, Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 1000}}}, 200, "")
		f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: drawer, CountedCents: 2000}, 200, "")
		f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: drawer, CountedCents: 2000}, 409, "")
		source := f.id(`SELECT id::text FROM v2_document_lines WHERE document_id=$1`, doc)
		f.post("/credit-notes", CreditInput{BranchID: f.branch, DocumentID: doc, Reason: "partial credit", Lines: []CreditLine{{SourceLineID: source, Quantity: 1}}}, 200, "")
		f.post("/credit-notes", CreditInput{BranchID: f.branch, DocumentID: doc, Reason: "remaining credit", Lines: []CreditLine{{SourceLineID: source, Quantity: 1}}}, 200, "")
		if f.balance(doc) != -6000 {
			t.Fatal("credit remaining", f.balance(doc))
		}
		f.post("/credits/use", CreditUseInput{BranchID: f.branch, SourceDocumentID: doc, Kind: "refund", Method: "bank_transfer", AmountCents: 1000, Reference: "refund", Reason: "customer"}, 200, "")
		other := strID(f.document(1))
		f.post("/credits/use", CreditUseInput{BranchID: f.branch, SourceDocumentID: doc, TargetDocumentID: other, Kind: "apply", AmountCents: 5000, Reference: "credit", Reason: "reuse"}, 200, "")
		if f.balance(doc) != 0 || f.balance(other) != 5700 {
			t.Fatal("credit application")
		}
		event := f.id(`SELECT id::text FROM v2_stock_events WHERE reference=$1 AND event_type='invoice.issue' LIMIT 1`, doc)
		f.post("/returns", ReturnInput{BranchID: f.branch, SourceEventID: event, BaseQuantity: 1, Reason: "inspect return"}, 200, "")
		f.post("/returns", ReturnInput{BranchID: f.branch, SourceEventID: event, BaseQuantity: 1, Reason: "second return"}, 200, "")
		f.post("/returns", ReturnInput{BranchID: f.branch, SourceEventID: event, BaseQuantity: 1, Reason: "excess"}, 409, "")
		f.stock(19, 1900)
		f.get("/documents/" + doc)
		f.get("/statements")
		f.get("/finance")
		if _, err := db.Exec(`UPDATE v2_money_events SET amount_cents=0 WHERE document_id=$1`, doc); err == nil {
			t.Fatal("evidence mutable")
		}
		if _, err := db.Exec(`UPDATE v2_documents SET total_cents=0 WHERE id=$1`, doc); err == nil {
			t.Fatal("document facts mutable")
		}
	})
	t.Run("promotions_snapshot_and_discount_approval", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(100, 100, "")
		reward := newID()
		f.exec(`INSERT INTO products(id,sku,name,cost_price,base_selling_price) VALUES($1,$2,'V2 reward',1,100)`, reward, "V2-"+reward)
		f.post("/stock/receive", StockInput{BranchID: f.branch, Reference: "rewards", Items: []StockLine{{ProductID: reward, Quantity: 10, UnitCostCents: 50, LotNumber: "gifts"}}}, 200, "")
		for _, kind := range []string{"percent", "amount", "buy_x_get_y", "bundle", "bill_giveaway"} {
			rule := PromotionRule{DiscountBPS: 1000, DiscountCents: 2000, BundlePriceCents: 15000, MinimumAmountCents: 10000}
			qty := int64(1)
			lines := []DocumentLine{{ProductID: f.product, Quantity: 1, VATBPS: 700}}
			expected := int64(9630)
			switch kind {
			case "amount":
				expected = 8560
			case "buy_x_get_y":
				qty = 2
				lines[0].Quantity = qty
				rule.Conditions = []PromotionMember{{ProductID: f.product, Quantity: 2}}
				rule.Rewards = []PromotionMember{{ProductID: reward, Quantity: 1}}
				expected = 21400
			case "bundle":
				rule.Conditions = []PromotionMember{{ProductID: f.product, Quantity: 1}, {ProductID: reward, Quantity: 1}}
				lines = append(lines, DocumentLine{ProductID: reward, Quantity: 1, VATBPS: 700})
				expected = 16050
			case "bill_giveaway":
				rule.Rewards = []PromotionMember{{ProductID: reward, Quantity: 1}}
				expected = 10700
			}
			promo := strID(f.post("/promotions", PromotionInput{BranchID: f.branch, Scope: "branch", Code: newID(), Name: kind, Type: kind, Rule: rule, StartsOn: today(), Active: true, Reason: "pilot rule"}, 200, ""))
			out := f.post("/documents", DocumentInput{BranchID: f.branch, Kind: "ar_invoice", CustomerID: f.customer, PromotionID: promo, Lines: lines}, 200, "")
			if number(out["total_cents"]) != expected {
				t.Fatal(kind, "total", out)
			}
			if f.scalar(`SELECT COUNT(*) FROM v2_document_promotions WHERE document_id=$1`, strID(out)) != 1 {
				t.Fatal("missing promo snapshot")
			}
		}
		in := DocumentInput{BranchID: f.branch, Kind: "quotation", CustomerID: f.customer, Lines: []DocumentLine{{ProductID: f.product, Quantity: 1, DiscountCents: 2500, VATBPS: 700}}}
		f.post("/documents", in, 409, "")
		grant := strID(f.post("/discount-approvals", DiscountApprovalInput{BranchID: f.branch, Reason: "owner approves", Document: in}, 200, ""))
		in.ApprovalID = grant
		stale := in
		stale.Lines = append([]DocumentLine(nil), in.Lines...)
		stale.Lines[0].Quantity = 2
		f.post("/documents", stale, 409, "")
		f.post("/documents", in, 200, "")
		f.post("/documents", in, 409, "")
	})
	t.Run("quotation_unit_snapshot_expiry_and_outbox", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(30, 100, "2099-12-31")
		quote := strID(f.post("/documents", DocumentInput{BranchID: f.branch, Kind: "quotation", CustomerID: f.customer, Lines: []DocumentLine{{ProductID: f.product, UnitID: f.unit, Quantity: 1}}}, 200, ""))
		f.exec(`UPDATE product_units SET conversion_qty=20 WHERE id=$1`, f.unit)
		f.post("/documents/transition", TransitionInput{BranchID: f.branch, ID: quote, Action: "convert"}, 200, "")
		f.stock(20, 2000)
		expired := f.receive(2, 100, "2020-01-01")
		f.post("/expiry/scan", BranchInput{BranchID: f.branch}, 200, "")
		if f.scalar(`SELECT band_months FROM v2_expiry_tasks WHERE lot_id=$1`, expired) != 0 {
			t.Fatal("expiry band")
		}
		again := f.post("/expiry/scan", BranchInput{BranchID: f.branch}, 200, "")
		if number(again["created_tasks"]) != 0 {
			t.Fatal("duplicate expiry task")
		}
		f.post("/stock/write-off", StockInput{BranchID: f.branch, Reference: "expired", Reason: "expired disposal", Items: []StockLine{{ProductID: f.product, LotID: expired, Quantity: 2}}}, 200, "")
		f.stock(20, 2000)
		service := &Service{db: db}
		for range 1000 {
			worked, err := service.processJob(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !worked {
				break
			}
		}
		if f.scalar(`SELECT COUNT(*) FROM v2_outbox WHERE status<>'done'`) != 0 {
			t.Fatal("outbox not drained")
		}
		before := f.scalar(`SELECT COUNT(*) FROM v2_notifications`)
		f.exec(`UPDATE v2_outbox SET status='processing',locked_until=NOW()-INTERVAL '1 minute',lease_token=$1 WHERE id=(SELECT id FROM v2_outbox LIMIT 1)`, newID())
		if _, err := service.processJob(context.Background()); err != nil {
			t.Fatal(err)
		}
		if f.scalar(`SELECT COUNT(*) FROM v2_notifications`) != before {
			t.Fatal("job replay duplicate notification")
		}
	})
	t.Run("cancellation_refund_goods_and_shift_carry", func(t *testing.T) { verifyRefundShifts(t, db) })
	t.Run("unpaid_cancel_remaining_credit_and_cheque_guard", func(t *testing.T) {
		f := fixture(t, db, "fifo")
		f.receive(5, 100, "2099-12-31")
		doc := strID(f.document(2))
		line := f.id(`SELECT id::text FROM v2_document_lines WHERE document_id=$1`, doc)
		f.post("/credit-notes", CreditInput{BranchID: f.branch, DocumentID: doc, Reason: "prior partial credit", Lines: []CreditLine{{SourceLineID: line, Quantity: 1}}}, 200, "")
		out := f.post("/documents/cancel", CancellationInput{BranchID: f.branch, DocumentID: doc, Reference: "unpaid", Reason: "cancel remainder"}, 200, "")
		if number(out["refund_due_cents"]) != 0 || f.balance(doc) != 0 {
			t.Fatal("unpaid cancellation created refund")
		}
		f.stock(3, 300)
		doc = strID(f.document(1))
		p := f.post("/payments", PaymentInput{BranchID: f.branch, Direction: "receive", CustomerID: f.customer, Method: "cheque", AmountCents: 10700, ChequeNumber: newID(), Bank: "isolated bank", ChequeDueOn: today(), Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 10700}}}, 200, "")
		in := CancellationInput{BranchID: f.branch, DocumentID: doc, Reference: "cheque cancel", Reason: "customer cancel"}
		f.post("/documents/cancel", in, 409, "")
		cheque := p["cheque_id"].(string)
		f.post("/cheques/transition", TransitionInput{BranchID: f.branch, ID: cheque, Revision: 1, Action: "deposited", Reason: "deposit", EffectiveOn: today()}, 200, "")
		f.post("/cheques/transition", TransitionInput{BranchID: f.branch, ID: cheque, Revision: 2, Action: "cleared", Reason: "clear", EffectiveOn: today()}, 200, "")
		f.post("/documents/cancel", in, 409, "")
		f.post("/cheques/transition", TransitionInput{BranchID: f.branch, ID: cheque, Revision: 3, Action: "bounced", Reason: "bounce", EffectiveOn: today()}, 200, "")
		out = f.post("/documents/cancel", in, 200, "")
		if number(out["refund_due_cents"]) != 0 || f.balance(doc) != 0 {
			t.Fatal("bounced cheque refunded unsettled money")
		}
	})
	// All V2 inventory accounts must agree with physical lots and append-only ledger.
	var mismatches int
	if err = db.QueryRow(`SELECT COUNT(*) FROM v2_stock_accounts a WHERE a.base_quantity<>(SELECT COALESCE(SUM(remaining_quantity),0) FROM v2_lots l WHERE l.branch_id=a.branch_id AND l.product_id=a.product_id) OR a.base_quantity<>(SELECT COALESCE(SUM(quantity_delta),0) FROM v2_stock_events e WHERE e.branch_id=a.branch_id AND e.product_id=a.product_id) OR a.value_cents<>(SELECT COALESCE(SUM(value_delta_cents),0) FROM v2_stock_events e WHERE e.branch_id=a.branch_id AND e.product_id=a.product_id)`).Scan(&mismatches); err != nil {
		t.Fatal(err)
	}
	if mismatches != 0 {
		t.Fatal(fmt.Sprintf("%d stock ledger mismatches", mismatches))
	}
}
