package app

import (
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/config"
)

// The seed used to compile its admin password in as "DevPassword123!", shared
// by two global-scope accounts and printed on the public login page, with no
// variable that could change it. These are the rules that replaced it.
func TestSeedPasswordsAreRequiredAndReal(t *testing.T) {
	strongAdmin := "AdminPassphrase-2026!"
	strongPOS := "TillPassphrase-2026!"

	for _, test := range []struct {
		name  string
		admin string
		pos   string
		wants string
	}{
		{name: "both strong and distinct", admin: strongAdmin, pos: strongPOS},
		{name: "admin unset", admin: "", pos: strongPOS, wants: "SEED_ADMIN_PASSWORD is not set"},
		{name: "pos unset", admin: strongAdmin, pos: "", wants: "SEED_POS_PASSWORD is not set"},
		{name: "admin is only whitespace", admin: "        ", pos: strongPOS, wants: "SEED_ADMIN_PASSWORD is not set"},
		{name: "admin too short", admin: "Sh0rt-Pass!", pos: strongPOS, wants: "SEED_ADMIN_PASSWORD is shorter than 16"},
		{name: "pos too short", admin: strongAdmin, pos: "Sh0rt-Pass!", wants: "SEED_POS_PASSWORD is shorter than 16"},
		{name: "exactly at the floor is accepted", admin: "0123456789abcdef", pos: strongPOS},
		{
			// The tills would otherwise hold the head-office credential, which
			// is what shipping one password for everything already meant.
			name: "the same password for both", admin: strongAdmin, pos: strongAdmin,
			wants: "are the same",
		},
		{name: "the old default is refused on length", admin: "DevPassword123!", pos: strongPOS, wants: "shorter than 16"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateSeedPasswords(config.Config{SeedAdminPassword: test.admin, SeedPOSPassword: test.pos})
			if test.wants == "" {
				if err != nil {
					t.Fatalf("expected this to be accepted, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a refusal mentioning %q", test.wants)
			}
			if !strings.Contains(err.Error(), test.wants) {
				t.Fatalf("expected the message to name the problem (%q), got %q", test.wants, err)
			}
		})
	}
}

// The message has to name the variable, because the person reading it is
// setting environment variables and needs to know which one.
func TestSeedRefusalNamesTheVariableAndTheAccounts(t *testing.T) {
	err := validateSeedPasswords(config.Config{SeedPOSPassword: "TillPassphrase-2026!"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, want := range []string{"SEED_ADMIN_PASSWORD", "superadmin@erp.local", "admin.central@erp.local"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %q in the message, got %q", want, err)
		}
	}
}

func TestPOSPasswordHasNoFallback(t *testing.T) {
	if _, err := normalizedPOSPassword(config.Config{}); err == nil {
		t.Fatal("an unset SEED_POS_PASSWORD must not silently become a default")
	}
	if _, err := normalizedPOSPassword(config.Config{SeedPOSPassword: "DevPassword123!"}); err == nil {
		t.Fatal("the old default is too short and must be refused")
	}
	if _, err := normalizedPOSPassword(config.Config{SeedPOSPassword: "TillPassphrase-2026!"}); err != nil {
		t.Fatalf("a real password should be accepted: %v", err)
	}
}
