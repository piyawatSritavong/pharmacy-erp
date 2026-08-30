package audit

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type LogEntry struct {
	ActorID    *string
	BranchID   *string
	EntityID   *string
	EntityType string
	Action     string
	Before     any
	After      any
	RequestID  string
	SourceIP   string
	UserAgent  string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Log(ctx context.Context, db platform.DBTX, entry LogEntry) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_id, branch_id, entity_type, entity_id, action, before_data, after_data, request_id, source_ip, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, $11, NOW())
	`,
		platform.MustUUID(),
		platform.NullUUID(entry.ActorID),
		platform.NullUUID(entry.BranchID),
		entry.EntityType,
		platform.NullUUID(entry.EntityID),
		entry.Action,
		platform.MustJSON(entry.Before),
		platform.MustJSON(entry.After),
		entry.RequestID,
		entry.SourceIP,
		entry.UserAgent,
	)
	return err
}

type AuditRow struct {
	ID         string         `json:"id"`
	EntityType string         `json:"entity_type"`
	EntityID   sql.NullString `json:"-"`
	Action     string         `json:"action"`
	SourceIP   string         `json:"source_ip"`
	UserAgent  string         `json:"user_agent"`
	CreatedAt  time.Time      `json:"created_at"`
	ActorName  sql.NullString `json:"-"`
	BranchName sql.NullString `json:"-"`
	BeforeData string         `json:"before_data"`
	AfterData  string         `json:"after_data"`
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string, entityType string, action string, dateFrom string, dateTo string) ([]map[string]any, error) {
	query := `
		SELECT a.id, a.entity_type, a.entity_id::text, a.action, a.source_ip, a.user_agent, a.created_at,
			   COALESCE(u.full_name, ''), COALESCE(b.name, ''), a.before_data::text, a.after_data::text
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		LEFT JOIN branches b ON b.id = a.branch_id
		WHERE 1 = 1
	`
	args := []any{}
	if user.RoleKey != "super_admin" {
		// Ghost-stock and hidden-invoice operations are deliberately absent
		// from non-superadmin history, including when a caller guesses a filter.
		query += ` AND a.entity_type NOT IN ('month_end_workpaper','month_end_workpaper_line','month_end_reconciliation')
			AND a.action NOT LIKE 'month_end.%'
			AND NOT (a.entity_type='invoice' AND a.action='invoice.delete')
			AND NOT (
				a.before_data ? 'qty_ghost' OR a.after_data ? 'qty_ghost'
				OR a.before_data->>'stock_bucket'='ghost' OR a.after_data->>'stock_bucket'='ghost'
				OR a.before_data->>'from_bucket'='ghost' OR a.after_data->>'from_bucket'='ghost'
				OR a.before_data->>'to_bucket'='ghost' OR a.after_data->>'to_bucket'='ghost'
			)`
	}
	if branchID != "" {
		args = append(args, branchID)
		query += " AND a.branch_id = $" + strconvI(len(args))
	} else if user.BranchID != nil && !platform.HasPermission(user, "audit.view.global") {
		args = append(args, *user.BranchID)
		query += " AND a.branch_id = $" + strconvI(len(args))
	}
	if entityType != "" {
		args = append(args, entityType)
		query += " AND a.entity_type = $" + strconvI(len(args))
	}
	if action != "" {
		args = append(args, action)
		query += " AND a.action = $" + strconvI(len(args))
	}
	if dateFrom != "" {
		args = append(args, dateFrom+" 00:00:00+07")
		query += " AND a.created_at >= $" + strconvI(len(args)) + "::timestamptz"
	}
	if dateTo != "" {
		args = append(args, dateTo+" 23:59:59.999999+07")
		query += " AND a.created_at <= $" + strconvI(len(args)) + "::timestamptz"
	}
	query += " ORDER BY a.created_at DESC LIMIT 50"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]any{}
	for rows.Next() {
		var row AuditRow
		var actorName string
		var branchName string
		if err := rows.Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action, &row.SourceIP, &row.UserAgent, &row.CreatedAt, &actorName, &branchName, &row.BeforeData, &row.AfterData); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"id":          row.ID,
			"entity_type": row.EntityType,
			"entity_id":   row.EntityID.String,
			"action":      row.Action,
			"source_ip":   row.SourceIP,
			"user_agent":  row.UserAgent,
			"created_at":  row.CreatedAt,
			"actor_name":  actorName,
			"branch_name": branchName,
			"before_data": row.BeforeData,
			"after_data":  row.AfterData,
		})
	}
	return results, rows.Err()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	user := platform.CurrentUser(c)
	items, err := h.service.List(
		c.Request().Context(),
		user,
		strings.TrimSpace(c.QueryParam("branch_id")),
		strings.TrimSpace(c.QueryParam("entity_type")),
		strings.TrimSpace(c.QueryParam("action")),
		strings.TrimSpace(c.QueryParam("date_from")),
		strings.TrimSpace(c.QueryParam("date_to")),
	)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load audit logs", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func MetaFromContext(c echo.Context) LogEntry {
	user := platform.CurrentUser(c)
	return LogEntry{
		ActorID:   &user.ID,
		BranchID:  user.BranchID,
		RequestID: requestIDFromContext(c),
		SourceIP:  c.RealIP(),
		UserAgent: c.Request().UserAgent(),
	}
}

func requestIDFromContext(c echo.Context) string {
	if value, ok := c.Get(platform.ContextRequestIDKey).(string); ok {
		return value
	}
	return ""
}

func strconvI(value int) string {
	return strconv.Itoa(value)
}
