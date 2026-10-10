package database

import "testing"

func TestEnsureSSLMode(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a URL that says nothing gets require",
			in:   "postgres://u:p@db.example.com:6543/postgres",
			want: "postgres://u:p@db.example.com:6543/postgres?sslmode=require",
		},
		{
			name: "local compose keeps its own disable",
			in:   "postgres://pharmacy:pharmacy@database:5432/pharmacy_erp?sslmode=disable",
			want: "postgres://pharmacy:pharmacy@database:5432/pharmacy_erp?sslmode=disable",
		},
		{
			name: "an explicit verify-full is not downgraded",
			in:   "postgresql://u:p@host/db?sslmode=verify-full",
			want: "postgresql://u:p@host/db?sslmode=verify-full",
		},
		{
			name: "other parameters survive",
			in:   "postgres://u:p@host/db?application_name=pharmacy",
			want: "postgres://u:p@host/db?application_name=pharmacy&sslmode=require",
		},
		{
			name: "a key/value DSN is left alone rather than mangled",
			in:   "host=localhost user=pharmacy dbname=pharmacy_erp",
			want: "host=localhost user=pharmacy dbname=pharmacy_erp",
		},
		{name: "empty stays empty", in: "", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := EnsureSSLMode(test.in); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestValidateTestDatabaseURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		env   string
		valid bool
	}{
		{name: "isolated test database", url: "postgres://user:secret@127.0.0.1/pharmacy_v2_test?sslmode=disable", env: "test", valid: true},
		{name: "production environment", url: "postgres://user:secret@127.0.0.1/pharmacy_v2_test", env: "production"},
		{name: "production database name", url: "postgres://user:secret@127.0.0.1/pharmacy", env: "test"},
		{name: "ambiguous contest name", url: "postgres://user:secret@127.0.0.1/contest_prod", env: "test"},
		{name: "ambiguous latest name", url: "postgres://user:secret@127.0.0.1/latest", env: "test"},
		{name: "key value DSN", url: "host=localhost dbname=pharmacy_test", env: "test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTestDatabaseURL(test.url, test.env)
			if test.valid && err != nil {
				t.Fatalf("expected valid test URL: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected unsafe test URL to be rejected")
			}
		})
	}
}
