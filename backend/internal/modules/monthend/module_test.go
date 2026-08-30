package monthend

import "testing"

func TestClosestQuantityChoosesNearestUnitCount(t *testing.T) {
	if got := closestQuantity(260, 100, 10); got != 3 {
		t.Fatalf("expected 3 units, got %d", got)
	}
	if got := closestQuantity(240, 100, 10); got != 2 {
		t.Fatalf("expected 2 units, got %d", got)
	}
	if got := closestQuantity(10, 100, 10); got != 0 {
		t.Fatalf("an overshoot that is farther from target must not be selected, got %d", got)
	}
}

func TestCalculateScenarioUsesGhostBeforePriceScenario(t *testing.T) {
	calculation := Calculation{
		ActualRevenue: 1000,
		TargetRevenue: 105,
		MarkupPercent: 5,
		Lines: []Line{{
			BranchID: "branch", ProductID: "product", PaymentType: "cash",
			TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 2, GhostAvailable: 1, OriginalUnitPrice: 500,
			CostSnapshot: 100, OriginalLineTotal: 1000, ScenarioLineTotal: 1000,
		}},
	}

	calculateScenario(&calculation)

	line := calculation.Lines[0]
	if line.AllocatedGhostQuantity != 1 || line.RepricedQuantity != 1 {
		t.Fatalf("expected one ghost and one repriced unit, got ghost=%d repriced=%d", line.AllocatedGhostQuantity, line.RepricedQuantity)
	}
	if line.AdjustmentType != "ghost_and_price" {
		t.Fatalf("expected combined adjustment, got %s", line.AdjustmentType)
	}
	if calculation.GhostReclassificationAmount != 500 || calculation.PriceScenarioReduction != 395 {
		t.Fatalf("unexpected reductions %.2f / %.2f", calculation.GhostReclassificationAmount, calculation.PriceScenarioReduction)
	}
	if calculation.ScenarioRevenue != 105 || calculation.UnresolvedDifference != 0 {
		t.Fatalf("expected exact scenario 105, got %.2f difference %.2f", calculation.ScenarioRevenue, calculation.UnresolvedDifference)
	}
}

func TestCalculateScenarioLocksFullTaxInvoice(t *testing.T) {
	calculation := Calculation{
		ActualRevenue: 1000,
		TargetRevenue: 0,
		MarkupPercent: 5,
		Lines: []Line{{
			BranchID: "branch", ProductID: "product", PaymentType: "cash",
			TaxInvoiceType: "full", OriginalStockBucket: "real", AdjustmentType: "locked_full_tax",
			Quantity: 1, GhostAvailable: 10, OriginalUnitPrice: 1000,
			CostSnapshot: 100, OriginalLineTotal: 1000, ScenarioLineTotal: 1000,
		}},
	}

	calculateScenario(&calculation)

	line := calculation.Lines[0]
	if line.AllocatedGhostQuantity != 0 || line.RepricedQuantity != 0 {
		t.Fatalf("full tax line must remain untouched: %#v", line)
	}
	if calculation.ScenarioRevenue != 1000 || calculation.UnresolvedDifference != 1000 {
		t.Fatalf("full tax amount must remain in scenario, got %.2f", calculation.ScenarioRevenue)
	}
}

func TestCalculateScenarioRejectsNonCashCandidate(t *testing.T) {
	calculation := Calculation{
		ActualRevenue: 500,
		TargetRevenue: 0,
		MarkupPercent: 5,
		Lines: []Line{{
			BranchID: "branch", ProductID: "product", PaymentType: "bank_transfer",
			TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 500,
			CostSnapshot: 100, OriginalLineTotal: 500, ScenarioLineTotal: 500,
		}},
	}

	calculateScenario(&calculation)

	if calculation.ScenarioRevenue != 500 || calculation.GhostReclassificationAmount != 0 || calculation.PriceScenarioReduction != 0 {
		t.Fatalf("non-cash line must remain unchanged: %#v", calculation)
	}
}
