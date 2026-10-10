package v2

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

// RunJobs is opt-in and only writes V2 tables. Notifications stay inside the
// owner workspace; sending LINE/SMS or touching V1 is not a job handler here.
func RunJobs(ctx context.Context, db *sql.DB) {
	s := &Service{db: db}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	var nextScan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for i := 0; i < 20; i++ {
				worked, err := s.processJob(ctx)
				if err != nil {
					log.Print("V2 outbox processing deferred after internal error")
					break
				}
				if !worked {
					break
				}
			}
			// Catch-up is repeatable and task uniqueness prevents duplicate bands.
			if time.Now().Before(nextScan) {
				continue
			}
			_, err := db.ExecContext(ctx, `INSERT INTO v2_expiry_tasks(id,lot_id,band_months)
 SELECT gen_random_uuid(),l.id,CASE WHEN l.expires_on<d.today THEN 0 WHEN l.expires_on<=(d.today+INTERVAL '3 months')::date THEN 3 WHEN l.expires_on<=(d.today+INTERVAL '6 months')::date THEN 6 ELSE 9 END
 FROM v2_lots l CROSS JOIN (SELECT (NOW() AT TIME ZONE 'Asia/Bangkok')::date AS today)d
 WHERE l.remaining_quantity>0 AND l.expires_on IS NOT NULL AND l.expires_on<=(d.today+INTERVAL '9 months')::date ON CONFLICT(lot_id,band_months) DO NOTHING`)
			nextScan = time.Now().Add(time.Hour)
			if err != nil {
				log.Print("V2 expiry catch-up deferred after internal error")
				nextScan = time.Now().Add(time.Minute)
			}
		}
	}
}

func (s *Service) processJob(ctx context.Context) (bool, error) {
	token := newID()
	var id, event string
	var payload []byte
	var attempt int
	// Claim has a finite lease. A crashed worker leaves work reclaimable.
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id::text,event_type,payload,attempts FROM v2_outbox WHERE
 (status='pending' AND available_at<=NOW()) OR (status='processing' AND locked_until<NOW())
 ORDER BY available_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &event, &payload, &attempt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE v2_outbox SET status='processing',attempts=attempts+1,lease_token=$2,locked_until=NOW()+INTERVAL '60 seconds' WHERE id=$1`, id, token)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	attempt++
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT lease_token=$2::uuid AND status='processing' AND locked_until>NOW() FROM v2_outbox WHERE id=$1 FOR UPDATE`, id, token).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return conflict("job lease expired")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO v2_notifications(id,outbox_id,event_type,payload) VALUES($1,$2,$3,$4::jsonb) ON CONFLICT(outbox_id) DO NOTHING`, newID(), id, event, string(payload)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE v2_outbox SET status='done',locked_until=NULL,lease_token=NULL,last_error='' WHERE id=$1 AND lease_token=$2`, id, token)
		return err
	})
	if err == nil {
		return true, nil
	}
	// Store only a generic reason; raw database errors can contain request data.
	status := "pending"
	if attempt >= 8 {
		status = "dead"
	}
	delay := 1 << min(attempt, 10)
	_, updateErr := s.db.ExecContext(ctx, `UPDATE v2_outbox SET status=$3,available_at=NOW()+($4*INTERVAL '1 second'),locked_until=NULL,lease_token=NULL,last_error='V2 internal handler failed' WHERE id=$1 AND lease_token=$2`, id, token, status, delay)
	if updateErr != nil {
		return true, updateErr
	}
	return true, err
}

type JobRetryInput struct {
	BranchID string `json:"branch_id"`
	ID       string `json:"id"`
	Reason   string `json:"reason"`
}

func (s *Service) retryJob(ctx context.Context, tx *sql.Tx, a Actor, in JobRetryInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.Reason == "" {
		return nil, bad("ระบุเหตุผล retry job")
	}
	result, err := tx.ExecContext(ctx, `UPDATE v2_outbox SET status='pending',attempts=0,available_at=NOW(),locked_until=NULL,lease_token=NULL WHERE id=$1 AND status='dead' AND payload->>'branch_id'=$2`, in.ID, b)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, conflict("ไม่พบ dead job ของสาขานี้")
	}
	return map[string]string{"id": in.ID, "status": "pending"}, emit(ctx, tx, a, "job.retried", map[string]string{"id": in.ID, "branch_id": b, "reason": in.Reason})
}

const jobsQuery = `SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (SELECT id,event_type,status,attempts,available_at,locked_until,last_error,created_at FROM v2_outbox WHERE payload->>'branch_id'=$1::text ORDER BY created_at DESC LIMIT 200)x),'[]'::jsonb),
 'notifications',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (SELECT n.* FROM v2_notifications n WHERE n.payload->>'branch_id'=$1::text ORDER BY created_at DESC LIMIT 100)x),'[]'::jsonb))`
