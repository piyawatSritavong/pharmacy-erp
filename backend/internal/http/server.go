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
	"pharmacy-erp/backend/internal/modules/finance"
	"pharmacy-erp/backend/internal/modules/inventory"
	"pharmacy-erp/backend/internal/modules/marketplace"
	"pharmacy-erp/backend/internal/modules/products"
	"pharmacy-erp/backend/internal/modules/reports"
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
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,
	}))

	auditService := audit.NewService(db)
	authHandler := auth.NewHandler(auth.NewService(db, cfg))
	branchHandler := branches.NewHandler(branches.NewService(db, auditService))
	userHandler := users.NewHandler(users.NewService(db, auditService))
	dashboardHandler := dashboard.NewHandler(dashboard.NewService(db))
	productHandler := products.NewHandler(products.NewService(db, auditService))
	inventoryHandler := inventory.NewHandler(inventory.NewService(db, auditService))
	salesHandler := sales.NewHandler(sales.NewService(db, auditService))
	transferHandler := transfers.NewHandler(transfers.NewService(db, auditService))
	financeHandler := finance.NewHandler(finance.NewService(db, auditService))
	reportHandler := reports.NewHandler(reports.NewService(db))
	marketplaceHandler := marketplace.NewHandler(marketplace.NewService(db, auditService))
	auditHandler := audit.NewHandler(auditService)

	api := engine.Group("/api/v1")
	api.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
	})
	api.POST("/auth/login", authHandler.Login)
	api.POST("/auth/logout", authHandler.Logout)

	protected := api.Group("")
	protected.Use(appMiddleware.JWT(cfg.JWTSecret))
	protected.GET("/me", authHandler.Me)
	protected.GET("/dashboard", dashboardHandler.Summary, appMiddleware.RequireAnyPermission("dashboard.view.global", "dashboard.view.branch", "dashboard.view.self"))
	protected.GET("/dashboard/daily-sales", dashboardHandler.DailySales, appMiddleware.RequireAnyPermission("dashboard.view.self"))

	protected.GET("/branches", branchHandler.List)
	protected.POST("/branches", branchHandler.Create, appMiddleware.RequireAnyPermission("settings.manage"))
	protected.PUT("/branches/:branchID", branchHandler.Update, appMiddleware.RequireAnyPermission("settings.manage"))
	protected.GET("/branches/sequences", branchHandler.ListSequences, appMiddleware.RequireAnyPermission("settings.manage", "invoice.sequence.manage"))
	protected.PUT("/branches/:branchID/sequences/:docType", branchHandler.UpdateSequence, appMiddleware.RequireAnyPermission("settings.manage", "invoice.sequence.manage"))

	protected.GET("/users", userHandler.ListUsers, appMiddleware.RequireAnyPermission("users.manage"))
	protected.POST("/users", userHandler.CreateUser, appMiddleware.RequireAnyPermission("users.manage"))
	protected.PUT("/users/:userID", userHandler.UpdateUser, appMiddleware.RequireAnyPermission("users.manage"))
	protected.POST("/users/:userID/reset-password", userHandler.ResetPassword, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/users/roles", userHandler.ListRoles, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/roles", userHandler.ListRoles, appMiddleware.RequireAnyPermission("users.manage"))
	protected.POST("/roles", userHandler.CreateRole, appMiddleware.RequireAnyPermission("users.manage"))
	protected.PUT("/roles/:roleID", userHandler.UpdateRole, appMiddleware.RequireAnyPermission("users.manage"))
	protected.PUT("/roles/:roleID/permissions", userHandler.UpdateRolePermissions, appMiddleware.RequireAnyPermission("users.manage"))
	protected.GET("/permissions", userHandler.ListPermissions, appMiddleware.RequireAnyPermission("users.manage"))

	protected.GET("/products", productHandler.List, appMiddleware.RequireAnyPermission("products.view", "products.manage"))
	protected.POST("/products", productHandler.CreateProduct, appMiddleware.RequireAnyPermission("products.manage"))
	protected.PUT("/products/:productID", productHandler.UpdateProduct, appMiddleware.RequireAnyPermission("products.manage"))
	protected.GET("/aliases", productHandler.ListAliases, appMiddleware.RequireAnyPermission("products.view", "government.use", "government.manage_alias", "invoice.create.branch", "invoice.create.pos", "quotation.manage"))
	protected.POST("/aliases", productHandler.CreateAlias, appMiddleware.RequireAnyPermission("government.manage_alias"))

	protected.GET("/inventory", inventoryHandler.List, appMiddleware.RequireAnyPermission("inventory.view.branch", "inventory.manage.branch", "inventory.manage.global"))
	protected.POST("/inventory/rebalance", inventoryHandler.Rebalance, appMiddleware.RequireAnyPermission("inventory.rebalance"))
	protected.POST("/inventory/adjust", inventoryHandler.Adjust, appMiddleware.RequireAnyPermission("inventory.manage.global", "inventory.manage.branch"))

	protected.POST("/quotations/preview", salesHandler.PreviewQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.GET("/quotations", salesHandler.ListQuotations, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.POST("/quotations", salesHandler.CreateQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))
	protected.POST("/quotations/:quotationID/convert", salesHandler.ConvertQuotation, appMiddleware.RequireAnyPermission("quotation.manage"))

	protected.POST("/invoices/preview", salesHandler.PreviewInvoice, appMiddleware.RequireAnyPermission("invoice.create.branch", "invoice.create.pos"))
	protected.GET("/invoices", salesHandler.ListInvoices, appMiddleware.RequireAnyPermission("invoice.view"))
	protected.GET("/invoices/:invoiceID", salesHandler.GetInvoice, appMiddleware.RequireAnyPermission("invoice.view"))
	protected.GET("/invoices/:invoiceID/print", salesHandler.GetInvoicePrint, appMiddleware.RequireAnyPermission("invoice.view", "invoice.reprint"))
	protected.POST("/invoices", salesHandler.CreateInvoice, appMiddleware.RequireAnyPermission("invoice.create.branch", "invoice.create.pos"))
	protected.POST("/invoices/:invoiceID/pay", salesHandler.CollectPayment, appMiddleware.RequireAnyPermission("payment.collect"))

	protected.GET("/transfers", transferHandler.List, appMiddleware.RequireAnyPermission("transfer.request", "transfer.receive", "transfer.approve"))
	protected.POST("/transfers", transferHandler.Create, appMiddleware.RequireAnyPermission("transfer.request"))
	protected.POST("/transfers/:transferID/dispatch", transferHandler.Dispatch, appMiddleware.RequireAnyPermission("transfer.dispatch", "transfer.approve"))
	protected.POST("/transfers/:transferID/receive", transferHandler.Receive, appMiddleware.RequireAnyPermission("transfer.receive", "transfer.approve"))
	protected.POST("/transfers/receive-by-code", transferHandler.ReceiveByCode, appMiddleware.RequireAnyPermission("transfer.receive", "transfer.approve"))

	protected.GET("/checks", financeHandler.ListChecks, appMiddleware.RequireAnyPermission("finance.manage.global", "finance.manage.branch"))
	protected.GET("/checks/outstanding-invoices", financeHandler.ListOutstandingInvoices, appMiddleware.RequireAnyPermission("finance.manage.global", "finance.manage.branch"))
	protected.POST("/checks/preview-apply", financeHandler.PreviewApply, appMiddleware.RequireAnyPermission("finance.manage.global", "finance.manage.branch"))
	protected.POST("/checks", financeHandler.CreateCheck, appMiddleware.RequireAnyPermission("finance.manage.global", "finance.manage.branch"))

	protected.GET("/reports/tax", reportHandler.Tax, appMiddleware.RequireAnyPermission("reports.view.global"))
	protected.GET("/reports/profit-loss", reportHandler.ProfitLoss, appMiddleware.RequireAnyPermission("reports.view.global"))

	protected.GET("/marketplace/providers", marketplaceHandler.ListProviders, appMiddleware.RequireAnyPermission("marketplace.manage.global", "marketplace.view.branch"))
	protected.GET("/marketplace/orders", marketplaceHandler.ListOrders, appMiddleware.RequireAnyPermission("marketplace.manage.global", "marketplace.view.branch"))
	protected.POST("/marketplace/connections", marketplaceHandler.UpsertConnection, appMiddleware.RequireAnyPermission("marketplace.manage.global"))

	protected.GET("/audit-logs", auditHandler.List, appMiddleware.RequireAnyPermission("audit.view.global"))

	return &Server{engine: engine}
}

func (s *Server) Start(address string) error {
	return s.engine.Start(address)
}
