package platform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func WithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func GetSettingFloat(ctx context.Context, db DBTX, key string, fallback float64) (float64, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT setting_value FROM app_settings WHERE setting_key = $1`, key).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return fallback, nil
		}
		return 0, err
	}
	var value float64
	_, err := fmt.Sscanf(raw, "%f", &value)
	if err != nil {
		return fallback, nil
	}
	return value, nil
}

func GetSettingString(ctx context.Context, db DBTX, key string, fallback string) (string, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT setting_value FROM app_settings WHERE setting_key = $1`, key).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return fallback, nil
		}
		return "", err
	}
	if raw == "" {
		return fallback, nil
	}
	return raw, nil
}

func StringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func BoolPointer(value bool) *bool {
	copy := value
	return &copy
}

func MustBranchID(user AuthUser) (string, error) {
	if user.BranchID == nil || *user.BranchID == "" {
		return "", NewError(400, "branch context is required")
	}
	return *user.BranchID, nil
}

var bangkokLocation = time.FixedZone("ICT", 7*60*60)

func InBangkok(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	return value.In(bangkokLocation)
}

func FormatSalesDocNumber(prefix string, issuedAt time.Time, next int64) string {
	return fmt.Sprintf("%s%s%05d", strings.ToUpper(strings.TrimSpace(prefix)), InBangkok(issuedAt).Format("20060102"), next)
}

func FormatBranchDocumentNumber(branchCode, prefix string, issuedAt time.Time, next int64) string {
	code := strings.ToUpper(strings.TrimSpace(branchCode))
	number := FormatSalesDocNumber(prefix, issuedAt, next)
	if code == "" {
		return number
	}
	return code + "-" + number
}
