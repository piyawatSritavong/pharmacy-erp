package inventory

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type StockAdjustmentFilter struct {
	BranchID         string
	ProductID        string
	InvoiceID        string
	ReconciliationID string
	Page             int
	PageSize         int
}

type MovementHistoryFilter struct {
	BranchID    string
	ProductID   string
	StockBucket string
	Page        int
	PageSize    int
}

func (s *Service) ListMovementHistory(ctx context.Context, user platform.AuthUser, filter MovementHistoryFilter) (ListResult, error) {
	if user.Portal == "pos" || user.RoleKey == "branch_pos" {
		return ListResult{}, platform.NewError(http.StatusForbidden, "บัญชี POS ไม่มีสิทธิ์ดูประวัติ movement ภายใน")
	}
	filter.StockBucket = strings.TrimSpace(filter.StockBucket)
	if filter.StockBucket != "" && filter.StockBucket != "real" && filter.StockBucket != "ghost" {
		return ListResult{}, platform.NewError(http.StatusBadRequest, "invalid stock bucket")
	}
	if filter.StockBucket == "ghost" && user.RoleKey != "super_admin" {
		return ListResult{}, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์เข้าถึงสต๊อกผี")
	}
	branchID, err := platform.BranchFilter(user, filter.BranchID)
	if err != nil {
		return ListResult{}, err
	}
	filter.BranchID = branchID
	args := []any{}
	conditions := []string{}
	add := func(template string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(template, len(args)))
	}
	if user.RoleKey != "super_admin" {
		conditions = append(conditions,
			"movement.stock_bucket='real'",
			"movement.movement_type<>'month_end_sale_source_reversal'",
			"NOT (movement.reference_type='month_end_reconciliation')",
			"NOT (movement.reference_type='invoice' AND EXISTS (SELECT 1 FROM invoices hidden WHERE hidden.id=movement.reference_id AND hidden.deleted_at IS NOT NULL))",
		)
	}
	if value := strings.TrimSpace(filter.BranchID); value != "" {
		add("movement.branch_id=$%d", value)
	}
	if value := strings.TrimSpace(filter.ProductID); value != "" {
		add("movement.product_id=$%d", value)
	}
	if filter.StockBucket != "" {
		add("movement.stock_bucket=$%d", filter.StockBucket)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory_movements movement`+where, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT movement.id::text,movement.branch_id::text,branch.name,movement.product_id::text,
		       product.sku,product.name,movement.movement_type,movement.stock_bucket,
		       movement.quantity_delta,movement.reference_type,COALESCE(movement.reference_id::text,''),
		       movement.note,COALESCE(actor.full_name,''),movement.created_at
		FROM inventory_movements movement
		INNER JOIN branches branch ON branch.id=movement.branch_id
		INNER JOIN products product ON product.id=movement.product_id
		LEFT JOIN users actor ON actor.id=movement.performed_by
	`+where+` ORDER BY movement.created_at DESC,movement.id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), queryArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, branchID, branchName, productID, sku, productName, movementType, bucket, referenceType, referenceID, note, actorName string
		var quantity int
		var createdAt any
		if err := rows.Scan(&id, &branchID, &branchName, &productID, &sku, &productName, &movementType, &bucket,
			&quantity, &referenceType, &referenceID, &note, &actorName, &createdAt); err != nil {
			return ListResult{}, err
		}
		items = append(items, map[string]any{
			"id": id, "branch_id": branchID, "branch_name": branchName, "product_id": productID,
			"sku": sku, "product_name": productName, "movement_type": movementType,
			"stock_bucket": bucket, "quantity_delta": quantity, "reference_type": referenceType,
			"reference_id": referenceID, "note": note, "performed_by_name": actorName, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (s *Service) ListStockAdjustments(ctx context.Context, user platform.AuthUser, filter StockAdjustmentFilter) (ListResult, error) {
	if user.Portal == "pos" || user.RoleKey == "branch_pos" {
		return ListResult{}, platform.NewError(http.StatusForbidden, "บัญชี POS ไม่มีสิทธิ์ดูประวัติการปรับสต๊อก")
	}
	branchID, err := platform.BranchFilter(user, filter.BranchID)
	if err != nil {
		return ListResult{}, err
	}
	filter.BranchID = branchID

	args := []any{}
	conditions := []string{}
	add := func(template string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(template, len(args)))
	}
	if user.RoleKey != "super_admin" {
		conditions = append(conditions, "note.stock_type='REAL'")
	}
	if value := strings.TrimSpace(filter.BranchID); value != "" {
		add("note.branch_id=$%d", value)
	}
	if value := strings.TrimSpace(filter.ProductID); value != "" {
		add("note.product_id=$%d", value)
	}
	if value := strings.TrimSpace(filter.InvoiceID); value != "" {
		add("note.reference_invoice_id=$%d", value)
	}
	if value := strings.TrimSpace(filter.ReconciliationID); value != "" {
		add("note.reconciliation_id=$%d", value)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_adjustment_notes note`+where, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT note.id::text,note.branch_id::text,branch.name,note.product_id::text,
		       product.sku,product.name,note.quantity,note.stock_type,note.reason,
		       COALESCE(note.reference_invoice_id::text,''),COALESCE(invoice.invoice_number,''),
		       COALESCE(note.reconciliation_id::text,''),COALESCE(reconciliation.reconciliation_number,''),
		       COALESCE(note.inventory_movement_id::text,''),COALESCE(movement.movement_type,''),
		       actor.full_name,note.created_at
		FROM stock_adjustment_notes note
		INNER JOIN branches branch ON branch.id=note.branch_id
		INNER JOIN products product ON product.id=note.product_id
		INNER JOIN users actor ON actor.id=note.created_by
		LEFT JOIN invoices invoice ON invoice.id=note.reference_invoice_id
		LEFT JOIN month_end_reconciliations reconciliation ON reconciliation.id=note.reconciliation_id
		LEFT JOIN inventory_movements movement ON movement.id=note.inventory_movement_id
	`+where+` ORDER BY note.created_at DESC,note.id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), queryArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, branchID, branchName, productID, sku, productName, stockType, reason string
		var invoiceID, invoiceNumber, reconciliationID, reconciliationNumber, movementID, movementType, actorName string
		var quantity int
		var createdAt any
		if err := rows.Scan(&id, &branchID, &branchName, &productID, &sku, &productName, &quantity, &stockType, &reason,
			&invoiceID, &invoiceNumber, &reconciliationID, &reconciliationNumber, &movementID, &movementType, &actorName, &createdAt); err != nil {
			return ListResult{}, err
		}
		items = append(items, map[string]any{
			"id": id, "branch_id": branchID, "branch_name": branchName,
			"product_id": productID, "sku": sku, "product_name": productName,
			"quantity": quantity, "stock_type": stockType, "reason": reason,
			"reference_invoice_id": invoiceID, "invoice_number": invoiceNumber,
			"reconciliation_id": reconciliationID, "reconciliation_number": reconciliationNumber,
			"inventory_movement_id": movementID, "movement_type": movementType,
			"created_by_name": actorName, "created_at": createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (h *Handler) ListStockAdjustments(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	user := platform.CurrentUser(c)
	branchID, err := platform.BranchFilter(user, strings.TrimSpace(c.QueryParam("branch_id")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result, err := h.service.ListStockAdjustments(c.Request().Context(), user, StockAdjustmentFilter{
		BranchID: branchID, ProductID: strings.TrimSpace(c.QueryParam("product_id")),
		InvoiceID: strings.TrimSpace(c.QueryParam("invoice_id")), ReconciliationID: strings.TrimSpace(c.QueryParam("reconciliation_id")),
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{
		"items":      result.Items,
		"pagination": map[string]any{"page": result.Page, "page_size": result.PageSize, "total": result.Total, "total_pages": result.TotalPages},
	})
}

func (h *Handler) ListMovementHistory(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	user := platform.CurrentUser(c)
	branchID, err := platform.BranchFilter(user, strings.TrimSpace(c.QueryParam("branch_id")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result, err := h.service.ListMovementHistory(c.Request().Context(), user, MovementHistoryFilter{
		BranchID: branchID, ProductID: strings.TrimSpace(c.QueryParam("product_id")),
		StockBucket: strings.TrimSpace(c.QueryParam("stock_bucket")),
		Page:        page, PageSize: pageSize,
	})
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{
		"items":      result.Items,
		"pagination": map[string]any{"page": result.Page, "page_size": result.PageSize, "total": result.Total, "total_pages": result.TotalPages},
	})
}
