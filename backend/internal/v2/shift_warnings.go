package v2

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	"pharmacy-erp/backend/internal/platform"
)

type ShiftWarning struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Reference   string `json:"reference"`
	Reason      string `json:"reason"`
	CrossDay    bool   `json:"cross_day"`
}

func shiftWarnings(ctx context.Context, db platform.DBTX, b, account string) ([]ShiftWarning, string, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.kind,x.id),'[]'::jsonb) FROM (
 SELECT c.id,'refund' AS kind,c.refund_due_cents-COALESCE((SELECT SUM(r.amount_cents) FROM v2_refund_settlements r WHERE r.cancellation_id=c.id),0)::bigint AS amount_cents,d.document_number AS reference,c.reason,
 LEAST(d.issued_on,(c.created_at AT TIME ZONE 'Asia/Bangkok')::date)<(NOW() AT TIME ZONE 'Asia/Bangkok')::date AS cross_day
 FROM v2_cancellations c JOIN v2_documents d ON d.id=c.document_id WHERE c.branch_id=$1 AND c.refund_due_cents>COALESCE((SELECT SUM(r.amount_cents) FROM v2_refund_settlements r WHERE r.cancellation_id=c.id),0)
 UNION ALL SELECT i.id,'variance',i.amount_cents,i.drawer_id::text,i.reason,(i.created_at AT TIME ZONE 'Asia/Bangkok')::date<(NOW() AT TIME ZONE 'Asia/Bangkok')::date
 FROM v2_shift_issues i WHERE i.branch_id=$1 AND i.account_id=$2 AND NOT EXISTS(SELECT 1 FROM v2_shift_issue_resolutions r WHERE r.issue_id=i.id)
 )x`, b, account).Scan(&raw)
	if err != nil {
		return nil, "", err
	}
	warnings := []ShiftWarning{}
	if err = json.Unmarshal(raw, &warnings); err != nil {
		return nil, "", err
	}
	return warnings, fmt.Sprintf("%x", sha256.Sum256([]byte(rawJSON(warnings)))), nil
}

func (s *Service) drawers(c echo.Context) error {
	ctx := c.Request().Context()
	user := platform.CurrentUser(c)
	b, err := branch(ctx, s.db, user, c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	warnings, token, err := shiftWarnings(ctx, s.db, b, user.ID)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var items json.RawMessage
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.opened_at DESC),'[]'::jsonb) FROM (
 SELECT d.*,d.opening_cents+COALESCE((SELECT SUM(e.amount_cents) FROM v2_drawer_events e WHERE e.drawer_id=d.id),0)::bigint AS current_expected_cents FROM v2_drawers d WHERE d.branch_id=$1 AND d.opened_by=$2
 )x`, b, user.ID).Scan(&items)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return c.JSON(200, map[string]any{"items": items, "warnings": warnings, "warnings_token": token})
}

type ShiftResolutionInput struct {
	BranchID string `json:"branch_id"`
	IssueID  string `json:"issue_id"`
	Reason   string `json:"reason"`
}

func (s *Service) resolveShiftIssue(ctx context.Context, tx *sql.Tx, a Actor, in ShiftResolutionInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if a.User.RoleKey != "super_admin" {
		return nil, platform.NewError(403, "เฉพาะ Superadmin อนุมัติผลต่างเงินสดได้")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุผลตรวจสอบและเหตุผลปิดยอดค้าง")
	}
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id::text FROM v2_shift_issues WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.IssueID, b).Scan(&id); err != nil {
		return nil, err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_shift_issue_resolutions WHERE issue_id=$1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, conflict("ยอดค้างนี้มีผลตรวจสอบแล้ว")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_shift_issue_resolutions(id,operation_id,issue_id,reason,actor_id) VALUES($1,$2,$3,$4,$5)`, newID(), a.OperationID, id, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, emit(ctx, tx, a, "shift.issue_resolved", map[string]string{"id": id, "branch_id": b})
}
