package sales

import (
	"database/sql"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

func TestResolveLineDisplayAndPriceGovernmentMode(t *testing.T) {
	displayName, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
		"เตียงผู้ป่วย",
		5400,
		0,
		0,
		"",
		sql.NullString{String: "alias", Valid: true},
		sql.NullString{String: "ผ้าอ้อมผู้ป่วย", Valid: true},
		sql.NullFloat64{Float64: 5200, Valid: true},
		true,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if displayName != "ผ้าอ้อมผู้ป่วย" || unitPrice != 5200 || priceSource != "government_alias_default" {
		t.Fatalf("unexpected government resolution: %s %.2f %s", displayName, unitPrice, priceSource)
	}
}

func TestResolveLineDisplayAndPriceTiers(t *testing.T) {
	cases := []struct {
		name         string
		tier         string
		retailPrice  float64
		installPrice float64
		wantPrice    float64
		wantSource   string
	}{
		{"retail tier set", "retail", 120, 150, 120, "retail_tier"},
		{"installment tier set", "installment", 120, 150, 150, "installment_tier"},
		{"retail tier not set falls back", "retail", 0, 150, 100, "branch_price"},
		{"cash tier uses branch price", "cash", 120, 150, 100, "branch_price"},
		{"empty tier uses branch price", "", 120, 150, 100, "branch_price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
				"สินค้า", 100, tc.retailPrice, tc.installPrice, tc.tier,
				sql.NullString{}, sql.NullString{}, sql.NullFloat64{},
				false, nil, false,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if unitPrice != tc.wantPrice || priceSource != tc.wantSource {
				t.Fatalf("unexpected tier resolution: %.2f %s (want %.2f %s)", unitPrice, priceSource, tc.wantPrice, tc.wantSource)
			}
		})
	}
}

func TestResolveLineDisplayAndPriceOverrideBeatsTier(t *testing.T) {
	override := 99.0
	_, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
		"สินค้า", 100, 120, 150, "retail",
		sql.NullString{}, sql.NullString{}, sql.NullFloat64{},
		false, &override, true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unitPrice != 99 || priceSource != "override" {
		t.Fatalf("expected override to win over tier: %.2f %s", unitPrice, priceSource)
	}
}

func TestCalculateLineAmounts(t *testing.T) {
	lineSubtotal, lineTaxRate, lineTaxAmount, lineTotal := calculateLineAmounts(100, 2, 7, false)
	if lineSubtotal != 200 || lineTaxRate != 7 || lineTaxAmount != 14 || lineTotal != 214 {
		t.Fatalf("unexpected totals: %.2f %.2f %.2f %.2f", lineSubtotal, lineTaxRate, lineTaxAmount, lineTotal)
	}
}

func TestValidateAliasSelection(t *testing.T) {
	aliasID := "alias-1"
	if err := validateAliasSelection(&aliasID, sql.NullString{String: aliasID, Valid: true}); err != nil {
		t.Fatalf("expected matching alias to pass: %v", err)
	}
	if err := validateAliasSelection(&aliasID, sql.NullString{}); err == nil {
		t.Fatal("expected mismatched alias to fail")
	}
	if err := validateAliasSelection(nil, sql.NullString{}); err != nil {
		t.Fatalf("expected empty alias to pass: %v", err)
	}
}

func TestFormatSalesDocNumber(t *testing.T) {
	issuedAt := time.Date(2026, 4, 15, 1, 30, 0, 0, time.UTC)
	value := platform.FormatSalesDocNumber("bl", issuedAt, 42)
	if value != "BL2026041500042" {
		t.Fatalf("unexpected formatted number: %s", value)
	}
}

func TestValidatePaymentCollector(t *testing.T) {
	if err := validatePaymentCollector(platform.AuthUser{RoleKey: "branch_pos"}); err != nil {
		t.Fatalf("expected branch POS to collect payment: %v", err)
	}
	if err := validatePaymentCollector(platform.AuthUser{RoleKey: "branch_admin"}); err == nil {
		t.Fatal("expected branch admin payment collection to be rejected")
	}
	if err := validatePaymentCollector(platform.AuthUser{RoleKey: "super_admin"}); err == nil {
		t.Fatal("expected super admin direct payment collection to be rejected")
	}
}
