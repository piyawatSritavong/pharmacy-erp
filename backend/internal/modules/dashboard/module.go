package dashboard

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/go-pdf/fpdf"
	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"
)

//go:embed assets/Sarabun-Regular.ttf
var sarabunRegular []byte

type salesSummaryResult struct {
	Start        time.Time
	End          time.Time
	StartLabel   string
	EndLabel     string
	TotalSales   float64
	InvoiceCount int
	CashReceived float64
	BankReceived float64
	Invoices     []map[string]any
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Summary is permission-driven (D11) to match the route's own
// RequireAnyPermission("dashboard.view.global", "dashboard.view.self") gate
// — any role holding one of those two permissions (not just super_admin/
// branch_pos) gets the matching summary shape.
func (s *Service) Summary(ctx context.Context, user platform.AuthUser) (map[string]any, error) {
	switch {
	case platform.HasPermission(user, "dashboard.view.global"):
		return s.globalSummary(ctx, user)
	case platform.HasPermission(user, "dashboard.view.self"):
		return s.DailySales(ctx, user, "")
	default:
		return nil, platform.NewError(http.StatusForbidden, "บทบาทนี้ไม่สามารถดูแดชบอร์ดได้")
	}
}

func (s *Service) globalSummary(ctx context.Context, user platform.AuthUser) (map[string]any, error) {
	return s.refactoredGlobalSummary(ctx, user)
}

func (s *Service) DailySales(ctx context.Context, user platform.AuthUser, dateInput string) (map[string]any, error) {
	start, end, label, err := bangkokDayRange(dateInput)
	if err != nil {
		return nil, err
	}
	result, err := s.loadSalesSummary(ctx, user, start, end, label, label)
	if err != nil {
		return nil, err
	}
	return result.payload(true), nil
}

func (s *Service) SalesSummary(ctx context.Context, user platform.AuthUser, startInput, endInput string) (map[string]any, error) {
	start, end, startLabel, endLabel, err := bangkokDateRange(startInput, endInput)
	if err != nil {
		return nil, err
	}
	result, err := s.loadSalesSummary(ctx, user, start, end, startLabel, endLabel)
	if err != nil {
		return nil, err
	}
	return result.payload(false), nil
}

func (s *Service) loadSalesSummary(ctx context.Context, user platform.AuthUser, start, end time.Time, startLabel, endLabel string) (salesSummaryResult, error) {
	result := salesSummaryResult{Start: start, End: end, StartLabel: startLabel, EndLabel: endLabel}
	if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE created_by = $1 AND invoice_status = 'issued' AND deleted_at IS NULL AND issued_at >= $2 AND issued_at < $3
		`, user.ID, start, end).Scan(&result.InvoiceCount, &result.TotalSales); err != nil {
		return salesSummaryResult{}, err
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN payment_type = 'cash' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN payment_type = 'bank_transfer' THEN amount ELSE 0 END), 0)
		FROM invoice_payments ip
		INNER JOIN invoices i ON i.id=ip.invoice_id AND i.deleted_at IS NULL
		WHERE ip.created_by = $1 AND ip.created_at >= $2 AND ip.created_at < $3
		`, user.ID, start, end).Scan(&result.CashReceived, &result.BankReceived); err != nil {
		return salesSummaryResult{}, err
	}

	invoices, err := s.listSalesInvoices(ctx, user.ID, start, end)
	if err != nil {
		return salesSummaryResult{}, err
	}
	result.Invoices = invoices
	return result, nil
}

func (result salesSummaryResult) payload(daily bool) map[string]any {
	salesLabel := "ยอดขายรวม"
	invoiceLabel := "จำนวนบิล"
	cashLabel := "รับเงินสด"
	bankLabel := "รับเงินโอน"
	if daily {
		salesLabel = "ยอดขายวันนี้"
		invoiceLabel = "จำนวนบิลวันนี้"
		cashLabel = "รับเงินสดวันนี้"
		bankLabel = "รับโอนวันนี้"
	}
	return map[string]any{
		"scope":       "self_sales_range",
		"date":        result.StartLabel,
		"start_date":  result.StartLabel,
		"end_date":    result.EndLabel,
		"range_label": result.StartLabel + " ถึง " + result.EndLabel,
		"metrics": []map[string]any{
			{"key": "sales_total", "label": salesLabel, "value": result.TotalSales},
			{"key": "invoice_count", "label": invoiceLabel, "value": result.InvoiceCount},
			{"key": "cash_received", "label": cashLabel, "value": result.CashReceived},
			{"key": "bank_received", "label": bankLabel, "value": result.BankReceived},
		},
		"recent_invoices": result.Invoices,
		"recent_items": map[string]any{
			"invoices": result.Invoices,
		},
	}
}

func bangkokDayRange(dateInput string) (time.Time, time.Time, string, error) {
	// Thailand has used UTC+7 year-round since 1920, so this remains portable
	// in minimal containers that do not ship the IANA timezone database.
	location := time.FixedZone("Asia/Bangkok", 7*60*60)

	day := time.Now().In(location)
	if trimmed := strings.TrimSpace(dateInput); trimmed != "" {
		parsed, err := time.ParseInLocation("2006-01-02", trimmed, location)
		if err != nil {
			return time.Time{}, time.Time{}, "", platform.NewError(http.StatusBadRequest, "date must be YYYY-MM-DD")
		}
		day = parsed
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.Add(24 * time.Hour)
	return start.UTC(), end.UTC(), start.Format("2006-01-02"), nil
}

func bangkokDateRange(startInput, endInput string) (time.Time, time.Time, string, string, error) {
	location := time.FixedZone("Asia/Bangkok", 7*60*60)
	today := time.Now().In(location).Format("2006-01-02")
	if strings.TrimSpace(startInput) == "" {
		startInput = today
	}
	if strings.TrimSpace(endInput) == "" {
		endInput = startInput
	}
	startDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startInput), location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันที่เริ่มต้นต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	endDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endInput), location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันที่สิ้นสุดต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	if endDay.Before(startDay) {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันที่สิ้นสุดต้องไม่น้อยกว่าวันที่เริ่มต้น")
	}
	return startDay.UTC(), endDay.AddDate(0, 0, 1).UTC(), startDay.Format("2006-01-02"), endDay.Format("2006-01-02"), nil
}

func (s *Service) listSalesInvoices(ctx context.Context, userID string, start, end time.Time) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.invoice_number, i.customer_name, i.total_amount, i.payment_status, i.is_government_mode, b.name, i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.invoice_status = 'issued' AND i.deleted_at IS NULL AND i.created_by = $1 AND i.issued_at >= $2 AND i.issued_at < $3
		ORDER BY i.issued_at DESC
	`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, paymentStatus, branchName string
		var totalAmount float64
		var isGovernment bool
		var issuedAt time.Time
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &totalAmount, &paymentStatus, &isGovernment, &branchName, &issuedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "invoice_number": invoiceNumber, "customer_name": customerName,
			"total_amount": totalAmount, "payment_status": paymentStatus,
			"branch_name": branchName, "issued_at": issuedAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) ExportSalesSummary(ctx context.Context, user platform.AuthUser, startInput, endInput, format string) ([]byte, string, string, error) {
	start, end, startLabel, endLabel, err := bangkokDateRange(startInput, endInput)
	if err != nil {
		return nil, "", "", err
	}
	result, err := s.loadSalesSummary(ctx, user, start, end, startLabel, endLabel)
	if err != nil {
		return nil, "", "", err
	}
	filenameBase := fmt.Sprintf("sales-summary-%s-to-%s", startLabel, endLabel)
	switch format {
	case "pdf":
		content, err := buildSalesPDF(result, user)
		return content, filenameBase + ".pdf", "application/pdf", err
	case "xlsx":
		content, err := buildSalesXLSX(result, user)
		return content, filenameBase + ".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
	default:
		return nil, "", "", platform.NewError(http.StatusBadRequest, "รองรับเฉพาะรูปแบบ pdf หรือ xlsx")
	}
}

func buildSalesPDF(result salesSummaryResult, user platform.AuthUser) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("Sarabun", "", sarabunRegular)
	pdf.SetMargins(12, 12, 12)
	pdf.AddPage()
	pdf.SetFont("Sarabun", "", 18)
	pdf.CellFormat(0, 10, "สรุปยอดขาย", "", 1, "L", false, 0, "")
	pdf.SetFont("Sarabun", "", 11)
	pdf.CellFormat(0, 7, fmt.Sprintf("ช่วงวันที่ %s ถึง %s · ผู้ขาย %s", result.StartLabel, result.EndLabel, user.Name), "", 1, "L", false, 0, "")
	pdf.Ln(2)
	pdf.SetFillColor(255, 245, 232)
	metrics := []string{
		fmt.Sprintf("ยอดขายรวม %.2f บาท", result.TotalSales),
		fmt.Sprintf("จำนวนบิล %d", result.InvoiceCount),
		fmt.Sprintf("รับเงินสด %.2f บาท", result.CashReceived),
		fmt.Sprintf("รับเงินโอน %.2f บาท", result.BankReceived),
	}
	for _, metric := range metrics {
		pdf.CellFormat(68, 10, metric, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(14)
	headers := []string{"เลขที่ใบขาย", "ลูกค้า", "สาขา", "สถานะ", "วันที่ขาย", "ยอดรวม (บาท)"}
	widths := []float64{42, 66, 48, 30, 48, 38}
	pdf.SetFillColor(255, 200, 61)
	for index, header := range headers {
		pdf.CellFormat(widths[index], 9, header, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFillColor(255, 255, 255)
	for _, invoice := range result.Invoices {
		issuedAt, _ := invoice["issued_at"].(time.Time)
		values := []string{
			fmt.Sprint(invoice["invoice_number"]), fmt.Sprint(invoice["customer_name"]),
			fmt.Sprint(invoice["branch_name"]), thaiPaymentStatus(fmt.Sprint(invoice["payment_status"])),
			issuedAt.In(time.FixedZone("Asia/Bangkok", 7*60*60)).Format("02/01/2006 15:04"),
			fmt.Sprintf("%.2f", invoice["total_amount"].(float64)),
		}
		for index, value := range values {
			pdf.CellFormat(widths[index], 8, value, "1", 0, map[bool]string{true: "R", false: "L"}[index == len(values)-1], false, 0, "")
		}
		pdf.Ln(-1)
	}
	var buffer bytes.Buffer
	if err := pdf.Output(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildSalesXLSX(result salesSummaryResult, user platform.AuthUser) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	sheet := "สรุปยอดขาย"
	file.SetSheetName("Sheet1", sheet)
	rows := [][]any{
		{"สรุปยอดขาย", nil, nil, nil, nil, nil},
		{"ช่วงวันที่", result.StartLabel, "ถึง", result.EndLabel, "ผู้ขาย", user.Name},
		{"ยอดขายรวม", result.TotalSales, "จำนวนบิล", result.InvoiceCount, "รับเงินสด", result.CashReceived},
		{"รับเงินโอน", result.BankReceived},
		{},
		{"เลขที่ใบขาย", "ลูกค้า", "สาขา", "สถานะ", "วันที่ขาย", "ยอดรวม (บาท)"},
	}
	for _, invoice := range result.Invoices {
		issuedAt, _ := invoice["issued_at"].(time.Time)
		rows = append(rows, []any{
			invoice["invoice_number"], invoice["customer_name"], invoice["branch_name"],
			thaiPaymentStatus(fmt.Sprint(invoice["payment_status"])), issuedAt, invoice["total_amount"],
		})
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}
	headerStyle, _ := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#FFC83D"}, Pattern: 1}})
	titleStyle, _ := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 16, Color: "#D21121"}})
	moneyStyle, _ := file.NewStyle(&excelize.Style{NumFmt: 4})
	dateStyle, _ := file.NewStyle(&excelize.Style{CustomNumFmt: stringPointer("dd/mm/yyyy hh:mm")})
	file.SetCellStyle(sheet, "A1", "F1", titleStyle)
	file.SetCellStyle(sheet, "A6", "F6", headerStyle)
	if len(rows) > 6 {
		file.SetCellStyle(sheet, "F7", fmt.Sprintf("F%d", len(rows)), moneyStyle)
		file.SetCellStyle(sheet, "E7", fmt.Sprintf("E%d", len(rows)), dateStyle)
	}
	file.SetColWidth(sheet, "A", "A", 24)
	file.SetColWidth(sheet, "B", "B", 34)
	file.SetColWidth(sheet, "C", "C", 24)
	file.SetColWidth(sheet, "D", "D", 18)
	file.SetColWidth(sheet, "E", "E", 22)
	file.SetColWidth(sheet, "F", "F", 18)
	file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 6, TopLeftCell: "A7", ActivePane: "bottomLeft"})
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func stringPointer(value string) *string { return &value }

func thaiPaymentStatus(status string) string {
	switch status {
	case "paid":
		return "ชำระแล้ว"
	case "unpaid":
		return "ค้างชำระ"
	default:
		return status
	}
}

func (s *Service) listRecentInvoices(ctx context.Context, scope string, branchID string, userID string, ranges ...time.Time) ([]map[string]any, error) {
	query := `
		SELECT i.id, i.invoice_number, i.customer_name, i.total_amount, i.payment_status, i.is_government_mode, b.name, i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.invoice_status = 'issued' AND i.deleted_at IS NULL
	`
	args := []any{}

	if scope == "branch" {
		args = append(args, branchID)
		query += " AND i.branch_id = $1"
	}
	if scope == "self_daily" {
		args = append(args, userID)
		query += " AND i.created_by = $" + strconvI(len(args))
		if len(ranges) == 2 {
			args = append(args, ranges[0], ranges[1])
			query += " AND i.issued_at >= $" + strconvI(len(args)-1) + " AND i.issued_at < $" + strconvI(len(args))
		}
	}

	query += " ORDER BY i.issued_at DESC LIMIT 8"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, paymentStatus, branchName string
		var totalAmount float64
		var isGovernment bool
		var issuedAt time.Time
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &totalAmount, &paymentStatus, &isGovernment, &branchName, &issuedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                 id,
			"invoice_number":     invoiceNumber,
			"customer_name":      customerName,
			"total_amount":       totalAmount,
			"payment_status":     paymentStatus,
			"is_government_mode": isGovernment,
			"branch_name":        branchName,
			"issued_at":          issuedAt,
		})
	}
	return items, rows.Err()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Summary(c echo.Context) error {
	data, err := h.service.Summary(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, data)
}

func (h *Handler) DailySales(c echo.Context) error {
	startDate := strings.TrimSpace(c.QueryParam("start_date"))
	endDate := strings.TrimSpace(c.QueryParam("end_date"))
	var data map[string]any
	var err error
	if startDate != "" || endDate != "" {
		data, err = h.service.SalesSummary(c.Request().Context(), platform.CurrentUser(c), startDate, endDate)
	} else {
		data, err = h.service.DailySales(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("date")))
	}
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, data)
}

func (h *Handler) ExportSales(c echo.Context) error {
	content, filename, contentType, err := h.service.ExportSalesSummary(
		c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("start_date")),
		strings.TrimSpace(c.QueryParam("end_date")), strings.TrimSpace(c.QueryParam("format")),
	)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	return c.Blob(http.StatusOK, contentType, content)
}

func strconvI(value int) string {
	return strconv.Itoa(value)
}
