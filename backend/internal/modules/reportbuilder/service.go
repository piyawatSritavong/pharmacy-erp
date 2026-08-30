package reportbuilder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) Catalog() CatalogResponse {
	return Catalog()
}

var ghostDatasets = map[string]bool{
	"branch_product_overview": true,
	"purchase_order_items":    true,
	"inventory_lots":          true,
	"inventory_movements":     true,
	"month_end":               true,
	"month_end_renumbered":    true,
}

func (s *Service) CatalogForUser(user platform.AuthUser) CatalogResponse {
	catalog := Catalog()
	if user.RoleKey == "super_admin" {
		return catalog
	}
	datasets := make([]Dataset, 0, len(catalog.Datasets))
	for _, dataset := range catalog.Datasets {
		if ghostDatasets[dataset.Key] {
			continue
		}
		fields := make([]Field, 0, len(dataset.Fields))
		for _, field := range dataset.Fields {
			if field.Key != "stock_bucket" {
				fields = append(fields, field)
			}
		}
		dataset.Fields = fields
		defaults := make([]string, 0, len(dataset.DefaultColumns))
		for _, field := range dataset.DefaultColumns {
			if field != "stock_bucket" {
				defaults = append(defaults, field)
			}
		}
		dataset.DefaultColumns = defaults
		datasets = append(datasets, dataset)
	}
	catalog.Datasets = datasets
	return catalog
}

func validateReportVisibility(user platform.AuthUser, definition Definition) error {
	if user.RoleKey == "super_admin" {
		return nil
	}
	if ghostDatasets[definition.DatasetKey] {
		return platform.NewError(http.StatusForbidden, "ชุดข้อมูลนี้มีสต๊อกผีและเปิดได้เฉพาะผู้ดูแลระบบสูงสุด")
	}
	checkField := func(field string) error {
		if normalizeKey(field) == "stock_bucket" {
			return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ใช้ฟิลด์ประเภทสต๊อก")
		}
		return nil
	}
	for _, column := range definition.Columns {
		if err := checkField(column.Field); err != nil {
			return err
		}
	}
	for _, sort := range definition.Sorts {
		if err := checkField(sort.Field); err != nil {
			return err
		}
	}
	if definition.TimeConfig != nil {
		if err := checkField(definition.TimeConfig.Field); err != nil {
			return err
		}
	}
	var checkGroup func(FilterGroup) error
	checkGroup = func(group FilterGroup) error {
		for _, rule := range group.Rules {
			if err := checkField(rule.Field); err != nil {
				return err
			}
		}
		for _, child := range group.Groups {
			if err := checkGroup(child); err != nil {
				return err
			}
		}
		return nil
	}
	return checkGroup(definition.Filters)
}

func (s *Service) Execute(ctx context.Context, user platform.AuthUser, ownerID string, request ExecuteRequest) (QueryResult, error) {
	var definition Definition
	if strings.TrimSpace(request.ReportID) != "" {
		report, err := s.Get(ctx, ownerID, request.ReportID)
		if err != nil {
			return QueryResult{}, err
		}
		definition = report.Definition
	} else if request.Definition != nil {
		definition = *request.Definition
	} else {
		return QueryResult{}, platform.NewError(http.StatusBadRequest, "report definition is required")
	}
	if err := validateReportVisibility(user, definition); err != nil {
		return QueryResult{}, err
	}
	compiled, err := compileDefinition(definition, request.Page, request.Preview, time.Now().In(bangkok))
	if err != nil {
		return QueryResult{}, err
	}
	return executeCompiled(ctx, s.db, compiled, request.Page, request.Preview)
}

// List returns the owner's saved reports. When pinTargetKey is non-empty,
// it's narrowed to reports currently pinned to that page (A6) — used by
// <ReportPinSlot> on each page instead of fetching every saved report.
func (s *Service) List(ctx context.Context, ownerID string, pinTargetKey string) ([]SavedReport, error) {
	query := `
		SELECT id::text, owner_user_id::text, name, description, definition_version,
		       definition, is_pinned, pin_order, pin_target_key, created_at, updated_at
		FROM report_definitions
		WHERE owner_user_id = $1
	`
	args := []any{ownerID}
	if strings.TrimSpace(pinTargetKey) != "" {
		query += ` AND is_pinned = TRUE AND pin_target_key = $2`
		args = append(args, pinTargetKey)
	}
	query += ` ORDER BY is_pinned DESC, pin_order NULLS LAST, updated_at DESC, name`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SavedReport{}
	for rows.Next() {
		item, err := scanSavedReport(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, ownerID, reportID string) (SavedReport, error) {
	if _, err := uuid.Parse(reportID); err != nil {
		return SavedReport{}, platform.NewError(http.StatusBadRequest, "report id is invalid")
	}
	return getSavedReport(ctx, s.db, ownerID, reportID)
}

func (s *Service) Create(ctx context.Context, ownerID string, input SaveReportRequest, meta audit.LogEntry) (SavedReport, error) {
	input, err := validateSaveRequest(input)
	if err != nil {
		return SavedReport{}, err
	}
	id := platform.MustUUID()
	definitionJSON, _ := json.Marshal(input.Definition)
	var result SavedReport
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO report_definitions (
				id, owner_user_id, name, description, definition_version, definition,
				is_pinned, pin_order, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6::jsonb, FALSE, NULL, NOW(), NOW())
		`, id, ownerID, input.Name, input.Description, DefinitionVersion, definitionJSON)
		if err != nil {
			return mapPersistenceError(err)
		}
		result, err = getSavedReport(ctx, tx, ownerID, id)
		if err != nil {
			return err
		}
		entry := meta
		entry.EntityType = "report_definition"
		entry.EntityID = &id
		entry.Action = "report.created"
		entry.After = result
		return s.audit.Log(ctx, tx, entry)
	})
	return result, err
}

func (s *Service) Update(ctx context.Context, ownerID, reportID string, input SaveReportRequest, meta audit.LogEntry) (SavedReport, error) {
	if _, err := uuid.Parse(reportID); err != nil {
		return SavedReport{}, platform.NewError(http.StatusBadRequest, "report id is invalid")
	}
	input, err := validateSaveRequest(input)
	if err != nil {
		return SavedReport{}, err
	}
	definitionJSON, _ := json.Marshal(input.Definition)
	var result SavedReport
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		before, err := getSavedReportForUpdate(ctx, tx, ownerID, reportID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE report_definitions
			SET name = $3, description = $4, definition_version = $5,
			    definition = $6::jsonb, updated_at = NOW()
			WHERE id = $1 AND owner_user_id = $2
		`, reportID, ownerID, input.Name, input.Description, DefinitionVersion, definitionJSON); err != nil {
			return mapPersistenceError(err)
		}
		result, err = getSavedReport(ctx, tx, ownerID, reportID)
		if err != nil {
			return err
		}
		entry := meta
		entry.EntityType = "report_definition"
		entry.EntityID = &reportID
		entry.Action = "report.updated"
		entry.Before = before
		entry.After = result
		return s.audit.Log(ctx, tx, entry)
	})
	return result, err
}

func (s *Service) Delete(ctx context.Context, ownerID, reportID string, meta audit.LogEntry) error {
	if _, err := uuid.Parse(reportID); err != nil {
		return platform.NewError(http.StatusBadRequest, "report id is invalid")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		before, err := getSavedReportForUpdate(ctx, tx, ownerID, reportID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM report_definitions WHERE id = $1 AND owner_user_id = $2`, reportID, ownerID); err != nil {
			return err
		}
		if before.IsPinned {
			if err := compactPinOrder(ctx, tx, ownerID, before.PinTargetKey); err != nil {
				return err
			}
		}
		entry := meta
		entry.EntityType = "report_definition"
		entry.EntityID = &reportID
		entry.Action = "report.deleted"
		entry.Before = before
		return s.audit.Log(ctx, tx, entry)
	})
}

func (s *Service) SetPinned(ctx context.Context, ownerID, reportID string, pinned bool, pinTargetKey string, meta audit.LogEntry) (SavedReport, error) {
	if _, err := uuid.Parse(reportID); err != nil {
		return SavedReport{}, platform.NewError(http.StatusBadRequest, "report id is invalid")
	}
	pinTargetKey = strings.TrimSpace(pinTargetKey)
	if pinned && pinTargetKey == "" {
		pinTargetKey = "generate_report"
	}
	var result SavedReport
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockOwnerReports(ctx, tx, ownerID); err != nil {
			return err
		}
		before, err := getSavedReportForUpdate(ctx, tx, ownerID, reportID)
		if err != nil {
			return err
		}
		if before.IsPinned == pinned && (!pinned || before.PinTargetKey == pinTargetKey) {
			result = before
			return nil
		}
		if pinned {
			// Re-pinning to a different target vacates its slot in the old one.
			if before.IsPinned && before.PinTargetKey != pinTargetKey {
				if err := compactPinOrder(ctx, tx, ownerID, before.PinTargetKey); err != nil {
					return err
				}
			}
			var next int
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(pin_order), -1) + 1 FROM report_definitions WHERE owner_user_id = $1 AND is_pinned = TRUE AND pin_target_key = $2`, ownerID, pinTargetKey).Scan(&next); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE report_definitions SET is_pinned = TRUE, pin_target_key = $3, pin_order = $4, updated_at = NOW() WHERE id = $1 AND owner_user_id = $2`, reportID, ownerID, pinTargetKey, next); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE report_definitions SET is_pinned = FALSE, pin_order = NULL, updated_at = NOW() WHERE id = $1 AND owner_user_id = $2`, reportID, ownerID); err != nil {
				return err
			}
			if err := compactPinOrder(ctx, tx, ownerID, before.PinTargetKey); err != nil {
				return err
			}
		}
		result, err = getSavedReport(ctx, tx, ownerID, reportID)
		if err != nil {
			return err
		}
		entry := meta
		entry.EntityType = "report_definition"
		entry.EntityID = &reportID
		entry.Action = "report.pinned"
		if !pinned {
			entry.Action = "report.unpinned"
		}
		entry.Before = before
		entry.After = result
		return s.audit.Log(ctx, tx, entry)
	})
	return result, err
}

func (s *Service) ReorderPins(ctx context.Context, ownerID, pinTargetKey string, reportIDs []string, meta audit.LogEntry) error {
	pinTargetKey = strings.TrimSpace(pinTargetKey)
	if pinTargetKey == "" {
		pinTargetKey = "generate_report"
	}
	seen := map[string]bool{}
	for _, reportID := range reportIDs {
		if _, err := uuid.Parse(reportID); err != nil || seen[reportID] {
			return platform.NewError(http.StatusBadRequest, "report pin order is invalid")
		}
		seen[reportID] = true
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockOwnerReports(ctx, tx, ownerID); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT id::text FROM report_definitions WHERE owner_user_id = $1 AND is_pinned = TRUE AND pin_target_key = $2 ORDER BY pin_order`, ownerID, pinTargetKey)
		if err != nil {
			return err
		}
		current := []string{}
		currentSet := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			current = append(current, id)
			currentSet[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(current) != len(reportIDs) {
			return platform.NewError(http.StatusBadRequest, "report pin order must include every pinned report")
		}
		for _, reportID := range reportIDs {
			if !currentSet[reportID] {
				return platform.NewError(http.StatusBadRequest, "report pin order contains a report you do not own")
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE report_definitions SET pin_order = pin_order + 100000 WHERE owner_user_id = $1 AND is_pinned = TRUE AND pin_target_key = $2`, ownerID, pinTargetKey); err != nil {
			return err
		}
		for index, reportID := range reportIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE report_definitions SET pin_order = $3, updated_at = NOW() WHERE id = $1 AND owner_user_id = $2 AND is_pinned = TRUE AND pin_target_key = $4`, reportID, ownerID, index, pinTargetKey); err != nil {
				return err
			}
		}
		entry := meta
		entry.EntityType = "report_definition"
		entry.Action = "report.pins_reordered"
		entry.Before = map[string]any{"report_ids": current, "pin_target_key": pinTargetKey}
		entry.After = map[string]any{"report_ids": reportIDs, "pin_target_key": pinTargetKey}
		return s.audit.Log(ctx, tx, entry)
	})
}

func validateSaveRequest(input SaveReportRequest) (SaveReportRequest, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		return SaveReportRequest{}, platform.NewError(http.StatusBadRequest, "report name is required")
	}
	if len([]rune(input.Name)) > 120 || len([]rune(input.Description)) > 500 {
		return SaveReportRequest{}, platform.NewError(http.StatusBadRequest, "report name or description is too long")
	}
	definition, err := ValidateDefinition(input.Definition)
	if err != nil {
		return SaveReportRequest{}, err
	}
	input.Definition = definition
	return input, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSavedReport(row scanner) (SavedReport, error) {
	var item SavedReport
	var definitionJSON []byte
	var pinOrder sql.NullInt64
	if err := row.Scan(&item.ID, &item.OwnerUserID, &item.Name, &item.Description, &item.DefinitionVersion, &definitionJSON, &item.IsPinned, &pinOrder, &item.PinTargetKey, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return SavedReport{}, err
	}
	if err := json.Unmarshal(definitionJSON, &item.Definition); err != nil {
		return SavedReport{}, err
	}
	normalizeFilterGroupArrays(&item.Definition.Filters)
	if pinOrder.Valid {
		value := int(pinOrder.Int64)
		item.PinOrder = &value
	}
	return item, nil
}

func normalizeFilterGroupArrays(group *FilterGroup) {
	if group.Rules == nil {
		group.Rules = []FilterRule{}
	}
	if group.Groups == nil {
		group.Groups = []FilterGroup{}
	}
	for index := range group.Groups {
		normalizeFilterGroupArrays(&group.Groups[index])
	}
}

func getSavedReport(ctx context.Context, db platform.DBTX, ownerID, reportID string) (SavedReport, error) {
	item, err := scanSavedReport(db.QueryRowContext(ctx, `
		SELECT id::text, owner_user_id::text, name, description, definition_version,
		       definition, is_pinned, pin_order, pin_target_key, created_at, updated_at
		FROM report_definitions
		WHERE id = $1 AND owner_user_id = $2
	`, reportID, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedReport{}, platform.NewError(http.StatusNotFound, "report not found")
	}
	return item, err
}

func getSavedReportForUpdate(ctx context.Context, tx *sql.Tx, ownerID, reportID string) (SavedReport, error) {
	item, err := scanSavedReport(tx.QueryRowContext(ctx, `
		SELECT id::text, owner_user_id::text, name, description, definition_version,
		       definition, is_pinned, pin_order, pin_target_key, created_at, updated_at
		FROM report_definitions
		WHERE id = $1 AND owner_user_id = $2
		FOR UPDATE
	`, reportID, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedReport{}, platform.NewError(http.StatusNotFound, "report not found")
	}
	return item, err
}

func lockOwnerReports(ctx context.Context, tx *sql.Tx, ownerID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM report_definitions WHERE owner_user_id = $1 FOR UPDATE`, ownerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func compactPinOrder(ctx context.Context, tx *sql.Tx, ownerID, pinTargetKey string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE report_definitions
		SET pin_order = pin_order + 100000
		WHERE owner_user_id = $1 AND is_pinned = TRUE AND pin_target_key = $2
	`, ownerID, pinTargetKey); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		WITH ordered AS (
			SELECT id, (ROW_NUMBER() OVER (ORDER BY pin_order, updated_at, id) - 1)::integer AS next_order
			FROM report_definitions
			WHERE owner_user_id = $1 AND is_pinned = TRUE AND pin_target_key = $2
		)
		UPDATE report_definitions target
		SET pin_order = ordered.next_order
		FROM ordered
		WHERE target.id = ordered.id
	`, ownerID, pinTargetKey)
	return err
}

func mapPersistenceError(err error) error {
	var pqError *pq.Error
	if errors.As(err, &pqError) && pqError.Code == "23505" {
		return platform.NewError(http.StatusConflict, "report name already exists")
	}
	return err
}

// FieldValues returns the distinct values a filterable field actually holds, so
// the filter builder can offer a picker instead of a blank text box.
//
// Injection safety: the request carries only KEYS. Both the FROM clause
// (dataset.Source) and the column expression (field.Expression) are looked up
// from the server-side catalog and never come from the caller — the same rule
// the query compiler follows.
//
// Only low-cardinality-ish types get a list: a dropdown of numbers or
// timestamps is noise, and those keep the free-text input on the client.
func (s *Service) FieldValues(ctx context.Context, user platform.AuthUser, datasetKey, fieldKey string, limit int) ([]string, error) {
	if err := validateReportVisibility(user, Definition{DatasetKey: datasetKey, Columns: []ColumnDefinition{{Field: fieldKey}}}); err != nil {
		return nil, err
	}
	dataset, ok := datasetByKey(datasetKey)
	if !ok {
		return nil, platform.NewError(http.StatusBadRequest, "ไม่พบชุดข้อมูลที่เลือก")
	}
	var field Field
	found := false
	for _, candidate := range dataset.Fields {
		if candidate.Key == fieldKey {
			field, found = candidate, true
			break
		}
	}
	if !found || !field.Filterable {
		return nil, platform.NewError(http.StatusBadRequest, "ฟิลด์นี้ใช้กรองไม่ได้")
	}
	if field.Type != "text" && field.Type != "boolean" {
		return []string{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT value FROM (
			SELECT DISTINCT (`+field.Expression+`)::text AS value
			FROM `+dataset.Source+`
		) distinct_values
		WHERE value IS NOT NULL AND value <> ''
		ORDER BY value
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
