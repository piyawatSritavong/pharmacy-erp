package platform

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type AppError struct {
	Code       int
	Message    string
	WrappedErr error
}

func (e *AppError) Error() string {
	if e.WrappedErr == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.WrappedErr)
}

func NewError(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func WrapError(code int, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, WrappedErr: err}
}

func JSON(c echo.Context, code int, payload any) error {
	return c.JSON(code, payload)
}

func JSONMessage(c echo.Context, code int, message string) error {
	return c.JSON(code, map[string]any{"message": message})
}

func HandleHTTPError(c echo.Context, err error) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return c.JSON(appErr.Code, ErrorResponse{Message: appErr.Message})
	}
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "internal server error"})
}

func MustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func Round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func NullString(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func NullFloat64(value *float64) sql.NullFloat64 {
	if value == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: Round2(*value), Valid: true}
}

func NullUUID(value *string) sql.NullString {
	if value == nil || strings.TrimSpace(*value) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func MustUUID() string {
	return uuid.NewString()
}

func Timestamp() time.Time {
	return time.Now().UTC()
}
