package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed seeddata/ocha_catalog.json
var ochaCatalogJSON []byte

//go:embed seeddata/ocha_images/*
var ochaImageAssets embed.FS

type OchaCatalogManifest struct {
	SchemaVersion         int                      `json:"schema_version"`
	GeneratedAt           string                   `json:"generated_at"`
	RandomSeed            string                   `json:"random_seed"`
	SourceFiles           []OchaSeedSourceFile     `json:"source_files"`
	Summary               OchaCatalogSummary       `json:"summary"`
	Branches              []OchaSeedBranch         `json:"branches"`
	ProductCategories     []OchaSeedCategory       `json:"product_categories"`
	Products              []OchaSeedProduct        `json:"products"`
	BranchProductPrices   []OchaSeedBranchPrice    `json:"branch_product_prices"`
	BranchProductSettings []OchaSeedBranchSettings `json:"branch_product_settings"`
	Inventory             []OchaSeedInventory      `json:"inventory"`
	InventoryLots         []OchaSeedInventoryLot   `json:"inventory_lots"`
	ProductImages         []OchaSeedProductImage   `json:"product_images"`
	ProductNameAliases    []OchaSeedProductAlias   `json:"product_name_aliases"`
	SourceRecords         []json.RawMessage        `json:"source_records"`
	MergeCandidates       []json.RawMessage        `json:"merge_candidates"`
	MergeResolutions      []json.RawMessage        `json:"merge_resolutions"`
}

type OchaSeedSourceFile struct {
	BranchCode string `json:"branch_code"`
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	RowCount   int    `json:"row_count"`
}

type OchaCatalogSummary struct {
	SourceRecords            int `json:"source_records"`
	Branches                 int `json:"branches"`
	ProductCategories        int `json:"product_categories"`
	Products                 int `json:"products"`
	BranchProductMemberships int `json:"branch_product_memberships"`
	BranchProductPrices      int `json:"branch_product_prices"`
	InventoryLots            int `json:"inventory_lots"`
	MultiPriceSourceRecords  int `json:"multi_price_source_records"`
	UncategorizedSourceRows  int `json:"uncategorized_source_records"`
	ProductImages            int `json:"product_images"`
	ProductsWithImages       int `json:"products_with_images"`
	ProductsWithPlaceholder  int `json:"products_with_placeholder"`
	ProductNameAliases       int `json:"product_name_aliases"`
}

type OchaSeedBranch struct {
	ID             string  `json:"id"`
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Address        string  `json:"address"`
	BranchType     string  `json:"branch_type"`
	ParentBranchID *string `json:"parent_branch_id"`
	Active         bool    `json:"active"`
	SalesEnabled   bool    `json:"sales_enabled"`
}

type OchaSeedCategory struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Active bool   `json:"active"`
}

type OchaSeedProduct struct {
	ID                     string  `json:"id"`
	SKU                    string  `json:"sku"`
	Barcode                string  `json:"barcode"`
	CategoryID             *string `json:"category_id"`
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	CostPrice              float64 `json:"cost_price"`
	BaseSellingPrice       float64 `json:"base_selling_price"`
	MaxDiscountAmount      float64 `json:"max_discount_amount"`
	LowStockRealThreshold  int     `json:"low_stock_real_threshold"`
	LowStockGhostThreshold int     `json:"low_stock_ghost_threshold"`
	TracksExpiry           bool    `json:"tracks_expiry"`
	ExpiryWarningDays      int     `json:"expiry_warning_days"`
	UnitName               string  `json:"unit_name"`
	TaxExempt              bool    `json:"tax_exempt"`
	Active                 bool    `json:"active"`
}

type OchaSeedBranchPrice struct {
	ID           string  `json:"id"`
	BranchID     string  `json:"branch_id"`
	ProductID    string  `json:"product_id"`
	SellingPrice float64 `json:"selling_price"`
}

type OchaSeedBranchSettings struct {
	ID                     string  `json:"id"`
	BranchID               string  `json:"branch_id"`
	ProductID              string  `json:"product_id"`
	MaxDiscountAmount      float64 `json:"max_discount_amount"`
	LowStockRealThreshold  int     `json:"low_stock_real_threshold"`
	LowStockGhostThreshold int     `json:"low_stock_ghost_threshold"`
}

type OchaSeedInventory struct {
	ID        string `json:"id"`
	BranchID  string `json:"branch_id"`
	ProductID string `json:"product_id"`
	QtyReal   int    `json:"qty_real"`
	QtyGhost  int    `json:"qty_ghost"`
}

type OchaSeedInventoryLot struct {
	ID                string  `json:"id"`
	BranchID          string  `json:"branch_id"`
	ProductID         string  `json:"product_id"`
	StockBucket       string  `json:"stock_bucket"`
	LotNumber         string  `json:"lot_number"`
	ExpiresOn         string  `json:"expires_on"`
	ReceivedQuantity  int     `json:"received_quantity"`
	RemainingQuantity int     `json:"remaining_quantity"`
	UnitCost          float64 `json:"unit_cost"`
	SourceType        string  `json:"source_type"`
	SourceID          *string `json:"source_id"`
	SourceItemID      *string `json:"source_item_id"`
	OriginLotID       *string `json:"origin_lot_id"`
	ReceivedAt        string  `json:"received_at"`
}

type OchaSeedProductImage struct {
	ID               string `json:"id"`
	ProductID        string `json:"product_id"`
	StorageKey       string `json:"storage_key"`
	AssetName        string `json:"asset_name"`
	MimeType         string `json:"mime_type"`
	SHA256           string `json:"sha256"`
	SourceURL        string `json:"source_url"`
	SourceBranchCode string `json:"source_branch_code"`
	SourceRowIndex   int    `json:"source_row_index"`
	SourceName       string `json:"source_name"`
	AltText          string `json:"alt_text"`
	IsPrimary        bool   `json:"is_primary"`
	SortOrder        int    `json:"sort_order"`
}

type OchaSeedProductAlias struct {
	ID               string `json:"id"`
	ProductID        string `json:"product_id"`
	AliasName        string `json:"alias_name"`
	NormalizedAlias  string `json:"normalized_alias"`
	SourceBranchCode string `json:"source_branch_code"`
	SourceRowIndex   int    `json:"source_row_index"`
}

func LoadOchaCatalog() (OchaCatalogManifest, error) {
	var manifest OchaCatalogManifest
	if err := json.Unmarshal(ochaCatalogJSON, &manifest); err != nil {
		return manifest, fmt.Errorf("decode embedded Ocha catalog: %w", err)
	}
	if err := ValidateOchaCatalog(manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func ValidateOchaCatalog(manifest OchaCatalogManifest) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported Ocha catalog schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.SourceFiles) != 8 {
		return fmt.Errorf("expected 8 Ocha source files, got %d", len(manifest.SourceFiles))
	}
	for _, source := range manifest.SourceFiles {
		if strings.TrimSpace(source.BranchCode) == "" || (source.Kind != "products" && source.Kind != "categories") || len(source.SHA256) != 64 || source.RowCount < 1 {
			return fmt.Errorf("invalid Ocha source file metadata for %s", source.Path)
		}
	}
	if len(manifest.SourceRecords) != 1360 || len(manifest.Branches) != 5 || len(manifest.ProductCategories) != 12 || len(manifest.Products) != manifest.Summary.Products || len(manifest.Inventory) != manifest.Summary.BranchProductMemberships {
		return fmt.Errorf(
			"unexpected Ocha catalog counts: source=%d branches=%d categories=%d products=%d inventory=%d",
			len(manifest.SourceRecords), len(manifest.Branches), len(manifest.ProductCategories), len(manifest.Products), len(manifest.Inventory),
		)
	}
	branchIDs := map[string]bool{}
	branchCodes := map[string]bool{}
	mainWarehouses := 0
	for _, branch := range manifest.Branches {
		if strings.TrimSpace(branch.ID) == "" || strings.TrimSpace(branch.Code) == "" || branchIDs[branch.ID] || branchCodes[branch.Code] {
			return fmt.Errorf("invalid or duplicate Ocha branch %q", branch.Code)
		}
		branchIDs[branch.ID] = true
		branchCodes[branch.Code] = true
		if branch.BranchType == "main_warehouse" {
			mainWarehouses++
			if branch.ParentBranchID != nil || branch.SalesEnabled {
				return fmt.Errorf("main warehouse %s cannot have a parent", branch.Code)
			}
		} else if branch.BranchType != "branch" || branch.ParentBranchID == nil || !branch.SalesEnabled {
			return fmt.Errorf("invalid hierarchy for branch %s", branch.Code)
		}
	}
	if mainWarehouses != 1 || !branchCodes["WH"] || !branchCodes["MES"] {
		return fmt.Errorf("Ocha catalog must contain WH as the only main warehouse and MES as a retail branch")
	}
	categoryIDs := map[string]bool{}
	for _, category := range manifest.ProductCategories {
		if strings.TrimSpace(category.Name) == "" || categoryIDs[category.ID] {
			return fmt.Errorf("invalid or duplicate Ocha category %q", category.Name)
		}
		categoryIDs[category.ID] = true
	}
	productIDs := map[string]bool{}
	skus := map[string]bool{}
	barcodes := map[string]bool{}
	for _, product := range manifest.Products {
		if productIDs[product.ID] || skus[product.SKU] || barcodes[product.Barcode] {
			return fmt.Errorf("duplicate Ocha product identity for %s", product.SKU)
		}
		if !strings.HasPrefix(product.SKU, "OCH-") || len(product.Barcode) != 13 || !product.TracksExpiry {
			return fmt.Errorf("invalid Ocha product %s", product.SKU)
		}
		if product.CategoryID != nil && !categoryIDs[*product.CategoryID] {
			return fmt.Errorf("unknown category for Ocha product %s", product.SKU)
		}
		productIDs[product.ID] = true
		skus[product.SKU] = true
		barcodes[product.Barcode] = true
	}
	memberships := map[string]OchaSeedInventory{}
	for _, item := range manifest.Inventory {
		key := item.BranchID + ":" + item.ProductID
		if !branchIDs[item.BranchID] || !productIDs[item.ProductID] || item.QtyReal < 0 || item.QtyGhost < 0 || (item.QtyReal == item.QtyGhost && item.QtyReal != 0) {
			return fmt.Errorf("invalid Ocha inventory membership %s", key)
		}
		if _, exists := memberships[key]; exists {
			return fmt.Errorf("duplicate Ocha inventory membership %s", key)
		}
		memberships[key] = item
	}
	primaryImages := map[string]int{}
	imageIDs := map[string]bool{}
	for _, image := range manifest.ProductImages {
		if imageIDs[image.ID] || !productIDs[image.ProductID] || strings.TrimSpace(image.StorageKey) == "" || strings.TrimSpace(image.AssetName) == "" || len(image.SHA256) != 64 {
			return fmt.Errorf("invalid Ocha product image %s", image.ID)
		}
		imageIDs[image.ID] = true
		if image.IsPrimary {
			primaryImages[image.ProductID]++
		}
	}
	for productID, count := range primaryImages {
		if count != 1 {
			return fmt.Errorf("product %s has %d primary images", productID, count)
		}
	}
	aliasIDs := map[string]bool{}
	for _, alias := range manifest.ProductNameAliases {
		if aliasIDs[alias.ID] || !productIDs[alias.ProductID] || strings.TrimSpace(alias.AliasName) == "" || strings.TrimSpace(alias.NormalizedAlias) == "" {
			return fmt.Errorf("invalid Ocha product source alias %s", alias.ID)
		}
		aliasIDs[alias.ID] = true
	}
	lotTotals := map[string]int{}
	for _, lot := range manifest.InventoryLots {
		if !branchIDs[lot.BranchID] || !productIDs[lot.ProductID] || (lot.StockBucket != "real" && lot.StockBucket != "ghost") || strings.TrimSpace(lot.ExpiresOn) == "" || lot.RemainingQuantity < 0 || lot.RemainingQuantity > lot.ReceivedQuantity {
			return fmt.Errorf("invalid Ocha inventory lot %s", lot.ID)
		}
		lotTotals[lot.BranchID+":"+lot.ProductID+":"+lot.StockBucket] += lot.RemainingQuantity
	}
	for _, item := range manifest.Inventory {
		prefix := item.BranchID + ":" + item.ProductID + ":"
		if lotTotals[prefix+"real"] != item.QtyReal || lotTotals[prefix+"ghost"] != item.QtyGhost {
			return fmt.Errorf("Ocha lot totals do not match inventory for %s", prefix)
		}
	}
	return nil
}
