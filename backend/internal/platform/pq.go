package platform

import (
	"errors"
	"net/http"

	"github.com/lib/pq"
)

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// MapUniqueViolation converts a unique constraint violation into a 409
// AppError with the given message; other errors pass through unchanged.
func MapUniqueViolation(err error, message string) error {
	if IsUniqueViolation(err) {
		return NewError(http.StatusConflict, message)
	}
	return err
}
