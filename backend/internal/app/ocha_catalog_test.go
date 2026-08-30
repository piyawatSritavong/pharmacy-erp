package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLoadOchaCatalogManifest(t *testing.T) {
	manifest, err := LoadOchaCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Summary.SourceRecords != 1360 || manifest.Summary.Products != 694 || manifest.Summary.BranchProductMemberships != 2068 || manifest.Summary.Branches != 5 {
		t.Fatalf("unexpected summary: %#v", manifest.Summary)
	}
	if len(manifest.InventoryLots) != 2829 || len(manifest.MergeCandidates) != 0 || len(manifest.MergeResolutions) == 0 {
		t.Fatalf("expected reviewed resolutions with no unresolved candidates, got lots=%d candidates=%d resolutions=%d", len(manifest.InventoryLots), len(manifest.MergeCandidates), len(manifest.MergeResolutions))
	}
	if len(manifest.ProductImages) != 798 || manifest.Summary.ProductsWithImages != 610 || manifest.Summary.ProductsWithPlaceholder != 84 {
		t.Fatalf("unexpected image summary: %#v", manifest.Summary)
	}
	if manifest.Summary.MultiPriceSourceRecords != 228 || manifest.Summary.UncategorizedSourceRows != 157 {
		t.Fatalf("source provenance counts changed: %#v", manifest.Summary)
	}

	branchRows := map[string]int{}
	for _, raw := range manifest.SourceRecords {
		var source struct {
			BranchCode string `json:"branch_code"`
			Name       string `json:"name"`
			PriceLabel string `json:"price_label"`
			ProductID  string `json:"product_id"`
		}
		if err := json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.PriceLabel) == "" || strings.TrimSpace(source.ProductID) == "" {
			t.Fatalf("incomplete source record: %s", raw)
		}
		branchRows[source.BranchCode]++
	}
	wantRows := map[string]int{"PHH": 431, "PHS": 429, "NPT": 298, "MES": 202}
	if !reflect.DeepEqual(branchRows, wantRows) {
		t.Fatalf("source row counts changed: got %v want %v", branchRows, wantRows)
	}
	warehouseCount := 0
	for _, branch := range manifest.Branches {
		if branch.Code == "WH" && branch.BranchType == "main_warehouse" && !branch.SalesEnabled {
			warehouseCount++
		}
		if branch.Code != "WH" && !branch.SalesEnabled {
			t.Fatalf("retail branch %s must remain sales enabled", branch.Code)
		}
	}
	if warehouseCount != 1 {
		t.Fatalf("expected one non-selling WH branch, got %d", warehouseCount)
	}
}

func TestOchaCatalogIdentifiersAreDeterministicAndValid(t *testing.T) {
	first, err := LoadOchaCatalog()
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOchaCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Products, second.Products) || !reflect.DeepEqual(first.Inventory, second.Inventory) {
		t.Fatal("embedded catalog output is not deterministic")
	}

	salinePoles := 0
	for _, product := range first.Products {
		if !validEAN13(product.Barcode) {
			t.Fatalf("invalid EAN-13 checksum for %s: %s", product.SKU, product.Barcode)
		}
		if strings.HasPrefix(product.Name, "เสาน้ำเกลือสแตนเลส (หมวด") {
			salinePoles++
		}
	}
	if salinePoles != 2 {
		t.Fatalf("expected the conflicting saline poles to remain two distinct products, got %d", salinePoles)
	}
}

func validEAN13(value string) bool {
	if len(value) != 13 {
		return false
	}
	sum := 0
	for index, char := range value[:12] {
		if char < '0' || char > '9' {
			return false
		}
		weight := 1
		if index%2 == 1 {
			weight = 3
		}
		sum += weight * int(char-'0')
	}
	return byte('0'+(10-sum%10)%10) == value[12]
}
