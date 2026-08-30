package sales

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

func TestSalesAndQuotationsRejectGhostForEveryRole(t *testing.T) {
	service := &Service{}
	for _, test := range []struct {
		role string
		code int
	}{
		{role: "central_admin", code: http.StatusForbidden},
		{role: "branch_admin", code: http.StatusForbidden},
		{role: "branch_pos", code: http.StatusForbidden},
		{role: "super_admin", code: http.StatusBadRequest},
	} {
		_, _, _, _, err := service.priceLines(context.Background(), nil, platform.AuthUser{RoleKey: test.role}, "branch", false, []LineInput{{ProductID: "product", StockBucket: "ghost", Quantity: 1}}, 0, false)
		appErr, ok := err.(*platform.AppError)
		if !ok || appErr.Code != test.code {
			t.Fatalf("role %s: expected AppError %d, got %#v", test.role, test.code, err)
		}
	}
}

func TestResolveLineDisplayAndPriceGovernmentMode(t *testing.T) {
	displayName, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
		"เตียงผู้ป่วย",
		5400,
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

func TestResolveLineDisplayAndPriceUsesSingleSellingPrice(t *testing.T) {
	// There is one sale price (base, optionally branch-overridden upstream in
	// SQL) — no retail tier exists anymore (business-flow.md pricing rule).
	_, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
		"สินค้า", 100,
		sql.NullString{}, sql.NullString{}, sql.NullFloat64{},
		false, nil, false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unitPrice != 100 || priceSource != "branch_price" {
		t.Fatalf("unexpected single-price resolution: %.2f %s", unitPrice, priceSource)
	}
}

func TestResolveLineDisplayAndPriceOverrideBeatsBase(t *testing.T) {
	override := 99.0
	_, unitPrice, priceSource, err := resolveLineDisplayAndPrice(
		"สินค้า", 100,
		sql.NullString{}, sql.NullString{}, sql.NullFloat64{},
		false, &override, true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unitPrice != 99 || priceSource != "override" {
		t.Fatalf("expected override to win: %.2f %s", unitPrice, priceSource)
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

func TestValidatePOSCatalogRulesRejectsGovernmentSalesAndAliases(t *testing.T) {
	user := platform.AuthUser{RoleKey: "branch_pos", Portal: "pos"}
	if err := validatePOSCatalogRules(user, true, []LineInput{{ProductID: "product"}}); err == nil {
		t.Fatal("expected government mode to be rejected for POS")
	}
	aliasID := "alias-1"
	if err := validatePOSCatalogRules(user, false, []LineInput{{ProductID: "product", AliasID: &aliasID}}); err == nil {
		t.Fatal("expected government alias to be rejected for POS")
	}
	if err := validatePOSCatalogRules(user, false, []LineInput{{ProductID: "product"}}); err != nil {
		t.Fatalf("expected standard POS sale to pass: %v", err)
	}
	if err := validatePOSCatalogRules(platform.AuthUser{RoleKey: "super_admin"}, true, []LineInput{{AliasID: &aliasID}}); err != nil {
		t.Fatalf("expected admin government sale to pass: %v", err)
	}
}

func TestFormatSalesDocNumber(t *testing.T) {
	issuedAt := time.Date(2026, 4, 15, 1, 30, 0, 0, time.UTC)
	value := platform.FormatSalesDocNumber("bl", issuedAt, 42)
	if value != "BL2026041500042" {
		t.Fatalf("unexpected formatted number: %s", value)
	}
	branchValue := platform.FormatBranchDocumentNumber("mes", "bl", issuedAt, 42)
	if branchValue != "MES-BL2026041500042" {
		t.Fatalf("unexpected branch formatted number: %s", branchValue)
	}
}

func TestValidatePaymentCollector(t *testing.T) {
	if err := validatePaymentCollector(platform.AuthUser{RoleKey: "branch_pos", Permissions: []string{"payment.collect"}}); err != nil {
		t.Fatalf("expected branch POS to collect payment: %v", err)
	}
	if err := validatePaymentCollector(platform.AuthUser{RoleKey: "super_admin", Permissions: []string{"payment.collect"}}); err != nil {
		t.Fatalf("expected super admin direct payment collection to pass: %v", err)
	}
}

func TestValidateCheckoutPaymentCash(t *testing.T) {
	got, err := validateCheckoutPayment(CheckoutRequest{
		PaymentType: "cash", TenderedAmount: 120,
	}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashAmount != 100 || got.TransferAmount != 0 || got.ChangeAmount != 20 {
		t.Fatalf("unexpected settlement %#v", got)
	}
}

func TestValidateCheckoutPaymentTransferDoesNotRequireReference(t *testing.T) {
	got, err := validateCheckoutPayment(CheckoutRequest{PaymentType: "bank_transfer"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashAmount != 0 || got.TransferAmount != 100 || got.ChangeAmount != 0 {
		t.Fatalf("unexpected settlement %#v", got)
	}
}

func TestValidateCheckoutPaymentMixed(t *testing.T) {
	got, err := validateCheckoutPayment(CheckoutRequest{
		PaymentType: "mixed", TransferAmount: 60, TenderedAmount: 50,
	}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashAmount != 40 || got.TransferAmount != 60 || got.ChangeAmount != 10 {
		t.Fatalf("unexpected settlement %#v", got)
	}
	rows := settlementPaymentRows(got)
	if len(rows) != 2 ||
		rows[0].PaymentType != "cash" || rows[0].Amount != 40 ||
		rows[1].PaymentType != "bank_transfer" || rows[1].Amount != 60 ||
		rows[1].ReferenceCode != "" {
		t.Fatalf("unexpected payment rows %#v", rows)
	}
}

func TestValidateCheckoutPaymentMixedCalculatesChangeFromBothInputs(t *testing.T) {
	got, err := validateCheckoutPayment(CheckoutRequest{
		PaymentType: "mixed", TransferAmount: 17586.65, TenderedAmount: 16793.65,
	}, 33587.30)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashAmount != 16000.65 || got.TransferAmount != 17586.65 || got.TenderedAmount != 16793.65 || got.ChangeAmount != 793 {
		t.Fatalf("unexpected settlement %#v", got)
	}
	rows := settlementPaymentRows(got)
	if len(rows) != 2 || checkoutCentsToMoney(moneyRowsTotalCents(rows)) != 33587.30 {
		t.Fatalf("payment rows do not balance to invoice total: %#v", rows)
	}
}

func TestValidateCheckoutPaymentMixedKeepsTransferAndReturnsCashChange(t *testing.T) {
	got, err := validateCheckoutPayment(CheckoutRequest{
		PaymentType: "mixed", TransferAmount: 2000, TenderedAmount: 500,
	}, 2400)
	if err != nil {
		t.Fatal(err)
	}
	if got.CashAmount != 400 || got.TransferAmount != 2000 || got.TenderedAmount != 500 || got.ChangeAmount != 100 {
		t.Fatalf("unexpected settlement %#v", got)
	}
	rows := settlementPaymentRows(got)
	if len(rows) != 2 || checkoutCentsToMoney(moneyRowsTotalCents(rows)) != 2400 {
		t.Fatalf("payment rows do not balance to invoice total: %#v", rows)
	}
}

func moneyRowsTotalCents(rows []checkoutPaymentRow) int64 {
	var total int64
	for _, row := range rows {
		value, valid := checkoutMoneyCents(row.Amount)
		if !valid {
			return -1
		}
		total += value
	}
	return total
}

func TestValidateCheckoutPaymentRejectsNonPositiveTotal(t *testing.T) {
	for _, paymentType := range []string{"cash", "bank_transfer", "mixed"} {
		if _, err := validateCheckoutPayment(CheckoutRequest{PaymentType: paymentType}, 0); err == nil {
			t.Fatalf("expected zero total to fail for %s", paymentType)
		}
	}
}

func TestValidateCheckoutPaymentMixedRejectsInvalidAmounts(t *testing.T) {
	tests := []CheckoutRequest{
		{PaymentType: "mixed", TransferAmount: 0, TenderedAmount: 100, ReferenceCode: "TR"},
		{PaymentType: "mixed", TransferAmount: 100, TenderedAmount: 0, ReferenceCode: "TR"},
		{PaymentType: "mixed", TransferAmount: 60, TenderedAmount: 39.99, ReferenceCode: "TR"},
		{PaymentType: "mixed", TransferAmount: 60.001, TenderedAmount: 40, ReferenceCode: "TR"},
	}
	for _, input := range tests {
		if _, err := validateCheckoutPayment(input, 100); err == nil {
			t.Fatalf("expected invalid input to fail: %#v", input)
		}
	}
}
