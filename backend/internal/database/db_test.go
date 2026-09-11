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
