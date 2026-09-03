package http

import (
	"database/sql"
	"net/http"

	"pharmacy-erp/backend/internal/config"
	appMiddleware "pharmacy-erp/backend/internal/http/middleware"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/auth"
	"pharmacy-erp/backend/internal/modules/branches"
	"pharmacy-erp/backend/internal/modules/dashboard"
	"pharmacy-erp/backend/internal/modules/fda"
	"pharmacy-erp/backend/internal/modules/inventory"
	"pharmacy-erp/backend/internal/modules/marketplace"
	"pharmacy-erp/backend/internal/modules/monthend"
	"pharmacy-erp/backend/internal/modules/parkedbills"
	"pharmacy-erp/backend/internal/modules/products"
	"pharmacy-erp/backend/internal/modules/promotions"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/modules/reports"
	"pharmacy-erp/backend/internal/modules/returns"
	"pharmacy-erp/backend/internal/modules/sales"
	"pharmacy-erp/backend/internal/modules/transfers"
	"pharmacy-erp/backend/internal/modules/users"

	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

type Server struct {
	engine *echo.Echo
}

func NewServer(cfg config.Config, db *sql.DB) *Server {
	engine := echo.New()
	engine.HideBanner = true
	engine.Use(echoMiddleware.Recover())
	engine.Use(echoMiddleware.RequestID())
	engine.Use(appMiddleware.RequestInfo())
	engine.Use(echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins:     []string{cfg.FrontendURL, "http://localhost:3000"},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, echo.HeaderXRequestID},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,
	}))

	auditService := audit.NewService(db)
	authHandler := auth.NewHandler(auth.NewService(db, cfg))
	branchHandler := branches.NewHandler(branches.NewService(db, auditService))
	userHandler := users.NewHandler(users.NewService(db, auditService))
	dashboardHandler := dashboard.NewHandler(dashboard.NewService(db))
	productHandler := products.NewHandler(products.NewService(db, auditService, cfg.UploadDir))
	promotionHandler := promotions.NewHandler(promotions.NewService(db, auditService))
	purchasingHandler := purchasing.NewHandler(purchasing.NewService(db, auditService))
	inventoryHandler := inventory.NewHandler(inventory.NewService(db, auditService))
	salesHandler := sales.NewHandler(sales.NewService(db, auditService))
	transferHandler := transfers.NewHandler(transfers.NewService(db, auditService))
	reportHandler := reports.NewHandler(reports.NewService(db))
	marketplaceHandler := marketplace.NewHandler(marketplace.NewService(db, auditService))
	monthEndHandler := monthend.NewHandler(monthend.NewService(db, auditService))
	fdaHandler := fda.NewHandler(fda.NewService(db))
	returnsHandler := returns.NewHandler(returns.NewService(db, auditService))
	monthEndWorkflowHandler := monthend.NewWorkflowHandler(monthend.NewService(db, auditService))
	auditHandler := audit.NewHandler(auditService)
	parkedBillHandler := parkedbills.NewHandler(parkedbills.NewService(db))

	api := engine.Group("/api/v1")
	api.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
	})
	api.POST("/auth/login", authHandler.Login)
	api.POST("/auth/logout", authHandler.Logout)
	// Exact admin report path retained for external integrations. The service,
	// this group, and the versioned alias below all enforce the literal role.
	adminAPI := engine.Group("/api/admin")
	adminAPI.Use(appMiddleware.JWT(cfg.JWTSecret))
	adminAPI.GET("/month-end-report", monthEndHandler.MonthEndReport, appMiddleware.RequireRole("super_admin"))

	protected := api.Group("")
	protected.Use(appMiddleware.JWT(cfg.JWTSecret))
	superadminOnly := appMiddleware.RequireRole("super_admin")
	protected.GET("/me", authHandler.Me)
	protected.GET("/dashboard", dashboardHandler.Summary, appMiddleware.RequireAnyPermission("dashboard.view.global", "dashboard.view.self"))
	protected.GET("/dashboard/branch-sales", dashboardHandler.BranchSales, appMiddleware.RequireAnyPermission("dashboard.view.global"))
	protected.GET("/dashboard/today-branch-sales", dashboardHandler.TodayBranchSales, appMiddleware.RequireAnyPermission("dashboard.view.global"))
	protected.GET("/dashboard/low-stock", dashboardHandler.LowStock, appMiddleware.RequireAnyPermission("dashboard.view.global"))
	protected.GET("/notifications", dashboardHandler.Notifications, appMiddleware.RequireAnyPermission("dashboard.view.global"))
	protected.GET("/dashboard/daily-sales", dashboardHandler.DailySales, appMiddleware.RequireAnyPermission("dashboard.view.self"))
	protected.GET("/dashboard/sales-export", dashboardHandler.ExportSales, appMiddleware.RequireAnyPermission("dashboard.view.self"))

	protected.GET("/branches", branchHandler.List)
	protected.POST("/branches", branchHandler.Create, appMiddleware.RequireAnyPermission("settings.manage"))
	protected.PUT("/branches/:branchID", branchHandler.Update, appMiddleware.RequireAnyPermission("settings.manage"))
	protected.GET("/branches/:branchID/deletion-impact", branchHandler.DeletionImpact, superadminOnly)
	protected.DELETE("/branches/:branchID", branchHandler.Delete, superadminOnly)
	protected.GET("/branches/sequences", branchHandler.ListSequences, appMiddleware.RequireAnyPermission("settings.manage", "invoice.sequence.manage"))
	protected.PUT("/branches/:branchID/sequences/:docType", branchHandler.UpdateSequence, appMiddleware.RequireAnyPermission("settings.manage", "invoice.sequence.manage"))

	protected.GET("/users", userHandler.ListUsers, appMiddleware.RequireAnyPermission("users.manage"))
	protected.POST("/users", userHandler.CreateUser, appMiddleware.RequireAnyPermission("users.manage"))
	protected.PUT("/users/:userID", userHandler.UpdateUser, appMiddleware.RequireAnyPermission("users.manage"))
	protected.POST("/users/:userID/reset-password", userHandler.ResetPassword, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/users/:userID/deletion-impact", userHandler.UserDeletionImpact, appMiddleware.RequireAnyPermission("users.manage"))
	protected.DELETE("/users/:userID", userHandler.DeleteUser, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/roles", userHandler.ListRoles, appMiddleware.RequireAnyPermission("users.manage"))
	protected.PUT("/roles/:roleID/permissions", userHandler.UpdateRolePermissions, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/permissions", userHandler.ListPermissions, appMiddleware.RequireAnyPermission("users.manage"))

	protected.GET("/products", productHandler.List, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.POST("/products", productHandler.CreateProduct, appMiddleware.RequireAnyPermission("products.manage"))
	protected.PUT("/products/:productID", productHandler.UpdateProduct, appMiddleware.RequireAnyPermission("products.manage"))
	protected.GET("/products/:productID/units", productHandler.ListUnits, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.PUT("/products/:productID/units", productHandler.SaveUnits, appMiddleware.RequireAnyPermission("products.manage"))

	protected.GET("/promotions", promotionHandler.List, appMiddleware.RequireAnyPermission("promotion.view", "promotion.manage"))
	protected.POST("/promotions", promotionHandler.Create, appMiddleware.RequireAnyPermission("promotion.manage"))
	protected.PUT("/promotions/:promotionID", promotionHandler.Update, appMiddleware.RequireAnyPermission("promotion.manage"))
	protected.DELETE("/promotions/:promotionID", promotionHandler.Delete, appMiddleware.RequireAnyPermission("promotion.manage"))
	protected.GET("/products/:productID/branch-settings/:branchID", productHandler.GetBranchSettings, appMiddleware.RequireAnyPermission("products.manage"))
	protected.PUT("/products/:productID/branch-settings/:branchID", productHandler.UpdateBranchSettings, appMiddleware.RequireAnyPermission("products.manage"))
	protected.DELETE("/products/:productID", productHandler.DeleteProduct, superadminOnly)
	protected.GET("/products/:productID/deletion-impact", productHandler.ProductDeletionImpact, superadminOnly)
	protected.POST("/products/:productID/image", productHandler.UploadProductImage, appMiddleware.RequireAnyPermission("products.manage"))
	protected.GET("/products/:productID/image", productHandler.ProductImage, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.GET("/products/:productID/images", productHandler.ListProductImages, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.GET("/products/:productID/images/:imageID", productHandler.ProductGalleryImage, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.DELETE("/products/:productID/image", productHandler.DeleteProductImage, appMiddleware.RequireAnyPermission("products.manage"))
	protected.GET("/product-categories", productHandler.ListCategories, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.POST("/product-categories", productHandler.CreateCategory, appMiddleware.RequireAnyPermission("products.manage"))
	protected.PUT("/product-categories/:categoryID", productHandler.UpdateCategory, appMiddleware.RequireAnyPermission("products.manage"))
	protected.DELETE("/product-categories/:categoryID", productHandler.DeleteCategory, appMiddleware.RequireAnyPermission("products.manage"))
	protected.GET("/aliases", productHandler.ListAliases, appMiddleware.RequireAnyPermission("products.view", "government.use", "government.manage_alias", "invoice.create.pos", "quotation.manage"))
	protected.POST("/aliases", productHandler.CreateAlias, appMiddleware.RequireAnyPermission("government.manage_alias"))
	protected.PUT("/aliases/:aliasID", productHandler.UpdateAlias, appMiddleware.RequireAnyPermission("government.manage_alias"))
	protected.DELETE("/aliases/:aliasID", productHandler.DeleteAlias, appMiddleware.RequireAnyPermission("government.manage_alias"))

	protected.GET("/suppliers", purchasingHandler.ListSuppliers, appMiddleware.RequireAnyPermission("suppliers.view.global", "suppliers.manage.global"))
	protected.POST("/suppliers", purchasingHandler.CreateSupplier, appMiddleware.RequireAnyPermission("suppliers.manage.global"))
	protected.GET("/suppliers/:supplierID", purchasingHandler.GetSupplier, appMiddleware.RequireAnyPermission("suppliers.view.global", "suppliers.manage.global"))
	protected.PUT("/suppliers/:supplierID", purchasingHandler.UpdateSupplier, appMiddleware.RequireAnyPermission("suppliers.manage.global"))
	protected.GET("/suppliers/:supplierID/deletion-impact", purchasingHandler.SupplierDeletionImpact, appMiddleware.RequireAnyPermission("suppliers.manage.global"))
	protected.DELETE("/suppliers/:supplierID", purchasingHandler.DeleteSupplier, appMiddleware.RequireAnyPermission("suppliers.manage.global"))

	protected.GET("/purchase-orders", purchasingHandler.ListPurchaseOrders, appMiddleware.RequireAnyPermission("purchase_orders.view.global", "purchase_orders.manage.global"))
	protected.POST("/purchase-orders", purchasingHandler.CreatePurchaseOrder, appMiddleware.RequireAnyPermission("purchase_orders.manage.global"))
	protected.GET("/purchase-orders/product-options", purchasingHandler.ProductOptions, appMiddleware.RequireAnyPermission("purchase_orders.manage.global"))
	protected.GET("/purchase-orders/:purchaseOrderID", purchasingHandler.GetPurchaseOrder, appMiddleware.RequireAnyPermission("purchase_orders.view.global", "purchase_orders.manage.global"))
	protected.PUT("/purchase-orders/:purchaseOrderID", purchasingHandler.UpdatePurchaseOrder, appMiddleware.RequireAnyPermission("purchase_orders.manage.global"))
	protected.POST("/purchase-orders/:purchaseOrderID/cancel", purchasingHandler.CancelPurchaseOrder, appMiddleware.RequireAnyPermission("purchase_orders.manage.global"))

	protected.GET("/inventory", inventoryHandler.List, appMiddleware.RequireAnyPermission("inventory.view.branch", "inventory.manage.global"))
	protected.GET("/inventory/lots", inventoryHandler.ListLots, appMiddleware.RequireAnyPermission("inventory.manage.global"))
	protected.GET("/inventory/stock-adjustment-notes", inventoryHandler.ListStockAdjustments)
	protected.GET("/inventory/movements", inventoryHandler.ListMovementHistory)
	protected.POST("/inventory/rebalance", inventoryHandler.Rebalance, appMiddleware.RequireAnyPermission("inventory.rebalance"))
	protected.POST("/inventory/adjust", inventoryHandler.Adjust, appMiddleware.RequireAnyPermission("inventory.manage.global"))
	protected.POST("/inventory/receive", inventoryHandler.Receive, appMiddleware.RequireAnyPermission("inventory.receive"))
	protected.GET("/stock-transfer-requests", inventoryHandler.ListTransferRequests, appMiddleware.RequireAnyPermission("transfer.request.branch", "transfer.approve"))
	protected.POST("/stock-transfer-requests", inventoryHandler.CreateTransferRequest, appMiddleware.RequireAnyPermission("transfer.request.branch", "transfer.approve"))
	protected.POST("/stock-transfer-requests/:requestID/review", inventoryHandler.ReviewTransferRequest, appMiddleware.RequireAnyPermission("transfer.approve"))

	protected.POST("/quotations/preview", salesHandler.PreviewQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.GET("/quotations", salesHandler.ListQuotations, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.GET("/quotations/:quotationID", salesHandler.GetQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.POST("/quotations", salesHandler.CreateQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.POST("/quotations/:quotationID/convert", salesHandler.ConvertQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.GET("/quotations/:quotationID/deletion-impact", salesHandler.QuotationDeletionImpact, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.DELETE("/quotations/:quotationID", salesHandler.DeleteQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))

	protected.POST("/invoices/preview", salesHandler.PreviewInvoice, appMiddleware.RequireAnyPermission("quotation.manage", "invoice.create.pos"))
	protected.GET("/invoices", salesHandler.ListInvoices, appMiddleware.RequireAnyPermission("invoice.view"))
	protected.GET("/invoices/:invoiceID", salesHandler.GetInvoice, appMiddleware.RequireAnyPermission("invoice.view"))
	protected.GET("/invoices/:invoiceID/print", salesHandler.GetInvoicePrint, appMiddleware.RequireAnyPermission("invoice.view", "invoice.reprint"))
	protected.POST("/invoices", salesHandler.CreateInvoice, appMiddleware.RequireAnyPermission("quotation.manage", "invoice.create.pos"))
	protected.GET("/sales/lot-options", salesHandler.LotOptions, appMiddleware.RequireAnyPermission("quotation.manage", "invoice.create.pos"))
	protected.POST("/invoices/:invoiceID/pay", salesHandler.CollectPayment, appMiddleware.RequireAnyPermission("payment.collect"))
	protected.GET("/invoices/:invoiceID/deletion-impact", salesHandler.InvoiceDeletionImpact, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.DELETE("/invoices/:invoiceID", salesHandler.DeleteInvoice, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.POST("/pos/preview", salesHandler.PreviewCheckout, appMiddleware.RequireAnyPermission("invoice.create.pos"))
	protected.POST("/admin/pos/preview", salesHandler.PreviewCheckout, appMiddleware.RequireAnyPermission("invoice.create.remote"))
	// พักบิล — intentionally has NO RequireAnyPermission gate: parking is
	// plain POS counter behaviour, not a privileged action, and branch_pos
	// holds no document permissions. Access is bounded instead by the
	// caller's own branch (see branchOf), so an account can only ever see
	// and clear parked bills at the branch it belongs to.
	protected.GET("/parked-bills", parkedBillHandler.List)
	protected.POST("/parked-bills", parkedBillHandler.Create)
	protected.GET("/parked-bills/:parkedBillID", parkedBillHandler.Get)
	protected.DELETE("/parked-bills/:parkedBillID", parkedBillHandler.Delete)
	protected.POST("/pos/checkout", salesHandler.Checkout, appMiddleware.RequireAnyPermission("invoice.create.pos", "payment.collect"))
	protected.POST("/admin/pos/checkout", salesHandler.AdminCheckout, appMiddleware.RequireAnyPermission("invoice.create.remote"))
	// รีโมตหน้าร้าน: head office keeps the cart here, the branch till reads it
	// and takes the money. State, not clicks — see modules/sales/remote_sessions.go.
	protected.PUT("/admin/pos/remote-session", salesHandler.SaveRemoteSession, appMiddleware.RequireAnyPermission("invoice.create.remote"))
	protected.DELETE("/admin/pos/remote-session", salesHandler.CancelRemoteSession, appMiddleware.RequireAnyPermission("invoice.create.remote"))
	protected.GET("/admin/pos/remote-session", salesHandler.AdminRemoteSession, appMiddleware.RequireAnyPermission("invoice.create.remote"))
	protected.GET("/pos/remote-session", salesHandler.PosRemoteSession, appMiddleware.RequireAnyPermission("invoice.create.pos"))
	protected.POST("/pos/remote-session/checkout", salesHandler.CheckoutRemoteSession, appMiddleware.RequireAnyPermission("invoice.create.pos"))

	protected.GET("/transfers", transferHandler.List, appMiddleware.RequireAnyPermission("transfer.request", "transfer.receive", "transfer.approve"))
	protected.POST("/transfers", transferHandler.Create, appMiddleware.RequireAnyPermission("transfer.request"))
	protected.POST("/transfers/:transferID/dispatch", transferHandler.Dispatch, appMiddleware.RequireAnyPermission("transfer.dispatch", "transfer.approve"))
	protected.POST("/transfers/:transferID/receive", transferHandler.Receive, appMiddleware.RequireAnyPermission("transfer.receive", "transfer.approve"))

	monthEndOnly := superadminOnly
	protected.GET("/admin/month-end-report", monthEndHandler.MonthEndReport, monthEndOnly)
	protected.POST("/accounting/month-end/reconciliation-overview", monthEndHandler.ReconciliationOverview, monthEndOnly)
	protected.POST("/accounting/month-end/reconciliation-preview", monthEndHandler.PreviewReconciliation, monthEndOnly)
	protected.POST("/accounting/month-end/reconciliations", monthEndHandler.FinalizeReconciliation, monthEndOnly)
	protected.GET("/accounting/month-end/reconciliations", monthEndHandler.ListReconciliations, monthEndOnly)
	protected.GET("/accounting/month-end/reconciliations/:reconciliationID", monthEndHandler.GetReconciliation, monthEndOnly)
	protected.POST("/accounting/month-end/preview", monthEndHandler.Preview, monthEndOnly)
	protected.GET("/accounting/month-end/source", monthEndHandler.Source, monthEndOnly)
	protected.GET("/accounting/month-end", monthEndHandler.List, monthEndOnly)
	protected.POST("/accounting/month-end", monthEndHandler.Create, monthEndOnly)
	protected.GET("/accounting/month-end/:workpaperID", monthEndHandler.Get, monthEndOnly)
	protected.POST("/accounting/month-end/:workpaperID/finalize", monthEndHandler.Finalize, monthEndOnly)
	protected.POST("/accounting/month-end/periods", monthEndWorkflowHandler.Create, monthEndOnly)
	protected.GET("/accounting/month-end/periods/:periodID", monthEndWorkflowHandler.Get, monthEndOnly)
	protected.PATCH("/accounting/month-end/periods/:periodID", monthEndWorkflowHandler.Update, monthEndOnly)
	protected.DELETE("/accounting/month-end/periods/:periodID", monthEndWorkflowHandler.CancelDraft, monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/validate", monthEndWorkflowHandler.Validate, monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/calculate", monthEndWorkflowHandler.Calculate, monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/recalculate", monthEndWorkflowHandler.Calculate, monthEndOnly)
	protected.PATCH("/accounting/month-end/periods/:periodID/proposals/:lineID", monthEndWorkflowHandler.Toggle, monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/submit", monthEndWorkflowHandler.Status("submit"), monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/approve", monthEndWorkflowHandler.Status("approve"), monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/close", monthEndWorkflowHandler.Close, monthEndOnly)
	protected.POST("/accounting/month-end/periods/:periodID/reopen", monthEndWorkflowHandler.Reopen, monthEndOnly)
	protected.GET("/accounting/month-end/periods/:periodID/audit", monthEndWorkflowHandler.Audit, monthEndOnly)
	protected.GET("/accounting/month-end/periods/:periodID/export", monthEndWorkflowHandler.Export, monthEndOnly)

	protected.GET("/reports/tax", reportHandler.Tax, appMiddleware.RequireAnyPermission("reports.view.global"))
	protected.GET("/reports/profit-loss", reportHandler.ProfitLoss, appMiddleware.RequireAnyPermission("reports.view.global"))

	protected.GET("/marketplace/providers", marketplaceHandler.ListProviders, appMiddleware.RequireAnyPermission("marketplace.manage.global", "marketplace.view.branch"))
	protected.GET("/marketplace/orders", marketplaceHandler.ListOrders, appMiddleware.RequireAnyPermission("marketplace.manage.global", "marketplace.view.branch"))
	protected.POST("/marketplace/connections", marketplaceHandler.UpsertConnection, appMiddleware.RequireAnyPermission("marketplace.manage.global"))
	protected.POST("/marketplace/test-connection", marketplaceHandler.TestConnection, appMiddleware.RequireAnyPermission("marketplace.manage.global"))

	protected.GET("/audit-logs", auditHandler.List, appMiddleware.RequireAnyPermission("audit.view.global"))

	protected.GET("/fda-reports/summary", fdaHandler.Summary, appMiddleware.RequireAnyPermission("fda.manage"))

	protected.POST("/product-returns", returnsHandler.Initiate, appMiddleware.RequireAnyPermission("invoice.create.pos"))
	protected.GET("/product-returns", returnsHandler.List, appMiddleware.RequireAnyPermission("returns.manage", "invoice.view"))
	protected.POST("/product-returns/:returnID/send-to-supplier", returnsHandler.SendToSupplier, appMiddleware.RequireAnyPermission("returns.manage"))
	protected.POST("/product-returns/:returnID/resolve-case-a", returnsHandler.ResolveCaseA, appMiddleware.RequireAnyPermission("returns.manage"))
	protected.POST("/product-returns/:returnID/resolve-case-b", returnsHandler.ResolveCaseB, appMiddleware.RequireAnyPermission("returns.manage"))
	protected.POST("/product-returns/:returnID/reject", returnsHandler.Reject, appMiddleware.RequireAnyPermission("returns.manage"))

	return &Server{engine: engine}
}

func (s *Server) Start(address string) error {
	return s.engine.Start(address)
}
