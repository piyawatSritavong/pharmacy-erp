package platform

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/lib/pq"
)

func TestIsUniqueViolation(t *testing.T) {
	unique := &pq.Error{Code: "23505"}
	if !IsUniqueViolation(unique) {
		t.Fatal("expected 23505 to be a unique violation")
	}
	if !IsUniqueViolation(fmt.Errorf("insert: %w", unique)) {
		t.Fatal("expected wrapped 23505 to be a unique violation")
	}
	if IsUniqueViolation(&pq.Error{Code: "23503"}) {
		t.Fatal("expected foreign key violation to not match")
	}
	if IsUniqueViolation(errors.New("plain error")) {
		t.Fatal("expected plain error to not match")
	}
	if IsUniqueViolation(nil) {
		t.Fatal("expected nil to not match")
	}
}

func TestMapUniqueViolation(t *testing.T) {
	mapped := MapUniqueViolation(&pq.Error{Code: "23505"}, "SKU already exists")
	var appErr *AppError
	if !errors.As(mapped, &appErr) || appErr.Code != http.StatusConflict || appErr.Message != "SKU already exists" {
		t.Fatalf("expected 409 AppError, got %#v", mapped)
	}
	plain := errors.New("boom")
	if MapUniqueViolation(plain, "x") != plain {
		t.Fatal("expected non-unique error to pass through")
	}
}
