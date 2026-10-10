package v2

import (
	"database/sql"
	"net/http"

	"pharmacy-erp/backend/internal/config"
	appMiddleware "pharmacy-erp/backend/internal/http/middleware"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

func NewHandler(cfg config.Config, db *sql.DB) http.Handler {
	e := echo.New()
	e.HideBanner = true
	e.Use(echoMiddleware.Recover(), echoMiddleware.RequestID(), echoMiddleware.BodyLimit("1M"))
	s := &Service{db: db}
	g := e.Group("/api/v2", appMiddleware.JWT(cfg.JWTSecret, db))
	// Pilot rollout is explicitly limited to the owner. V1 roles and grants are
	// unchanged; V2 role/entitlement onboarding is a separate release decision.
	g.Use(appMiddleware.RequireRole("super_admin"))
	g.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]any{"version": 2, "scope": "pilot", "v1_month_end": "preserved"})
	})
	g.GET("/catalog", s.listing(`SELECT jsonb_build_object('products',COALESCE((SELECT jsonb_agg(row_to_json(p)) FROM (SELECT id,sku,name,unit_name,ROUND(base_selling_price*100)::bigint AS price_cents FROM products WHERE active=TRUE ORDER BY name LIMIT 500)p),'[]'::jsonb),'units',COALESCE((SELECT jsonb_agg(row_to_json(u)) FROM product_units u WHERE active=TRUE),'[]'::jsonb),'suppliers',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id,'name',legal_name)) FROM suppliers WHERE active=TRUE),'[]'::jsonb),'policy',(SELECT row_to_json(bp) FROM v2_branch_policies bp WHERE branch_id=$1))`))
	g.GET("/customers", s.listing(`SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(c) ORDER BY c.name),'[]'::jsonb)) FROM v2_customers c WHERE branch_id=$1`))
	g.POST("/policy", command(s, "policy.save", s.savePolicy))
	g.POST("/customers", command(s, "customer.save", s.saveCustomer))
	g.GET("/inventory", s.listing(inventoryQuery))
	g.GET("/counts", s.listing(countsQuery))
	g.POST("/counts", command(s, "count.create", s.createCount))
	g.POST("/counts/observe", command(s, "count.observe", s.observeCount))
	g.POST("/counts/transition", command(s, "count.transition", s.confirmCount))
	g.GET("/returns", s.listing(returnsQuery))
	g.POST("/returns", command(s, "stock.return", s.returnStock))
	g.GET("/lots/:id/trace", s.lotTrace)
	g.GET("/stock-report", s.listing(stockReportQuery))
	g.POST("/stock/receive", command(s, "stock.receive", s.receive))
	g.POST("/stock/issue", command(s, "stock.issue", s.issue))
	g.POST("/stock/transfer", command(s, "stock.transfer", s.transfer))
	g.GET("/shipments", s.listing(shipmentsQuery))
	g.POST("/shipments", command(s, "shipment.dispatch", s.dispatchShipment))
	g.POST("/shipments/receive", command(s, "shipment.receive", s.receiveShipment))
	g.POST("/lots/state", command(s, "lot.state", s.lotState))
	g.POST("/lots/quarantine-part", command(s, "lot.quarantine_part", s.quarantinePart))
	g.POST("/recalls", command(s, "lot.recall_family", s.recallFamily))
	g.POST("/stock/write-off", command(s, "stock.write_off", s.writeOff))
	g.POST("/reservations", command(s, "reservation.create", s.reserve))
	g.GET("/reservations", s.listing(`SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(r) ORDER BY r.created_at DESC),'[]'::jsonb)) FROM v2_reservations r WHERE branch_id=$1`))
	g.POST("/reservations/transition", command(s, "reservation.transition", s.reservationTransition))
	g.GET("/documents", s.listing(documentsQuery))
	g.POST("/documents", command(s, "document.create", s.createDocument))
	g.POST("/documents/transition", command(s, "document.transition", s.documentTransition))
	g.POST("/quotations/revise", command(s, "quotation.revise", s.reviseQuotation))
	g.POST("/credit-notes", command(s, "document.credit", s.creditNote))
	g.POST("/documents/cancel", command(s, "invoice.cancel", s.cancelInvoice))
	g.GET("/cancellations", s.listing(cancellationsQuery))
	g.POST("/refunds", command(s, "refund.settle", s.settleRefund))
	g.POST("/credits/use", command(s, "credit.use", s.useCredit))
	g.GET("/statements", s.statements)
	g.POST("/payments", command(s, "payment.create", s.createPayment))
	g.POST("/allocations", command(s, "payment.allocate", s.allocatePayment))
	g.GET("/finance", s.listing(financeQuery))
	g.POST("/cheques/transition", command(s, "cheque.transition", s.chequeTransition))
	g.POST("/price-rules", command(s, "price_rule.save", s.priceRule))
	g.GET("/promotions", s.listing(promotionsQuery))
	g.POST("/promotions", command(s, "promotion.save", s.savePromotion))
	g.POST("/discount-approvals", command(s, "discount.approve", s.approveDiscount))
	g.GET("/price-rules", s.listing(`SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(r) ORDER BY r.starts_on DESC),'[]'::jsonb)) FROM v2_price_rules r WHERE r.branch_id=$1`))
	g.GET("/jobs", s.listing(jobsQuery))
	g.POST("/jobs/retry", command(s, "job.retry", s.retryJob))
	g.GET("/expiry", s.listing(expiryQuery))
	g.POST("/expiry/scan", command(s, "expiry.scan", s.scanExpiry))
	g.POST("/expiry/tasks", command(s, "expiry.task", s.expiryTask))
	g.GET("/drawers", s.drawers)
	g.POST("/drawers/issues/resolve", command(s, "shift.resolve", s.resolveShiftIssue))
	g.POST("/drawers", command(s, "drawer.open", s.openDrawer))
	g.POST("/drawers/cash", command(s, "drawer.cash", s.drawerCash))
	g.POST("/drawers/close", command(s, "drawer.close", s.closeDrawer))
	g.GET("/ledger", s.listing(`SELECT jsonb_build_object('stock',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (SELECT * FROM v2_stock_events WHERE branch_id=$1 ORDER BY created_at DESC LIMIT 200)x),'[]'::jsonb),'money',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (SELECT * FROM v2_money_events WHERE branch_id=$1 ORDER BY created_at DESC LIMIT 200)x),'[]'::jsonb))`))
	g.GET("/documents/:id", func(c echo.Context) error { return s.documentDetail(c) })
	return e
}

type PolicyInput struct {
	BranchID              string `json:"branch_id"`
	CostMethod            string `json:"cost_method"`
	AllowBranchPromotions bool   `json:"allow_branch_promotions"`
	MaxDiscountBPS        int64  `json:"max_discount_bps"`
}
