package v2

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
)

func verifyRefundShifts(t *testing.T, db *sql.DB) {
	f := fixture(t, db, "fifo")
	f.receive(10, 100, "2099-12-31")
	doc := strID(f.document(2))
	drawer := strID(f.post("/drawers", DrawerInput{BranchID: f.branch, OpeningCents: 10000}, 200, ""))
	f.post("/drawers", DrawerInput{BranchID: f.destination}, 409, "")
	f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "cash", DrawerID: drawer, AmountCents: 1000, Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 1000}}}, 200, "")
	f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "bank_transfer", AmountCents: 20400, Allocations: []PaymentAllocation{{DocumentID: doc, AmountCents: 20400}}}, 200, "")
	in := CancellationInput{BranchID: f.branch, DocumentID: doc, Reference: "cancel evidence", Reason: "customer returns all goods", ReturnStock: true}
	key := newID()
	out := f.post("/documents/cancel", in, 200, key)
	cancel := strID(out)
	if number(out["refund_due_cents"]) != 21400 || f.balance(doc) != -21400 {
		t.Fatal("cancel refund liability")
	}
	if strID(f.post("/documents/cancel", in, 200, key)) != cancel {
		t.Fatal("cancel replay")
	}
	f.post("/documents/cancel", in, 409, "")
	f.stock(10, 1000)
	if f.scalar(`SELECT COUNT(*) FROM v2_stock_returns WHERE branch_id=$1`, f.branch) != 1 {
		t.Fatal("return duplicated")
	}
	f.post("/credits/use", CreditUseInput{BranchID: f.branch, SourceDocumentID: doc, Kind: "refund", Method: "bank_transfer", AmountCents: 100, Reference: "bypass", Reason: "bypass"}, 409, "")
	warnings := f.get("/drawers")
	oldToken := warnings["warnings_token"].(string)
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: drawer, CountedCents: 11000}, 409, "")
	goods := RefundInput{BranchID: f.branch, CancellationID: cancel, Method: "goods", AmountCents: 10701, Reference: "exchange", Reason: "same value goods", Goods: []RefundGoodsLine{{ProductID: f.product, Quantity: 1, VATBPS: 700}}}
	f.post("/refunds", goods, 409, "")
	f.stock(10, 1000)
	goods.AmountCents = 10700
	goodsKey := newID()
	f.post("/refunds", goods, 200, goodsKey)
	f.post("/refunds", goods, 200, goodsKey)
	f.stock(9, 900)
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: drawer, CountedCents: 11000, WarningsToken: oldToken}, 409, "")
	token := f.get("/drawers")["warnings_token"].(string)
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: drawer, CountedCents: 11000, WarningsToken: token}, 200, "")
	next := f.post("/drawers", DrawerInput{BranchID: f.branch, OpeningCents: 11000}, 200, "")
	nextDrawer := strID(next)
	if len(next["warnings"].([]any)) != 1 {
		t.Fatal("pending refund lost on next shift")
	}
	token = next["warnings_token"].(string)
	refund := RefundInput{BranchID: f.branch, CancellationID: cancel, Method: "bank_transfer", AmountCents: 5000, Reference: "bank evidence", Reason: "partial bank refund"}
	f.post("/refunds", refund, 200, "")
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: nextDrawer, CountedCents: 11000, WarningsToken: token}, 409, "")
	refund.Method = "cash"
	refund.AmountCents = 5700
	refund.DrawerID = drawer
	refund.Reference = "cash evidence"
	f.post("/refunds", refund, 409, "")
	if f.balance(doc) != -5700 {
		t.Fatal("failed refund changed liability")
	}
	refund.DrawerID = nextDrawer
	key = newID()
	f.post("/refunds", refund, 200, key)
	f.post("/refunds", refund, 200, key)
	refund.AmountCents = 1
	f.post("/refunds", refund, 409, "")
	if f.balance(doc) != 0 {
		t.Fatal("refund did not settle")
	}
	if len(f.get("/drawers")["warnings"].([]any)) != 0 {
		t.Fatal("settled refund still pending")
	}
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: nextDrawer, CountedCents: 5200, Reason: "short by 100"}, 200, "")
	next = f.post("/drawers", DrawerInput{BranchID: f.branch, OpeningCents: 5200}, 200, "")
	nextDrawer = strID(next)
	pending := next["warnings"].([]any)
	if len(pending) != 1 || number(pending[0].(map[string]any)["amount_cents"]) != -100 {
		t.Fatal("variance carry forward")
	}
	issue := pending[0].(map[string]any)["id"].(string)
	f.post("/drawers/issues/resolve", ShiftResolutionInput{BranchID: f.branch, IssueID: issue}, 400, "")
	f.post("/drawers/issues/resolve", ShiftResolutionInput{BranchID: f.branch, IssueID: issue, Reason: "owner investigated"}, 200, "")
	f.post("/drawers/issues/resolve", ShiftResolutionInput{BranchID: f.branch, IssueID: issue, Reason: "duplicate"}, 409, "")
	f.post("/drawers/close", DrawerInput{BranchID: f.branch, ID: nextDrawer, CountedCents: 5200}, 200, "")
	// A historical paid invoice must warn even when cancellation is entered today.
	historical := newID()
	var operation, actor string
	if err := db.QueryRow(`SELECT operation_id::text,created_by::text FROM v2_documents WHERE id=$1`, doc).Scan(&operation, &actor); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO v2_documents(id,operation_id,branch_id,document_number,kind,status,customer_id,counterparty_snapshot,total_cents,issued_on,due_on,created_by) VALUES($1,$2,$3,$4,'ar_invoice','issued',$5,'{}',10000,(NOW() AT TIME ZONE 'Asia/Bangkok')::date-1,(NOW() AT TIME ZONE 'Asia/Bangkok')::date,$6)`, historical, operation, f.branch, "HIST-"+historical, f.customer, actor)
	f.exec(`INSERT INTO v2_document_lines(id,document_id,product_id,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source) VALUES($1,$2,$3,'historical fixture',1,1,'{"name":"unit","conversion":1,"quantity":1,"base_quantity":1}',10000,0,0,0,10000,'fixture')`, newID(), historical, f.product)
	f.exec(`INSERT INTO v2_money_events(id,operation_id,branch_id,document_id,event_type,amount_cents,effective_on,actor_id) VALUES($1,$2,$3,$4,'document.issued',10000,(NOW() AT TIME ZONE 'Asia/Bangkok')::date-1,$5)`, newID(), operation, f.branch, historical, actor)
	f.post("/payments", PaymentInput{BranchID: f.branch, CustomerID: f.customer, Direction: "receive", Method: "bank_transfer", AmountCents: 10000, Allocations: []PaymentAllocation{{DocumentID: historical, AmountCents: 10000}}}, 200, "")
	out = f.post("/documents/cancel", CancellationInput{BranchID: f.branch, DocumentID: historical, Reference: "cross day", Reason: "historical cancellation"}, 200, "")
	warnings = f.get("/drawers")
	pending = warnings["warnings"].([]any)
	if len(pending) != 1 || pending[0].(map[string]any)["cross_day"] != true {
		t.Fatal("cross-day warning missing")
	}
	f.document(1)
	f.exec(`UPDATE v2_customers SET credit_limit_cents=20000 WHERE id=$1`, f.customer)
	f.post("/documents", DocumentInput{BranchID: f.branch, Kind: "ar_invoice", CustomerID: f.customer, Lines: []DocumentLine{{ProductID: f.product, Quantity: 1, VATBPS: 700}}}, 409, "")
	refund = RefundInput{BranchID: f.branch, CancellationID: strID(out), Method: "bank_transfer", AmountCents: 10000, Reference: "historical refund", Reason: "full refund"}
	// Two independent retries cannot both spend the remaining liability.
	refund.AmountCents = 6000
	statuses := make(chan int, 2)
	for range 2 {
		go func() {
			req := httptest.NewRequest("POST", "/api/v2/refunds", strings.NewReader(rawJSON(refund)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+f.token)
			req.Header.Set("Idempotency-Key", newID())
			res := httptest.NewRecorder()
			f.handler.ServeHTTP(res, req)
			statuses <- res.Code
		}()
	}
	first, second := <-statuses, <-statuses
	if !((first == 200 && second == 409) || (first == 409 && second == 200)) {
		t.Fatalf("concurrent refund codes %d/%d", first, second)
	}
	refund.AmountCents = 4000
	f.post("/refunds", refund, 200, "")
	for _, table := range []string{"v2_cancellations", "v2_refund_settlements", "v2_refund_goods", "v2_shift_issues", "v2_shift_issue_resolutions", "v2_shift_acknowledgements"} {
		if _, err := db.Exec(`DELETE FROM ` + table + ` WHERE id=(SELECT id FROM ` + table + ` LIMIT 1)`); err == nil {
			t.Fatalf("%s evidence was mutable", table)
		}
	}
}
