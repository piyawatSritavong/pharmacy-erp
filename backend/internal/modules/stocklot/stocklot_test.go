package stocklot

import "testing"

func TestWeightedCost(t *testing.T) {
	allocations := []Allocation{
		{Lot: Lot{UnitCost: 10}, Quantity: 2},
		{Lot: Lot{UnitCost: 16}, Quantity: 1},
	}
	if got := WeightedCost(allocations); got != 12 {
		t.Fatalf("expected weighted cost 12, got %.2f", got)
	}
	if got := WeightedCost(nil); got != 0 {
		t.Fatalf("expected empty weighted cost 0, got %.2f", got)
	}
}

func TestValidateBucket(t *testing.T) {
	for _, bucket := range []string{"real", "ghost"} {
		if err := ValidateBucket(bucket); err != nil {
			t.Fatalf("expected %s bucket to be accepted: %v", bucket, err)
		}
	}
	if err := ValidateBucket("secret"); err == nil {
		t.Fatal("expected unknown stock bucket to fail")
	}
}
