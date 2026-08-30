package monthend

import (
	"reflect"
	"testing"
)

func TestValidateDraftCancellation(t *testing.T) {
	valid := PeriodInput{Confirmation: "MEC-001", Reason: "สร้างรอบผิดเดือน", LockVersion: 3}
	if err := validateDraftCancellation("DRAFT", "MEC-001", 3, valid); err != nil {
		t.Fatalf("expected valid cancellation: %v", err)
	}
	tests := []struct {
		name   string
		status string
		input  PeriodInput
	}{
		{"non draft", "CLOSED", valid},
		{"stale lock", "DRAFT", PeriodInput{Confirmation: "MEC-001", Reason: "เหตุผล", LockVersion: 2}},
		{"missing lock", "DRAFT", PeriodInput{Confirmation: "MEC-001", Reason: "เหตุผล"}},
		{"wrong confirmation", "DRAFT", PeriodInput{Confirmation: "MEC-002", Reason: "เหตุผล", LockVersion: 3}},
		{"missing reason", "DRAFT", PeriodInput{Confirmation: "MEC-001", LockVersion: 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateDraftCancellation(test.status, "MEC-001", 3, test.input); err == nil {
				t.Fatal("expected cancellation validation to fail")
			}
		})
	}
}

func baseCalculation() Calculation {
	return Calculation{ActualRevenue: 1000, Lines: []Line{{
		InvoiceID: "invoice-1", BranchID: "branch", ProductID: "product", PaymentType: "cash",
		TaxInvoiceType: "abbreviated", OriginalStockBucket: "real", Quantity: 2,
		GhostAvailable: 1, OriginalUnitPrice: 500, CostSnapshot: 100,
		OriginalLineTotal: 1000, ScenarioLineTotal: 1000, Included: true,
	}}}
}

func TestParseDecimalCentsWhole(t *testing.T)       { assertCents(t, "125", 12500) }
func TestParseDecimalCentsOneDecimal(t *testing.T)  { assertCents(t, "125.4", 12540) }
func TestParseDecimalCentsTwoDecimals(t *testing.T) { assertCents(t, "125.49", 12549) }
func TestParseDecimalCentsComma(t *testing.T)       { assertCents(t, "1,125.49", 112549) }
func TestParseDecimalCentsNegative(t *testing.T)    { assertCents(t, "-0.30", -30) }
func TestParseDecimalCentsEmpty(t *testing.T)       { assertCents(t, "", 0) }

func TestParseDecimalCentsRejectsTooManyDecimals(t *testing.T) {
	if _, err := parseDecimalCents("1.001"); err == nil {
		t.Fatal("expected precision error")
	}
}

func TestParseDecimalCentsRejectsNonNumber(t *testing.T) {
	if _, err := parseDecimalCents("one"); err == nil {
		t.Fatal("expected numeric error")
	}
}

func TestCentsFloatBoundaryPositive(t *testing.T) {
	if got := centsFromFloat(centsToFloat(10530)); got != 10530 {
		t.Fatalf("got %d", got)
	}
}

func TestCentsFloatBoundaryNegativeSubunit(t *testing.T) {
	if got := centsFromFloat(centsToFloat(-30)); got != -30 {
		t.Fatalf("got %d", got)
	}
}

func TestRoundedRatioRoundsHalfUp(t *testing.T) {
	if got := roundedRatio(105, 2); got != 53 {
		t.Fatalf("got %d", got)
	}
}

func TestClosestQuantityCentsHonorsLimit(t *testing.T) {
	if got := closestQuantityCents(1000, 100, 3); got != 3 {
		t.Fatalf("got %d", got)
	}
}

func TestClosestQuantityCentsChoosesNearest(t *testing.T) {
	if got := closestQuantityCents(260, 100, 10); got != 3 {
		t.Fatalf("got %d", got)
	}
}

func TestClosestQuantityCentsRejectsInvalidInputs(t *testing.T) {
	for _, input := range [][3]int64{{0, 100, 1}, {100, 0, 1}, {100, 100, 0}} {
		if got := closestQuantityCents(input[0], input[1], int(input[2])); got != 0 {
			t.Fatalf("got %d for %#v", got, input)
		}
	}
}

func TestDeterministicCalculationReclassifiesSecondaryStockFirst(t *testing.T) {
	result := calculateDeterministic(baseCalculation(), 10500, 500)
	if result.Lines[0].AllocatedGhostQuantity != 1 || result.Lines[0].RepricedQuantity != 1 {
		t.Fatalf("unexpected line %#v", result.Lines[0])
	}
}

func TestDeterministicCalculationLocksFullTax(t *testing.T) {
	source := baseCalculation()
	source.Lines[0].TaxInvoiceType = "full"
	result := calculateDeterministic(source, 0, 500)
	if result.Lines[0].AllocatedGhostQuantity != 0 || result.Lines[0].RepricedQuantity != 0 {
		t.Fatal("full tax was changed")
	}
}

func TestDeterministicCalculationRejectsNonCash(t *testing.T) {
	source := baseCalculation()
	source.Lines[0].PaymentType = "bank_transfer"
	result := calculateDeterministic(source, 0, 500)
	if result.ScenarioRevenue != 1000 {
		t.Fatalf("got %.2f", result.ScenarioRevenue)
	}
}

func TestDeterministicCalculationDoesNotMutateInput(t *testing.T) {
	source := baseCalculation()
	before := append([]Line(nil), source.Lines...)
	_ = calculateDeterministic(source, 10500, 500)
	if !reflect.DeepEqual(source.Lines, before) {
		t.Fatal("source lines were mutated")
	}
}

func TestDeterministicCalculationIsRepeatable(t *testing.T) {
	a := calculateDeterministic(baseCalculation(), 10500, 500)
	b := calculateDeterministic(baseCalculation(), 10500, 500)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same input produced different results")
	}
}

func TestDeterministicCalculationCountsAffectedInvoicesOnce(t *testing.T) {
	source := baseCalculation()
	second := source.Lines[0]
	second.ProductID = "product-2"
	second.OriginalLineTotal = 500
	second.ScenarioLineTotal = 500
	source.Lines = append(source.Lines, second)
	source.ActualRevenue = 1500
	result := calculateDeterministic(source, 0, 500)
	if result.AffectedInvoices != 1 {
		t.Fatalf("got %d", result.AffectedInvoices)
	}
}

func TestDeterministicCalculationSharesAvailableStockAcrossLines(t *testing.T) {
	source := baseCalculation()
	second := source.Lines[0]
	second.InvoiceID = "invoice-2"
	source.Lines = append(source.Lines, second)
	source.ActualRevenue = 2000
	result := calculateDeterministic(source, 0, 500)
	allocated := result.Lines[0].AllocatedGhostQuantity + result.Lines[1].AllocatedGhostQuantity
	if allocated != 1 {
		t.Fatalf("allocated %d", allocated)
	}
}

func TestDeterministicCalculationTargetActualHasNoProposal(t *testing.T) {
	result := calculateDeterministic(baseCalculation(), 100000, 500)
	if result.AffectedItems != 0 || result.UnresolvedDifference != 0 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestDeterministicCalculationReportsExactTarget(t *testing.T) {
	result := calculateDeterministic(baseCalculation(), 10500, 500)
	if result.UnresolvedDifference != 0 {
		t.Fatalf("difference %.2f", result.UnresolvedDifference)
	}
}

func TestDeterministicCalculationReportsGP(t *testing.T) {
	result := calculateDeterministic(baseCalculation(), 10500, 500)
	if result.GPBefore != 80000 || result.GPAfter >= result.GPBefore {
		t.Fatalf("gp before=%d after=%d", result.GPBefore, result.GPAfter)
	}
}

func TestDeterministicCalculationSkipsPriceWhenNotBelowOriginal(t *testing.T) {
	source := baseCalculation()
	source.Lines[0].GhostAvailable = 0
	source.Lines[0].CostSnapshot = 600
	result := calculateDeterministic(source, 0, 500)
	if result.Lines[0].RepricedQuantity != 0 {
		t.Fatal("invalid higher simulated price was used")
	}
}

func TestDeterministicCalculationUsesIntegerTaxRounding(t *testing.T) {
	source := baseCalculation()
	source.Lines[0].GhostAvailable = 0
	source.Lines[0].TaxRate = 7
	result := calculateDeterministic(source, 0, 500)
	if centsFromFloat(result.Lines[0].ProposedUnitPrice) != 10500 {
		t.Fatalf("price %.2f", result.Lines[0].ProposedUnitPrice)
	}
}

func TestClosestStrategyEvaluatesAllFeasibleLines(t *testing.T) {
	source := Calculation{ActualRevenue: 160, Lines: []Line{
		{
			InvoiceID: "old", BranchID: "branch", ProductID: "product-old", PaymentType: "cash",
			CashPaymentAmount: 100, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 100, CostSnapshot: 100,
			OriginalLineTotal: 100, ScenarioLineTotal: 100,
		},
		{
			InvoiceID: "exact", BranchID: "branch", ProductID: "product-exact", PaymentType: "cash",
			CashPaymentAmount: 60, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 60, CostSnapshot: 60,
			OriginalLineTotal: 60, ScenarioLineTotal: 60,
		},
	}}

	result := calculateDeterministicWithStrategy(source, 10000, 500, "closest_then_oldest")
	if result.UnresolvedDifference != 0 || result.Lines[1].AllocatedGhostQuantity != 1 {
		t.Fatalf("closest strategy did not choose the exact line: %#v", result)
	}
}

func TestOldestFirstStrategyUsesChronologicalOrder(t *testing.T) {
	source := Calculation{ActualRevenue: 160, Lines: []Line{
		{
			InvoiceID: "old", BranchID: "branch", ProductID: "product-old", PaymentType: "cash",
			CashPaymentAmount: 100, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 100, CostSnapshot: 100,
			OriginalLineTotal: 100, ScenarioLineTotal: 100,
		},
		{
			InvoiceID: "exact", BranchID: "branch", ProductID: "product-exact", PaymentType: "cash",
			CashPaymentAmount: 60, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 60, CostSnapshot: 60,
			OriginalLineTotal: 60, ScenarioLineTotal: 60,
		},
	}}

	result := calculateDeterministicWithStrategy(source, 10000, 500, "oldest_first")
	if result.Lines[0].AllocatedGhostQuantity != 1 || result.UnresolvedDifference != -40 {
		t.Fatalf("oldest strategy was not honored: %#v", result)
	}
}

func TestMixedPaymentReductionNeverExceedsCashPortion(t *testing.T) {
	source := Calculation{ActualRevenue: 100, Lines: []Line{{
		InvoiceID: "mixed", BranchID: "branch", ProductID: "product", PaymentType: "mixed",
		CashPaymentAmount: 40, TransferPaymentAmount: 60, TaxInvoiceType: "abbreviated",
		OriginalStockBucket: "real", Quantity: 2, GhostAvailable: 2, OriginalUnitPrice: 50,
		CostSnapshot: 50, OriginalLineTotal: 100, ScenarioLineTotal: 100,
	}}}

	result := calculateDeterministic(source, 0, 500)
	if result.GhostReclassificationAmount != 0 || result.PriceScenarioReduction != 0 {
		t.Fatalf("reduction exceeded indivisible cash cap: %#v", result)
	}
}

func TestClosestStrategyCombinesGhostAndPriceForNearestTarget(t *testing.T) {
	source := Calculation{ActualRevenue: 166855.80, Lines: []Line{
		{
			InvoiceID: "bill-1", BranchID: "branch", ProductID: "a", PaymentType: "cash",
			CashPaymentAmount: 14445, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 3, OriginalUnitPrice: 13500, CostSnapshot: 9800,
			TaxRate: 7, OriginalLineTotal: 14445, ScenarioLineTotal: 14445,
		},
		{
			InvoiceID: "bill-2", BranchID: "branch", ProductID: "a", PaymentType: "cash",
			CashPaymentAmount: 14445, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 3, OriginalUnitPrice: 13500, CostSnapshot: 9800,
			TaxRate: 7, OriginalLineTotal: 14445, ScenarioLineTotal: 14445,
		},
		{
			InvoiceID: "bill-3", BranchID: "branch", ProductID: "a", PaymentType: "cash",
			CashPaymentAmount: 14445, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 3, OriginalUnitPrice: 13500, CostSnapshot: 9800,
			TaxRate: 7, OriginalLineTotal: 14445, ScenarioLineTotal: 14445,
		},
		{
			InvoiceID: "bill-4", BranchID: "branch", ProductID: "price", PaymentType: "cash",
			CashPaymentAmount: 12733, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 0, OriginalUnitPrice: 11900, CostSnapshot: 8200,
			TaxRate: 7, OriginalLineTotal: 12733, ScenarioLineTotal: 12733,
		},
		{
			InvoiceID: "bill-5", BranchID: "branch", ProductID: "overshoot", PaymentType: "cash",
			CashPaymentAmount: 6955, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
			Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 6500, CostSnapshot: 3800,
			TaxRate: 7, OriginalLineTotal: 6955, ScenarioLineTotal: 6955,
		},
	}}

	result := calculateDeterministicWithStrategy(source, 12000080, 500, "closest_then_oldest")
	if centsFromFloat(result.GhostReclassificationAmount) != 4333500 {
		t.Fatalf("unexpected ghost reduction %.2f", result.GhostReclassificationAmount)
	}
	if centsFromFloat(result.PriceScenarioReduction) != 352030 {
		t.Fatalf("unexpected price reduction %.2f", result.PriceScenarioReduction)
	}
	if centsFromFloat(result.UnresolvedDifference) != -30 {
		t.Fatalf("expected a 0.30 overshoot, got %.2f", result.UnresolvedDifference)
	}
	if result.Lines[3].RepricedQuantity != 1 || result.Lines[4].AllocatedGhostQuantity != 0 {
		t.Fatalf("optimizer chose the farther ghost action: %#v", result.Lines)
	}
}

func TestGPAfterExcludesCostOfGhostAllocatedQuantity(t *testing.T) {
	source := Calculation{ActualRevenue: 500, Lines: []Line{{
		InvoiceID: "invoice", BranchID: "branch", ProductID: "product", PaymentType: "cash",
		CashPaymentAmount: 500, TaxInvoiceType: "abbreviated", OriginalStockBucket: "real",
		Quantity: 1, GhostAvailable: 1, OriginalUnitPrice: 500, CostSnapshot: 100,
		OriginalLineTotal: 500, ScenarioLineTotal: 500,
	}}}

	result := calculateDeterministic(source, 0, 500)
	if result.GPAfter != 0 {
		t.Fatalf("ghost quantity cost remained in GP: %d", result.GPAfter)
	}
	if result.MinimumAchievable != 0 {
		t.Fatalf("unexpected minimum: %d", result.MinimumAchievable)
	}
}

func TestSourceHashChangesWhenPaymentSplitChanges(t *testing.T) {
	source := baseCalculation()
	source.Lines[0].PaymentType = "mixed"
	source.Lines[0].CashPaymentAmount = 400
	source.Lines[0].TransferPaymentAmount = 600
	before := sourceHash(source.Lines)
	source.Lines[0].CashPaymentAmount = 500
	source.Lines[0].TransferPaymentAmount = 500
	if after := sourceHash(source.Lines); after == before {
		t.Fatal("payment split was not included in source hash")
	}
}

func assertCents(t *testing.T, input string, want int64) {
	t.Helper()
	got, err := parseDecimalCents(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %d want %d", got, want)
	}
}
