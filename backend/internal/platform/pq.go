package platform

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/lib/pq"
)

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// IsNotFound reports whether err, wrapped as a server fault, actually means the
// requested record does not exist: no row, or an id that is not even a valid
// UUID (SQLSTATE 22P02). A deliberate AppError below 500 is never rewritten.
func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Code < http.StatusInternalServerError {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "22P02" && strings.Contains(pqErr.Message, "uuid")
}

// MapUniqueViolation converts a unique constraint violation into a 409
// AppError with the given message; other errors pass through unchanged.
func MapUniqueViolation(err error, message string) error {
	if IsUniqueViolation(err) {
		return NewError(http.StatusConflict, message)
	}
	return err
}
