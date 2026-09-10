package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type RebalanceRequest struct {
	ProductID  string `json:"product_id"`
	BranchID   string `json:"branch_id"`
	Quantity   int    `json:"quantity"`
	FromBucket string `json:"from_bucket"`
	ToBucket   string `json:"to_bucket"`
	Reason     string `json:"reason"`
}

type AdjustRequest struct {
	ProductID   string `json:"product_id"`
	BranchID    string `json:"branch_id"`
	StockBucket string `json:"stock_bucket"`
	// InventoryLotID targets one specific lot instead of letting the service
	// choose. Optional: left blank, a decrease still consumes FEFO and an
	// increase opens a new lot, which is what receiving and corrections want.
	// The branch-settings screen sets it, because a stock count is always a
	// count of one physical lot on one shelf.
	InventoryLotID string  `json:"inventory_lot_id"`
	QuantityDelta  int     `json:"quantity_delta"`
	Reason         string  `json:"reason"`
	LotNumber      string  `json:"lot_number"`
	ExpiresOn      string  `json:"expires_on"`
	UnitCost       float64 `json:"unit_cost"`
}

type ReceiveRequest struct {
	ProductID     string  `json:"product_id"`
	BranchID      string  `json:"branch_id"`
	RealQuantity  int     `json:"real_quantity"`
	GhostQuantity int     `json:"ghost_quantity"`
	Note          string  `json:"note"`
	LotNumber     string  `json:"lot_number"`
	ExpiresOn     string  `json:"expires_on"`
	UnitCost      float64 `json:"unit_cost"`
}

type CreateReceiptRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Note      string `json:"note"`
}

type ReviewReceiptRequest struct {
	Decision    string `json:"decision"`
	Quantity    int    `json:"quantity"`
	StockBucket string `json:"stock_bucket"`
	ReviewNote  string `json:"review_note"`
}

type ListFilter struct {
	Search   string
	Page     int
	PageSize int
}

type ListResult struct {
	Items      []map[string]any
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string, filter ListFilter) (ListResult, error) {
	branchID, err := platform.BranchFilter(user, branchID)
	if err != nil {
		return ListResult{}, err
	}
	args := []any{}
	conditions := []string{}
	if branchID != "" {
		args = append(args, branchID)
		conditions = append(conditions, "i.branch_id = $"+strconv.Itoa(len(args)))
	}
	if keyword := strings.TrimSpace(filter.Search); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		placeholder := "$" + strconv.Itoa(len(args))
		conditions = append(conditions, fmt.Sprintf("(LOWER(p.name) LIKE %s OR LOWER(p.sku) LIKE %s OR LOWER(b.name) LIKE %s)", placeholder, placeholder, placeholder))
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory i
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN products p ON p.id = i.product_id
		`+whereClause, args...).Scan(&total); err != nil {
		return ListResult{}, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	limitClause := ""
	if pageSize > 0 {
		if pageSize > 200 {
			pageSize = 200
		}
		args = append(args, pageSize, (page-1)*pageSize)
		limitClause = fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	lotScope := ""
	ghostQuantityExpression := "i.qty_ghost"
	ghostThresholdExpression := "COALESCE(bps.low_stock_ghost_threshold,p.low_stock_ghost_threshold)"
	if user.RoleKey != "super_admin" {
		lotScope = " WHERE stock_bucket='real'"
		ghostQuantityExpression = "0"
		ghostThresholdExpression = "0"
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH lot_summary AS (
			SELECT branch_id, product_id,
				COALESCE(SUM(remaining_quantity) FILTER (WHERE stock_bucket='real' AND (expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)),0)::bigint AS sellable_real,
				COALESCE(SUM(remaining_quantity) FILTER (WHERE stock_bucket='ghost' AND (expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)),0)::bigint AS sellable_ghost,
				COALESCE(SUM(remaining_quantity) FILTER (WHERE stock_bucket='real' AND expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date),0)::bigint AS expired_real,
				COALESCE(SUM(remaining_quantity) FILTER (WHERE stock_bucket='ghost' AND expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date),0)::bigint AS expired_ghost,
				MIN(expires_on) FILTER (WHERE stock_bucket='real' AND remaining_quantity>0 AND expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date) AS nearest_expiry_real,
				MIN(expires_on) FILTER (WHERE stock_bucket='ghost' AND remaining_quantity>0 AND expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date) AS nearest_expiry_ghost
			FROM inventory_lots`+lotScope+` GROUP BY branch_id, product_id
			)
		SELECT i.id, i.branch_id::text, b.name, p.id::text, p.sku, p.name, i.qty_real, `+ghostQuantityExpression+`,
		       p.cost_price, p.base_selling_price, COALESCE(bpp.selling_price, p.base_selling_price),
		       COALESCE(bps.max_discount_amount,p.max_discount_amount),
		       COALESCE(bps.low_stock_real_threshold,p.low_stock_real_threshold),
		       `+ghostThresholdExpression+`,
		       p.tracks_expiry,p.expiry_warning_days,
		       COALESCE(ls.sellable_real,0),COALESCE(ls.sellable_ghost,0),
		       COALESCE(ls.expired_real,0),COALESCE(ls.expired_ghost,0),
		       ls.nearest_expiry_real,ls.nearest_expiry_ghost,
		       COALESCE(p.image_storage_key,''),(SELECT COUNT(*) FROM product_images pi WHERE pi.product_id=p.id)
		FROM inventory i
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN products p ON p.id = i.product_id
		LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = i.branch_id
		LEFT JOIN branch_product_settings bps ON bps.product_id = p.id AND bps.branch_id = i.branch_id
		LEFT JOIN lot_summary ls ON ls.branch_id=i.branch_id AND ls.product_id=i.product_id
		`+whereClause+`
		ORDER BY b.name, p.name
		`+limitClause, args...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	items, err := scanInventory(rows, user.RoleKey == "super_admin", user.Portal != "pos" && user.RoleKey != "branch_pos")
	if err != nil {
		return ListResult{}, err
	}
	totalPages := 1
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
	} else {
		pageSize = total
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func scanInventory(rows *sql.Rows, includeGhost bool, includeExactQuantity bool) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		var id, branchID, branchName, productID, sku, productName string
		var qtyReal, qtyGhost int
		var costPrice, baseSellingPrice, price, maxDiscount float64
		var thresholdReal, thresholdGhost, warningDays, sellableReal, sellableGhost, expiredReal, expiredGhost, imageCount int
		var imageKey string
		var tracksExpiry bool
		var nearestReal, nearestGhost sql.NullTime
		if err := rows.Scan(&id, &branchID, &branchName, &productID, &sku, &productName, &qtyReal, &qtyGhost,
			&costPrice, &baseSellingPrice, &price, &maxDiscount, &thresholdReal, &thresholdGhost,
			&tracksExpiry, &warningDays, &sellableReal, &sellableGhost, &expiredReal, &expiredGhost,
			&nearestReal, &nearestGhost, &imageKey, &imageCount); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":                       id,
			"branch_id":                branchID,
			"branch_name":              branchName,
			"product_id":               productID,
			"sku":                      sku,
			"product_name":             productName,
			"price":                    price,
			"low_stock_real_threshold": thresholdReal,
			"tracks_expiry":            tracksExpiry,
			"is_low_stock_real":        sellableReal <= thresholdReal,
			"availability_status":      inventoryAvailability(sellableReal, thresholdReal),
			"image_available":          imageKey != "",
			"image_count":              imageCount,
		}
		if includeExactQuantity {
			item["qty_real"] = qtyReal
			item["cost_price"] = costPrice
			item["base_selling_price"] = baseSellingPrice
			item["max_discount_amount"] = maxDiscount
			item["expiry_warning_days"] = warningDays
			item["sellable_qty_real"] = sellableReal
			item["expired_qty_real"] = expiredReal
			item["nearest_expiry_real"] = nullableTime(nearestReal)
		}
		if includeGhost {
			item["qty_ghost"] = qtyGhost
			item["low_stock_ghost_threshold"] = thresholdGhost
			item["sellable_qty_ghost"] = sellableGhost
			item["expired_qty_ghost"] = expiredGhost
			item["nearest_expiry_ghost"] = nullableTime(nearestGhost)
			item["is_low_stock_ghost"] = sellableGhost <= thresholdGhost
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func inventoryAvailability(sellable, threshold int) string {
	if sellable <= 0 {
		return "out_of_stock"
	}
	if sellable <= threshold {
		return "low_stock"
	}
	return "available"
}

func (s *Service) ListLots(ctx context.Context, user platform.AuthUser, branchID, productID, bucket string, includeDepleted bool) ([]map[string]any, error) {
	branchID, err := platform.MustBranchID(user, branchID)
	if err != nil {
		return nil, err
	}
	if bucket == "ghost" && user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์เข้าถึงสต๊อกผี")
	}
	if strings.TrimSpace(branchID) == "" || strings.TrimSpace(productID) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขาและสินค้า")
	}
	if err := stocklot.ValidateBucket(bucket); err != nil {
		return nil, err
	}
	if bucket == "ghost" {
		if err := ensureGhostWarehouse(ctx, s.db, branchID); err != nil {
			return nil, err
		}
	}
	condition := "AND il.remaining_quantity > 0"
	if includeDepleted {
		condition = ""
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT il.id::text,il.lot_number,il.stock_bucket,il.received_quantity,il.remaining_quantity,
		il.unit_cost,il.expires_on,il.received_at,il.source_type,COALESCE(po.po_number,''),
		CASE WHEN il.expires_on IS NOT NULL AND il.expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date THEN 'expired'
		WHEN il.expires_on IS NOT NULL AND il.expires_on <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date + p.expiry_warning_days THEN 'expiring' ELSE 'normal' END
		FROM inventory_lots il INNER JOIN products p ON p.id=il.product_id
		LEFT JOIN purchase_order_items poi ON poi.id=il.source_item_id LEFT JOIN purchase_orders po ON po.id=poi.purchase_order_id
		WHERE il.branch_id=$1 AND il.product_id=$2 AND il.stock_bucket=$3 `+condition+`
		ORDER BY il.expires_on ASC NULLS LAST,il.received_at,il.id
	`, branchID, productID, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, lotNumber, stockBucket, sourceType, poNumber, status string
		var received, remaining int
		var cost float64
		var expires sql.NullTime
		var receivedAt time.Time
		if err := rows.Scan(&id, &lotNumber, &stockBucket, &received, &remaining, &cost, &expires, &receivedAt, &sourceType, &poNumber, &status); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "lot_number": lotNumber, "stock_bucket": stockBucket, "received_quantity": received, "remaining_quantity": remaining, "unit_cost": cost, "expires_on": nullableTime(expires), "received_at": receivedAt, "source_type": sourceType, "po_number": poNumber, "expiry_status": status})
	}
	return items, rows.Err()
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func (s *Service) CreateReceiptRequest(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CreateReceiptRequest) (string, error) {
	// Note: not currently wired to any route (see server.go) — fixed for
	// consistency with the rest of the D11 sweep, not because it's reachable.
	if !platform.HasPermission(user, "inventory.receive") || user.BranchID == nil {
		return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ส่งคำขอรับสินค้า")
	}
	if strings.TrimSpace(input.ProductID) == "" || input.Quantity <= 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้าและระบุจำนวนมากกว่า 0")
	}

	requestID := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT i.qty_real, i.qty_ghost
			FROM inventory i
			INNER JOIN products p ON p.id = i.product_id AND p.active = TRUE
			WHERE i.branch_id = $1 AND i.product_id = $2
		`, *user.BranchID, input.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้าในสต๊อกของสาขานี้")
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_receipt_requests (
				id, branch_id, product_id, requested_quantity, snapshot_qty_real,
				snapshot_qty_ghost, note, status, requested_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, NOW(), NOW())
		`, requestID, *user.BranchID, input.ProductID, input.Quantity, qtyReal, qtyGhost, strings.TrimSpace(input.Note), user.ID); err != nil {
			if strings.Contains(err.Error(), "idx_inventory_receipt_requests_pending_product") {
				return platform.NewError(http.StatusConflict, "สินค้านี้มีคำขอรับเข้าที่รอตรวจสอบอยู่แล้ว")
			}
			return err
		}
		meta.EntityType = "inventory_receipt_request"
		meta.EntityID = &requestID
		meta.Action = "inventory.receipt_request.create"
		meta.After = map[string]any{"product_id": input.ProductID, "quantity": input.Quantity, "snapshot_qty_real": qtyReal}
		return s.audit.Log(ctx, tx, meta)
	})
	return requestID, err
}

func (s *Service) ListReceiptRequests(ctx context.Context, user platform.AuthUser, status string) ([]map[string]any, error) {
	query := `
		SELECT r.id::text, r.branch_id::text, b.name, r.product_id::text, p.sku, p.name,
		       r.requested_quantity, r.snapshot_qty_real, r.snapshot_qty_ghost,
		       COALESCE(i.qty_real, 0), COALESCE(i.qty_ghost, 0), r.note, r.status,
		       r.approved_quantity, r.approved_stock_bucket, r.review_note,
		       requester.full_name, COALESCE(reviewer.full_name, ''), r.reviewed_at, r.created_at
		FROM inventory_receipt_requests r
		INNER JOIN branches b ON b.id = r.branch_id
		INNER JOIN products p ON p.id = r.product_id
		INNER JOIN users requester ON requester.id = r.requested_by
		LEFT JOIN users reviewer ON reviewer.id = r.reviewed_by
		LEFT JOIN inventory i ON i.branch_id = r.branch_id AND i.product_id = r.product_id
	`
	args := []any{}
	conditions := []string{}
	if !platform.HasPermission(user, "inventory.manage.global") {
		if user.BranchID == nil {
			return nil, platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่ได้ผูกกับสาขา")
		}
		args = append(args, *user.BranchID)
		conditions = append(conditions, fmt.Sprintf("r.branch_id = $%d", len(args)))
	}
	if status != "" {
		if status != "pending" && status != "approved" && status != "rejected" {
			return nil, platform.NewError(http.StatusBadRequest, "สถานะคำขอไม่ถูกต้อง")
		}
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY CASE WHEN r.status = 'pending' THEN 0 ELSE 1 END, r.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, branchID, branchName, productID, sku, productName, note, requestStatus, reviewNote, requesterName, reviewerName string
		var requestedQuantity, snapshotReal, snapshotGhost, currentReal, currentGhost int
		var approvedQuantity sql.NullInt64
		var approvedBucket sql.NullString
		var reviewedAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&id, &branchID, &branchName, &productID, &sku, &productName,
			&requestedQuantity, &snapshotReal, &snapshotGhost, &currentReal, &currentGhost,
			&note, &requestStatus, &approvedQuantity, &approvedBucket, &reviewNote,
			&requesterName, &reviewerName, &reviewedAt, &createdAt); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "branch_id": branchID, "branch_name": branchName, "product_id": productID,
			"sku": sku, "product_name": productName, "requested_quantity": requestedQuantity,
			"snapshot_qty_real": snapshotReal, "current_qty_real": currentReal, "note": note,
			"status": requestStatus, "review_note": reviewNote, "requested_by_name": requesterName,
			"reviewed_by_name": reviewerName, "created_at": createdAt,
		}
		if reviewedAt.Valid {
			item["reviewed_at"] = reviewedAt.Time
		}
		if user.RoleKey == "super_admin" {
			item["snapshot_qty_ghost"] = snapshotGhost
			item["current_qty_ghost"] = currentGhost
			item["has_conflict"] = snapshotReal != currentReal || snapshotGhost != currentGhost
			if approvedQuantity.Valid {
				item["approved_quantity"] = approvedQuantity.Int64
			}
			if approvedBucket.Valid {
				item["approved_stock_bucket"] = approvedBucket.String
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ReviewReceiptRequest(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, requestID string, input ReviewReceiptRequest) error {
	if !platform.HasPermission(user, "inventory.manage.global") {
		return platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบเท่านั้นที่ตรวจสอบคำขอได้")
	}
	if input.Decision != "approve" && input.Decision != "reject" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกอนุมัติหรือไม่รับนำเข้า")
	}
	if input.Decision == "approve" && (input.Quantity <= 0 || (input.StockBucket != "real" && input.StockBucket != "ghost")) {
		return platform.NewError(http.StatusBadRequest, "กรุณาระบุจำนวนและประเภทสต๊อกที่จะนำเข้า")
	}
	if err := platform.EnforceGhostWritePolicy(user, input.Decision == "approve" && input.StockBucket == "ghost"); err != nil {
		return err
	}

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchID, productID, status string
		var requestedQuantity, snapshotReal, snapshotGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT branch_id::text, product_id::text, requested_quantity, snapshot_qty_real, snapshot_qty_ghost, status
			FROM inventory_receipt_requests WHERE id = $1 FOR UPDATE
		`, requestID).Scan(&branchID, &productID, &requestedQuantity, &snapshotReal, &snapshotGhost, &status); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบคำขอรับสินค้า")
			}
			return err
		}
		if status != "pending" {
			return platform.NewError(http.StatusConflict, "คำขอนี้ได้รับการตรวจสอบแล้ว")
		}
		if input.Decision == "approve" && input.StockBucket == "ghost" {
			if err := ensureGhostWarehouse(ctx, tx, branchID); err != nil {
				return err
			}
		}
		reviewNote := strings.TrimSpace(input.ReviewNote)
		if input.Decision == "reject" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory_receipt_requests
				SET status = 'rejected', review_note = $2, reviewed_by = $3, reviewed_at = NOW(), updated_at = NOW()
				WHERE id = $1
			`, requestID, reviewNote, user.ID); err != nil {
				return err
			}
		} else {
			var currentReal, currentGhost int
			if err := tx.QueryRowContext(ctx, `
				SELECT qty_real, qty_ghost FROM inventory
				WHERE branch_id = $1 AND product_id = $2 FOR UPDATE
			`, branchID, productID).Scan(&currentReal, &currentGhost); err != nil {
				return err
			}
			newReal, newGhost := currentReal, currentGhost
			if input.StockBucket == "real" {
				newReal += input.Quantity
			} else {
				newGhost += input.Quantity
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
				WHERE branch_id = $1 AND product_id = $2
			`, branchID, productID, newReal, newGhost); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (
					id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
					reference_type, reference_id, note, performed_by, created_at
				) VALUES ($1, $2, $3, 'receive_request_approved', $4, $5, 'inventory_receipt_request', $6, $7, $8, NOW())
			`, platform.MustUUID(), branchID, productID, input.StockBucket, input.Quantity, requestID, reviewNote, user.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory_receipt_requests
				SET status = 'approved', approved_quantity = $2, approved_stock_bucket = $3,
				    review_note = $4, reviewed_by = $5, reviewed_at = NOW(), updated_at = NOW()
				WHERE id = $1
			`, requestID, input.Quantity, input.StockBucket, reviewNote, user.ID); err != nil {
				return err
			}
			meta.Before = map[string]any{"qty_real": currentReal, "qty_ghost": currentGhost}
			meta.After = map[string]any{
				"qty_real": newReal, "qty_ghost": newGhost, "stock_bucket": input.StockBucket,
				"approved_quantity": input.Quantity, "requested_quantity": requestedQuantity,
				"snapshot_conflict": snapshotReal != currentReal || snapshotGhost != currentGhost,
			}
		}
		meta.EntityType = "inventory_receipt_request"
		meta.EntityID = &requestID
		meta.Action = "inventory.receipt_request." + input.Decision
		if input.Decision == "reject" {
			meta.Before = map[string]any{"status": "pending"}
			meta.After = map[string]any{"status": "rejected", "review_note": reviewNote}
		}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Rebalance(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input RebalanceRequest) error {
	if input.Quantity <= 0 || input.FromBucket == input.ToBucket ||
		(input.FromBucket != "real" && input.FromBucket != "ghost") ||
		(input.ToBucket != "real" && input.ToBucket != "ghost") {
		return platform.NewError(http.StatusBadRequest, "invalid rebalance request")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	if err := platform.EnforceGhostWritePolicy(user, true); err != nil {
		return err
	}
	branchID, err := platform.MustBranchID(user, input.BranchID)
	if err != nil {
		return err
	}
	input.BranchID = branchID
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, input.BranchID, input.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		before := map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost}
		var err error
		qtyReal, qtyGhost, err = applyRebalanceResult(qtyReal, qtyGhost, input.Quantity, input.FromBucket)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, input.BranchID, input.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}

		allocations, err := stocklot.AllocateFEFO(ctx, tx, input.BranchID, input.ProductID, input.FromBucket, input.Quantity)
		if err != nil {
			return err
		}
		outMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'rebalance', $4, $5, 'inventory.rebalance', $6, $7, NOW())
			`, outMovementID, input.BranchID, input.ProductID, input.FromBucket, -input.Quantity, input.Reason, user.ID); err != nil {
			return err
		}
		if err := stocklot.AttachMovement(ctx, tx, outMovementID, allocations, -1); err != nil {
			return err
		}
		destinationAllocations := []stocklot.Allocation{}
		for _, allocation := range allocations {
			originID := allocation.Lot.ID
			lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: input.BranchID, ProductID: input.ProductID, StockBucket: input.ToBucket, LotNumber: allocation.Lot.LotNumber, ExpiresOn: allocation.Lot.ExpiresOn, ReceivedQuantity: allocation.Quantity, RemainingQuantity: allocation.Quantity, UnitCost: allocation.Lot.UnitCost}, "inventory_rebalance", nil, nil, &originID)
			if err != nil {
				return err
			}
			destinationAllocations = append(destinationAllocations, stocklot.Allocation{Lot: stocklot.Lot{ID: lotID}, Quantity: allocation.Quantity})
		}
		inMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,note,performed_by,created_at) VALUES($1,$2,$3,'rebalance',$4,$5,'inventory.rebalance',$6,$7,NOW())`, inMovementID, input.BranchID, input.ProductID, input.ToBucket, input.Quantity, input.Reason, user.ID); err != nil {
			return err
		}
		if err := stocklot.AttachMovement(ctx, tx, inMovementID, destinationAllocations, 1); err != nil {
			return err
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.rebalance"
		meta.Before = before
		meta.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "from_bucket": input.FromBucket, "to_bucket": input.ToBucket, "quantity": input.Quantity}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Adjust(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input AdjustRequest) error {
	if strings.TrimSpace(input.ProductID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้า")
	}
	if input.QuantityDelta == 0 {
		return platform.NewError(http.StatusBadRequest, "quantity_delta must not be zero")
	}
	if input.StockBucket != "real" && input.StockBucket != "ghost" {
		return platform.NewError(http.StatusBadRequest, "invalid stock bucket")
	}
	if err := platform.EnforceGhostWritePolicy(user, input.StockBucket == "ghost"); err != nil {
		return err
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	branchID, err := platform.MustBranchID(user, input.BranchID)
	if err != nil {
		return err
	}
	input.BranchID = branchID
	if input.StockBucket == "ghost" {
		if err := ensureGhostWarehouse(ctx, s.db, input.BranchID); err != nil {
			return err
		}
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, input.BranchID, input.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้าในสต๊อกของสาขานี้")
			}
			return err
		}

		before := map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost}
		var err error
		qtyReal, qtyGhost, err = applyAdjustmentResult(qtyReal, qtyGhost, input.StockBucket, input.QuantityDelta)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, input.BranchID, input.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}

		movementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'manual_adjust', $4, $5, 'inventory.adjust', $6, $7, NOW())
		`, movementID, input.BranchID, input.ProductID, input.StockBucket, input.QuantityDelta, input.Reason, user.ID); err != nil {
			return err
		}
		if lotID := strings.TrimSpace(input.InventoryLotID); lotID != "" {
			// Counting a specific lot: move that lot's own remaining quantity,
			// never FEFO and never a fresh lot, so the ledger matches the shelf
			// the person actually counted.
			var remaining int
			if err := tx.QueryRowContext(ctx, `
				SELECT remaining_quantity FROM inventory_lots
				WHERE id = $1 AND branch_id = $2 AND product_id = $3 AND stock_bucket = $4
				FOR UPDATE
			`, lotID, input.BranchID, input.ProductID, input.StockBucket).Scan(&remaining); err != nil {
				if err == sql.ErrNoRows {
					return platform.NewError(http.StatusNotFound, "ไม่พบ lot ที่เลือกในสาขาและประเภทสต๊อกนี้")
				}
				return err
			}
			if remaining+input.QuantityDelta < 0 {
				return platform.NewError(http.StatusConflict, "จำนวนใน lot ที่เลือกไม่พอสำหรับการปรับลด")
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory_lots
				SET remaining_quantity = remaining_quantity + $2, updated_at = NOW()
				WHERE id = $1
			`, lotID, input.QuantityDelta); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movement_lots (id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at)
				VALUES ($1, $2, $3, $4, NOW())
			`, platform.MustUUID(), movementID, lotID, input.QuantityDelta); err != nil {
				return err
			}
		} else if input.QuantityDelta < 0 {
			allocations, err := stocklot.AllocateFEFO(ctx, tx, input.BranchID, input.ProductID, input.StockBucket, -input.QuantityDelta)
			if err != nil {
				return err
			}
			if err := stocklot.AttachMovement(ctx, tx, movementID, allocations, -1); err != nil {
				return err
			}
		} else {
			var tracks bool
			var cost float64
			if err := tx.QueryRowContext(ctx, `SELECT tracks_expiry,cost_price FROM products WHERE id=$1`, input.ProductID).Scan(&tracks, &cost); err != nil {
				return err
			}
			var expiry sql.NullTime
			if strings.TrimSpace(input.ExpiresOn) != "" {
				parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.ExpiresOn))
				if err != nil {
					return platform.NewError(http.StatusBadRequest, "วันหมดอายุต้องเป็นรูปแบบ YYYY-MM-DD")
				}
				expiry = sql.NullTime{Time: parsed, Valid: true}
			}
			if tracks && !expiry.Valid {
				return platform.NewError(http.StatusBadRequest, "สินค้านี้ต้องระบุวันหมดอายุ")
			}
			if input.UnitCost > 0 {
				cost = input.UnitCost
			}
			lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: input.BranchID, ProductID: input.ProductID, StockBucket: input.StockBucket, LotNumber: input.LotNumber, ExpiresOn: expiry, ReceivedQuantity: input.QuantityDelta, RemainingQuantity: input.QuantityDelta, UnitCost: cost}, "inventory_adjust", nil, nil, nil)
			if err != nil {
				return err
			}
			if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: input.QuantityDelta}}, 1); err != nil {
				return err
			}
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.adjust"
		meta.Before = before
		meta.After = map[string]any{
			"qty_real": qtyReal, "qty_ghost": qtyGhost, "stock_bucket": input.StockBucket,
			"quantity_delta": input.QuantityDelta, "reason": input.Reason,
			"inventory_lot_id": input.InventoryLotID,
		}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Receive(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ReceiveRequest) error {
	if err := platform.EnforceGhostWritePolicy(user, input.GhostQuantity != 0); err != nil {
		return err
	}
	if input.RealQuantity < 0 || input.GhostQuantity < 0 {
		return platform.NewError(http.StatusBadRequest, "quantities must not be negative")
	}
	if input.RealQuantity == 0 && input.GhostQuantity == 0 {
		return platform.NewError(http.StatusBadRequest, "at least one of real_quantity or ghost_quantity is required")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	branchID, err := platform.MustBranchID(user, input.BranchID)
	if err != nil {
		return err
	}
	input.BranchID = branchID
	if input.GhostQuantity != 0 {
		if err := ensureGhostWarehouse(ctx, s.db, input.BranchID); err != nil {
			return err
		}
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var productExists, tracksExpiry bool
		var currentCost float64
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE id = $1 AND active = TRUE), COALESCE((SELECT tracks_expiry FROM products WHERE id=$1),FALSE), COALESCE((SELECT cost_price FROM products WHERE id=$1),0)`, input.ProductID).Scan(&productExists, &tracksExpiry, &currentCost); err != nil {
			return err
		}
		if !productExists {
			return platform.NewError(http.StatusNotFound, "product not found")
		}
		var expiresOn sql.NullTime
		if strings.TrimSpace(input.ExpiresOn) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.ExpiresOn))
			if err != nil {
				return platform.NewError(http.StatusBadRequest, "วันหมดอายุต้องเป็นรูปแบบ YYYY-MM-DD")
			}
			expiresOn = sql.NullTime{Time: parsed, Valid: true}
		}
		if tracksExpiry && !expiresOn.Valid {
			return platform.NewError(http.StatusBadRequest, "สินค้านี้ต้องระบุวันหมดอายุ")
		}
		unitCost := input.UnitCost
		if unitCost == 0 {
			unitCost = currentCost
		}
		if unitCost < 0 {
			return platform.NewError(http.StatusBadRequest, "ต้นทุนต้องไม่ติดลบ")
		}

		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (branch_id, product_id) DO UPDATE
			SET qty_real = inventory.qty_real + EXCLUDED.qty_real,
			    qty_ghost = inventory.qty_ghost + EXCLUDED.qty_ghost,
			    updated_at = NOW()
			RETURNING qty_real, qty_ghost
		`, platform.MustUUID(), input.BranchID, input.ProductID, input.RealQuantity, input.GhostQuantity).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		note := strings.TrimSpace(input.Note)
		for _, movement := range []struct {
			Bucket string
			Delta  int
		}{
			{"real", input.RealQuantity},
			{"ghost", input.GhostQuantity},
		} {
			if movement.Delta == 0 {
				continue
			}
			movementID := platform.MustUUID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'receive', $4, $5, 'inventory.receive', $6, $7, NOW())
			`, movementID, input.BranchID, input.ProductID, movement.Bucket, movement.Delta, note, user.ID); err != nil {
				return err
			}
			lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: input.BranchID, ProductID: input.ProductID, StockBucket: movement.Bucket, LotNumber: input.LotNumber, ExpiresOn: expiresOn, ReceivedQuantity: movement.Delta, RemainingQuantity: movement.Delta, UnitCost: unitCost}, "inventory_receive", nil, nil, nil)
			if err != nil {
				return err
			}
			if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: movement.Delta}}, 1); err != nil {
				return err
			}
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.receive"
		meta.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "received_real": input.RealQuantity, "received_ghost": input.GhostQuantity}
		return s.audit.Log(ctx, tx, meta)
	})
}

func applyRebalanceResult(qtyReal, qtyGhost, quantity int, fromBucket string) (int, int, error) {
	if fromBucket == "real" {
		if qtyReal < quantity {
			return 0, 0, platform.NewError(http.StatusConflict, "insufficient real stock")
		}
		return qtyReal - quantity, qtyGhost + quantity, nil
	}
	if qtyGhost < quantity {
		return 0, 0, platform.NewError(http.StatusConflict, "insufficient ghost stock")
	}
	return qtyReal + quantity, qtyGhost - quantity, nil
}

func applyAdjustmentResult(qtyReal, qtyGhost int, stockBucket string, quantityDelta int) (int, int, error) {
	if stockBucket == "real" {
		if qtyReal+quantityDelta < 0 {
			return 0, 0, platform.NewError(http.StatusConflict, "real stock would become negative")
		}
		return qtyReal + quantityDelta, qtyGhost, nil
	}
	if qtyGhost+quantityDelta < 0 {
		return 0, 0, platform.NewError(http.StatusConflict, "ghost stock would become negative")
	}
	return qtyReal, qtyGhost + quantityDelta, nil
}

func ensureGhostWarehouse(ctx context.Context, db platform.DBTX, branchID string) error {
	var allowed bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM branches
			WHERE id=$1 AND branch_type='main_warehouse' AND active=TRUE
		)
	`, branchID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return platform.NewError(http.StatusBadRequest, "สต๊อกผีสร้างและปรับได้เฉพาะโกดัง WH")
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	result, err := h.service.List(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("branch_id")), ListFilter{
		Search: strings.TrimSpace(c.QueryParam("search")), Page: page, PageSize: pageSize,
	})
	if err != nil {
		// Pass typed errors through as-is, the way Receive/Adjust/Rebalance do:
		// blanket-wrapping turned the service's deliberate 403 for a
		// cross-branch branch_id into a 500 "โหลดสต๊อกไม่สำเร็จ".
		var appErr *platform.AppError
		if errors.As(err, &appErr) {
			return platform.HandleHTTPError(c, err)
		}
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load inventory", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{
		"items":      result.Items,
		"pagination": map[string]any{"page": result.Page, "page_size": result.PageSize, "total": result.Total, "total_pages": result.TotalPages},
	})
}

func (h *Handler) ListLots(c echo.Context) error {
	items, err := h.service.ListLots(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("branch_id")), strings.TrimSpace(c.QueryParam("product_id")), strings.TrimSpace(c.QueryParam("stock_bucket")), c.QueryParam("include_depleted") == "true")
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListReceiptRequests(c echo.Context) error {
	items, err := h.service.ListReceiptRequests(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("status")))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดคำขอรับสินค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateReceiptRequest(c echo.Context) error {
	var input CreateReceiptRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลคำขอไม่ถูกต้อง"))
	}
	id, err := h.service.CreateReceiptRequest(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "ส่งคำขอรับสินค้าให้ผู้ดูแลแล้ว"})
}

func (h *Handler) ReviewReceiptRequest(c echo.Context) error {
	var input ReviewReceiptRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการตรวจสอบไม่ถูกต้อง"))
	}
	if err := h.service.ReviewReceiptRequest(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("requestID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	message := "อนุมัติและนำสินค้าเข้าสต๊อกแล้ว"
	if input.Decision == "reject" {
		message = "ไม่รับนำเข้าสินค้าตามคำขอแล้ว"
	}
	return platform.JSONMessage(c, http.StatusOK, message)
}

func (h *Handler) Rebalance(c echo.Context) error {
	var input RebalanceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Rebalance(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ย้ายประเภทสต๊อกแล้ว")
}

func (h *Handler) Receive(c echo.Context) error {
	var input ReceiveRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Receive(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "รับสินค้าเข้าสต๊อกแล้ว")
}

func (h *Handler) Adjust(c echo.Context) error {
	var input AdjustRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Adjust(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ปรับยอดสต๊อกแล้ว")
}
