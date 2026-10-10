package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// The pool is sized for one long-lived instance, because that is what Render
// runs: a service with a fixed instance count, not containers the platform
// brings up and tears down under load.
//
// It was briefly 5. That was correct for Cloud Run, where the number that
// matters is instances × pool size against the pooler's limit, and ten
// instances of twenty would have asked for two hundred connections. On a single
// instance the same 5 is just a queue: the sixth concurrent request waits for a
// connection instead of a database. Do not reduce these without first changing
// how many instances run — the two numbers are one decision.
const (
	MaxOpenConns = 20
	MaxIdleConns = 10
)

func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", EnsureSSLMode(databaseURL))
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(MaxOpenConns)
	db.SetMaxIdleConns(MaxIdleConns)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return db, nil
}

// OpenTest refuses to connect unless both the process environment and database
// name make the destructive integration-test intent explicit. Test fixtures
// truncate and rewrite tables, so accepting an arbitrary TEST_DATABASE_URL is
// not a safe enough boundary by itself.
func OpenTest(databaseURL string) (*sql.DB, error) {
	if err := ValidateTestDatabaseURL(databaseURL, os.Getenv("APP_ENV")); err != nil {
		return nil, err
	}
	return Open(databaseURL)
}

func ValidateTestDatabaseURL(databaseURL, appEnv string) error {
	if appEnv != "test" {
		return fmt.Errorf("integration database requires APP_ENV=test")
	}
	parsed, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return fmt.Errorf("TEST_DATABASE_URL must use postgres:// or postgresql:// URL form")
	}
	databaseName := strings.TrimPrefix(path.Clean(parsed.Path), "/")
	if databaseName == "" || databaseName == "." || !strings.HasSuffix(strings.ToLower(databaseName), "_test") {
		return fmt.Errorf("TEST_DATABASE_URL database name must end with _test")
	}
	return nil
}

// EnsureSSLMode defaults a DSN that does not say to sslmode=require.
//
// lib/pq's own default is "prefer", which falls back to plaintext without
// complaint when the server declines TLS — so a DSN that simply forgets to say
// gets an unencrypted link to a managed database across the public internet,
// and nothing in the logs marks it. A DSN that states its own sslmode, local
// compose's sslmode=disable included, is left exactly as written.
func EnsureSSLMode(databaseURL string) string {
	trimmed := strings.TrimSpace(databaseURL)
	if trimmed == "" {
		return trimmed
	}

	// A key/value DSN ("host=... sslmode=...") is not a URL; only the URL form
	// is rewritten, and the key/value form is returned untouched rather than
	// mangled.
	if !strings.HasPrefix(trimmed, "postgres://") && !strings.HasPrefix(trimmed, "postgresql://") {
		return trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	query := parsed.Query()
	if query.Get("sslmode") != "" {
		return trimmed
	}
	query.Set("sslmode", "require")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
