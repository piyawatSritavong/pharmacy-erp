package v2

import (
	"context"
	"database/sql"
	"strings"
)

type QuotationRevisionInput struct {
	BranchID string        `json:"branch_id"`
	ID       string        `json:"id"`
	Reason   string        `json:"reason"`
	Document DocumentInput `json:"document"`
}

func (s *Service) reviseQuotation(ctx context.Context, tx *sql.Tx, a Actor, in QuotationRevisionInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุเหตุผลแก้ใบเสนอราคา")
	}
	var kind, status string
	if err = tx.QueryRowContext(ctx, `SELECT kind,status FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ID, b).Scan(&kind, &status); err != nil {
		return nil, err
	}
	if kind != "quotation" || status != "draft" {
		return nil, conflict("แก้ไขได้เฉพาะใบเสนอราคาฉบับร่าง")
	}
	in.Document.BranchID = b
	in.Document.Kind = "quotation"
	// A new immutable revision preserves every fact of the previous quotation.
	result, err := s.createDocument(ctx, tx, a, in.Document)
	if err != nil {
		return nil, err
	}
	next := result.(map[string]any)
	id := next["id"].(string)
	// The revision link is separate evidence; original document facts stay fixed.
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_quotation_revisions(id,operation_id,previous_id,next_id,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, newID(), a.OperationID, in.ID, id, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_documents SET status='cancelled' WHERE id=$1`, in.ID); err != nil {
		return nil, err
	}
	return result, emit(ctx, tx, a, "quotation.revised", map[string]string{"id": id, "previous_id": in.ID, "branch_id": b})
}
