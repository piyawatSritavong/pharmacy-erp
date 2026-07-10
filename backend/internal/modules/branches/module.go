package branches

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type Branch struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Active  bool   `json:"active"`
}

type BranchInput struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Active  *bool  `json:"active"`
}

type Sequence struct {
	ID         string `json:"id"`
	BranchID   string `json:"branch_id"`
	BranchCode string `json:"branch_code"`
	BranchName string `json:"branch_name"`
	DocType    string `json:"doc_type"`
	Prefix     string `json:"prefix"`
	NextNumber int64  `json:"next_number"`
	IsLocked   bool   `json:"is_locked"`
	Example    string `json:"example_number"`
}

type UpdateSequenceRequest struct {
	Prefix     string `json:"prefix"`
	NextNumber int64  `json:"next_number"`
	IsLocked   bool   `json:"is_locked"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context) ([]Branch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, code, name, address, active FROM branches ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Branch{}
	for rows.Next() {
		var item Branch
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Address, &item.Active); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input BranchInput) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return "", platform.NewError(http.StatusBadRequest, "branch code and name are required")
	}

	branchID := platform.MustUUID()
	active := true
	if input.Active != nil {
		active = *input.Active
	}

	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO branches (id, code, name, address, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, branchID, code, name, strings.TrimSpace(input.Address), active); err != nil {
			return err
		}

		for _, docType := range []string{"invoice", "quotation"} {
			prefix := "BL"
			if docType == "quotation" {
				prefix = "QT"
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO document_sequences (id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at)
				VALUES ($1, $2, $3, $4, 1, FALSE, NOW(), NOW())
			`, platform.MustUUID(), branchID, docType, prefix); err != nil {
				return err
			}
		}

		meta.EntityType = "branch"
		meta.EntityID = &branchID
		meta.Action = "branch.create"
		meta.After = map[string]any{"code": code, "name": name, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
	return branchID, err
}

func (s *Service) Update(ctx context.Context, branchID string, user platform.AuthUser, meta audit.LogEntry, input BranchInput) error {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return platform.NewError(http.StatusBadRequest, "branch code and name are required")
	}

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(b)::text
			FROM (
				SELECT code, name, address, active
				FROM branches
				WHERE id = $1
			) b
		`, branchID).Scan(&beforeJSON); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "branch not found")
			}
			return err
		}

		var currentActive bool
		if err := tx.QueryRowContext(ctx, `SELECT active FROM branches WHERE id = $1`, branchID).Scan(&currentActive); err != nil {
			return err
		}
		active := currentActive
		if input.Active != nil {
			active = *input.Active
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE branches
			SET code = $2, name = $3, address = $4, active = $5, updated_at = NOW()
			WHERE id = $1
		`, branchID, code, name, strings.TrimSpace(input.Address), active); err != nil {
			return err
		}

		meta.EntityType = "branch"
		meta.EntityID = &branchID
		meta.Action = "branch.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"code": code, "name": name, "address": strings.TrimSpace(input.Address), "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) ListSequences(ctx context.Context) ([]Sequence, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ds.id, ds.branch_id::text, b.code, b.name, ds.doc_type, ds.prefix, ds.next_number, ds.is_locked
		FROM document_sequences ds
		INNER JOIN branches b ON b.id = ds.branch_id
		ORDER BY b.name, ds.doc_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Sequence{}
	for rows.Next() {
		var item Sequence
		if err := rows.Scan(&item.ID, &item.BranchID, &item.BranchCode, &item.BranchName, &item.DocType, &item.Prefix, &item.NextNumber, &item.IsLocked); err != nil {
			return nil, err
		}
		item.Example = platform.FormatSalesDocNumber(item.Prefix, time.Now().UTC(), item.NextNumber)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) UpdateSequence(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, branchID string, docType string, input UpdateSequenceRequest) error {
	if input.NextNumber <= 0 {
		return platform.NewError(http.StatusBadRequest, "next_number must be greater than zero")
	}

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		var currentPrefix string
		var currentNextNumber int64
		var currentLocked bool
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(ds)::text, ds.prefix, ds.next_number, ds.is_locked
			FROM (
				SELECT prefix, next_number, is_locked
				FROM document_sequences
				WHERE branch_id = $1 AND doc_type = $2
			) ds
		`, branchID, docType).Scan(&beforeJSON, &currentPrefix, &currentNextNumber, &currentLocked); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "sequence not found")
			}
			return err
		}

		prefix, err := validateSequenceUpdate(currentPrefix, currentNextNumber, currentLocked, input)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE document_sequences
			SET prefix = $3, next_number = $4, is_locked = $5, updated_at = NOW()
			WHERE branch_id = $1 AND doc_type = $2
		`, branchID, docType, prefix, input.NextNumber, input.IsLocked); err != nil {
			return err
		}

		entityID := branchID + ":" + docType
		meta.EntityType = "document_sequence"
		meta.EntityID = &entityID
		meta.Action = "sequence.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{
			"prefix":         prefix,
			"next_number":    input.NextNumber,
			"is_locked":      input.IsLocked,
			"example_number": platform.FormatSalesDocNumber(prefix, time.Now().UTC(), input.NextNumber),
		}
		return s.audit.Log(ctx, tx, meta)
	})
}

func validateSequenceUpdate(currentPrefix string, currentNextNumber int64, currentLocked bool, input UpdateSequenceRequest) (string, error) {
	prefix := strings.ToUpper(strings.TrimSpace(input.Prefix))
	if prefix == "" {
		return "", platform.NewError(http.StatusBadRequest, "prefix is required")
	}
	if !currentLocked {
		return prefix, nil
	}

	changedFields := prefix != currentPrefix || input.NextNumber != currentNextNumber
	if changedFields {
		if !input.IsLocked {
			return "", platform.NewError(http.StatusConflict, "sequence is locked; unlock it before editing prefix or next number")
		}
		return "", platform.NewError(http.StatusConflict, "sequence is locked; prefix and next number cannot be edited")
	}
	return prefix, nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load branches", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	var input BranchInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "branch created"})
}

func (h *Handler) Update(c echo.Context) error {
	var input BranchInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Update(c.Request().Context(), c.Param("branchID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "branch updated")
}

func (h *Handler) ListSequences(c echo.Context) error {
	items, err := h.service.ListSequences(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load sequences", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) UpdateSequence(c echo.Context) error {
	var input UpdateSequenceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	err := h.service.UpdateSequence(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("branchID"), c.Param("docType"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "sequence updated")
}
