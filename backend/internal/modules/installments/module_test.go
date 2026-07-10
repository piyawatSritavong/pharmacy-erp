package installments

import (
	"testing"

	"pharmacy-erp/backend/internal/platform"
)

func TestSplitInstallmentsSumMatchesTotal(t *testing.T) {
	cases := []struct {
		name   string
		total  float64
		months int
	}{
		{"even split", 1200, 12},
		{"rounding drift", 1000, 3},
		{"single month", 535.50, 1},
		{"satang precision", 999.99, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			monthly, last := splitInstallments(tc.total, tc.months)
			sum := platform.Round2(monthly*float64(tc.months-1) + last)
			if sum != tc.total {
				t.Fatalf("installments sum %.2f does not match total %.2f (monthly %.2f, last %.2f)", sum, tc.total, monthly, last)
			}
			if monthly <= 0 || last <= 0 {
				t.Fatalf("expected positive amounts, got monthly %.2f last %.2f", monthly, last)
			}
		})
	}
}

func TestSplitInstallmentsEvenCase(t *testing.T) {
	monthly, last := splitInstallments(1200, 12)
	if monthly != 100 || last != 100 {
		t.Fatalf("expected 100/100, got %.2f/%.2f", monthly, last)
	}
}
