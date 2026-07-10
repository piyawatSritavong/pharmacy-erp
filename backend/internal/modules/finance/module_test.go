package finance

import "testing"

func TestCheckAmountMatches(t *testing.T) {
	if !checkAmountMatches(5564, 5564) {
		t.Fatal("expected equal totals to match")
	}
	if checkAmountMatches(5564, 5564.01) {
		t.Fatal("expected mismatch totals to fail")
	}
}
