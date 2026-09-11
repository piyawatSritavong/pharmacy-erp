package database

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// MaxOpenConnsPerInstance is deliberately small. Cloud Run runs many instances
// of this process and each one holds its own pool, so the ceiling that matters
// is instances × this number against the Supabase pooler's limit — not what a
// single box could keep busy. Twenty here, at a max-instances of ten, is two
// hundred connections for a pooler that will not give them.
const MaxOpenConnsPerInstance = 5

func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", EnsureSSLMode(databaseURL))
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(MaxOpenConnsPerInstance)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return db, nil
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
