package http

import (
	"testing"

	"pharmacy-erp/backend/internal/config"
)

func TestAllowedOrigins(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  config.Config
		want []string
	}{
		{
			name: "production trusts the configured frontend and nothing else",
			cfg:  config.Config{AppEnv: "production", FrontendURL: "https://mes.ihavepro.com"},
			want: []string{"https://mes.ihavepro.com"},
		},
		{
			name: "development also trusts the dev server",
			cfg:  config.Config{AppEnv: "development", FrontendURL: "http://localhost:3000"},
			want: []string{"http://localhost:3000"},
		},
		{
			name: "a trailing slash does not create a second origin that never matches",
			cfg:  config.Config{AppEnv: "production", FrontendURL: "https://mes.ihavepro.com/"},
			want: []string{"https://mes.ihavepro.com"},
		},
		{
			name: "an unset frontend url in production allows nobody rather than everybody",
			cfg:  config.Config{AppEnv: "production"},
			want: []string{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := allowedOrigins(test.cfg)
			if len(got) != len(test.want) {
				t.Fatalf("expected %v, got %v", test.want, got)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("expected %v, got %v", test.want, got)
				}
			}
			for _, origin := range got {
				if origin == "*" {
					t.Fatal("a wildcard must never appear in a credentialed allow-list")
				}
			}
		})
	}
}

// The dev origin must not be trusted by a production deployment, whatever the
// frontend url happens to be.
func TestProductionNeverTrustsLocalhost(t *testing.T) {
	for _, origin := range allowedOrigins(config.Config{AppEnv: "production", FrontendURL: "https://mes.ihavepro.com"}) {
		if origin == "http://localhost:3000" {
			t.Fatal("production must not allow the dev origin")
		}
	}
}
