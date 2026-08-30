package reportbuilder

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogExcludesSecretFields(t *testing.T) {
	for _, dataset := range Catalog().Datasets {
		for _, field := range dataset.Fields {
			key := strings.ToLower(field.Key)
			for _, forbidden := range []string{"password", "credential", "raw_payload", "jwt", "before_data", "after_data", "user_agent", "source_ip"} {
				if strings.Contains(key, forbidden) {
					t.Fatalf("catalog exposed forbidden field %q in dataset %q", field.Key, dataset.Key)
				}
			}
		}
	}
}

func TestCatalogIncludesSCMDatasetsAndSupplierExpiryFields(t *testing.T) {
	datasets := map[string]Dataset{}
	for _, dataset := range Catalog().Datasets {
		datasets[dataset.Key] = dataset
	}
	for _, key := range []string{"suppliers", "purchase_order_items", "inventory_lots"} {
		if _, ok := datasets[key]; !ok {
			t.Fatalf("missing SCM dataset %q", key)
		}
	}
	for datasetKey, fields := range map[string][]string{
		"purchase_order_items":    {"supplier_name", "purchased_at", "received_quantity", "remaining_quantity", "lot_number", "expires_on", "expiry_status"},
		"inventory_lots":          {"supplier_name", "po_number", "remaining_quantity", "expires_on", "expiry_status"},
		"branch_product_overview": {"sales_enabled", "base_selling_price", "branch_price_override", "branch_selling_price", "selling_price_source", "purchase_quantity", "purchase_amount", "latest_supplier_name", "nearest_expiry_on", "expiring_quantity", "expired_quantity"},
		"sales":                   {"cost_snapshot", "lot_number", "lot_received_at", "lot_expires_on"},
	} {
		available := map[string]bool{}
		for _, field := range datasets[datasetKey].Fields {
			available[field.Key] = true
		}
		for _, field := range fields {
			if !available[field] {
				t.Fatalf("dataset %q missing field %q", datasetKey, field)
			}
		}
	}
	for _, cte := range []string{"purchases AS (", "latest_supplier AS (", "lot_totals AS ("} {
		if !strings.Contains(branchProductOverviewSource, cte) {
			t.Fatalf("overview source must pre-aggregate SCM metrics in %q", cte)
		}
	}
}

func TestCompileDefinitionParameterizesFilterValues(t *testing.T) {
	definition := defaultDefinition("branch_product_overview")
	injection := `ถุงมือ%' OR TRUE --`
	definition.Filters.Rules = []FilterRule{{Field: "product_name", Operator: "contains", Value: injection}}
	compiled, err := compileDefinition(definition, 1, false, time.Now())
	if err != nil {
		t.Fatalf("compile definition: %v", err)
	}
	if strings.Contains(compiled.selectSQL, injection) {
		t.Fatalf("filter value leaked into SQL: %s", compiled.selectSQL)
	}
	if len(compiled.args) == 0 || !strings.Contains(compiled.args[0].(string), `ถุงมือ`) || !strings.Contains(compiled.args[0].(string), `OR TRUE --`) {
		t.Fatalf("expected injection text to be held only in args: %#v", compiled.args)
	}
	if !strings.Contains(compiled.selectSQL, "$1::text") {
		t.Fatalf("expected parameterized SQL, got %s", compiled.selectSQL)
	}
}

func TestCompileGroupedAggregateUsesPreAggregatedOverview(t *testing.T) {
	definition := defaultDefinition("branch_product_overview")
	definition.Columns = []ColumnDefinition{
		{Field: "branch_name", Group: true},
		{Field: "sales_revenue", Aggregate: "sum", Format: "currency"},
		{Field: "movement_in", Aggregate: "sum"},
	}
	definition.Sorts = []SortDefinition{{Field: "sales_revenue", Aggregate: "sum", Direction: "desc"}}
	compiled, err := compileDefinition(definition, 1, false, time.Now())
	if err != nil {
		t.Fatalf("compile grouped definition: %v", err)
	}
	for _, expected := range []string{"WITH sales AS", "WITH sales AS", "GROUP BY r.branch_name", "SUM(r.sales_revenue)", "ORDER BY SUM(r.sales_revenue) DESC"} {
		if !strings.Contains(compiled.selectSQL, expected) {
			t.Fatalf("expected %q in query: %s", expected, compiled.selectSQL)
		}
	}
	if !strings.Contains(branchProductOverviewSource, "GROUP BY i.branch_id, ii.product_id") || !strings.Contains(branchProductOverviewSource, "GROUP BY branch_id, product_id") {
		t.Fatal("overview metrics must be aggregated before joining")
	}
}

func TestValidateDefinitionLimitsAndAllowlist(t *testing.T) {
	definition := defaultDefinition("branch_product_overview")
	definition.Columns = []ColumnDefinition{{Field: "password_hash"}}
	if _, err := ValidateDefinition(definition); err == nil {
		t.Fatal("expected unknown secret field to be rejected")
	}

	definition = defaultDefinition("branch_product_overview")
	definition.Sorts = make([]SortDefinition, maxSorts+1)
	if _, err := ValidateDefinition(definition); err == nil {
		t.Fatal("expected excess sorts to be rejected")
	}

	definition = defaultDefinition("branch_product_overview")
	for index := 0; index < maxFilters+1; index++ {
		definition.Filters.Rules = append(definition.Filters.Rules, FilterRule{Field: "sku", Operator: "eq", Value: "A"})
	}
	if _, err := ValidateDefinition(definition); err == nil {
		t.Fatal("expected excess filters to be rejected")
	}

	definition = defaultDefinition("branch_product_overview")
	definition.Layout = &LayoutConfig{FieldSidebarWidth: minSidebarWidth - 1}
	if _, err := ValidateDefinition(definition); err == nil {
		t.Fatal("expected undersized field sidebar width to be rejected")
	}

	definition.Layout.FieldSidebarWidth = maxSidebarWidth + 1
	if _, err := ValidateDefinition(definition); err == nil {
		t.Fatal("expected oversized field sidebar width to be rejected")
	}

	definition.Layout.FieldSidebarWidth = 288
	if _, err := ValidateDefinition(definition); err != nil {
		t.Fatalf("expected valid field sidebar width: %v", err)
	}

	definition.Layout = nil
	if _, err := ValidateDefinition(definition); err != nil {
		t.Fatalf("expected legacy definition without layout to remain valid: %v", err)
	}
}

func TestTimelineBucketThresholds(t *testing.T) {
	start := time.Now()
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{24 * time.Hour, "hour"},
		{30 * 24 * time.Hour, "day"},
		{365 * 24 * time.Hour, "week"},
		{3 * 365 * 24 * time.Hour, "month"},
	}
	for _, test := range tests {
		end := start.Add(test.duration)
		if got := timelineBucket("auto", &start, &end); got != test.want {
			t.Fatalf("duration %s: want %s, got %s", test.duration, test.want, got)
		}
	}
}
