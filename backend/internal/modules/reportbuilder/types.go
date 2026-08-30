package reportbuilder

import "time"

const DefinitionVersion = 1

type Field struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	Group         string   `json:"group"`
	Type          string   `json:"type"`
	RelationKey   string   `json:"relation_key,omitempty"`
	RelationDepth int      `json:"relation_depth"`
	Filterable    bool     `json:"filterable"`
	Sortable      bool     `json:"sortable"`
	Groupable     bool     `json:"groupable"`
	Aggregates    []string `json:"aggregates"`
	Formats       []string `json:"formats"`
	Expression    string   `json:"-"`
}

type Relation struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Cardinality string `json:"cardinality"`
	Depth       int    `json:"depth"`
}

type Dataset struct {
	Key              string     `json:"key"`
	Label            string     `json:"label"`
	Description      string     `json:"description"`
	Grain            string     `json:"grain"`
	DefaultColumns   []string   `json:"default_columns"`
	DefaultTimeField string     `json:"default_time_field,omitempty"`
	Fields           []Field    `json:"fields"`
	Relations        []Relation `json:"relations"`
	Source           string     `json:"-"`
}

type Operator struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	ValueCount int      `json:"value_count"`
	Types      []string `json:"types"`
}

type CatalogResponse struct {
	DefinitionVersion int             `json:"definition_version"`
	Datasets          []Dataset       `json:"datasets"`
	Operators         []Operator      `json:"operators"`
	Aggregates        []CatalogChoice `json:"aggregates"`
	Formats           []CatalogChoice `json:"formats"`
	Limits            map[string]int  `json:"limits"`
	PageSizes         []int           `json:"page_sizes"`
	TimelineBuckets   []CatalogChoice `json:"timeline_buckets"`
	FilterLogic       []CatalogChoice `json:"filter_logic"`
}

type CatalogChoice struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type ColumnDefinition struct {
	Field     string `json:"field"`
	Label     string `json:"label,omitempty"`
	Format    string `json:"format,omitempty"`
	Aggregate string `json:"aggregate,omitempty"`
	Group     bool   `json:"group,omitempty"`
}

type FilterRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	Values   []any  `json:"values,omitempty"`
}

type FilterGroup struct {
	Logic  string        `json:"logic"`
	Rules  []FilterRule  `json:"rules"`
	Groups []FilterGroup `json:"groups"`
}

type SortDefinition struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
	Aggregate string `json:"aggregate,omitempty"`
}

type TimeConfig struct {
	Field     string `json:"field,omitempty"`
	RangeType string `json:"range_type,omitempty"`
	Relative  string `json:"relative,omitempty"`
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
}

type LayoutConfig struct {
	FieldSidebarWidth int `json:"field_sidebar_width"`
}

type Definition struct {
	Version    int                `json:"version"`
	DatasetKey string             `json:"dataset_key"`
	Columns    []ColumnDefinition `json:"columns"`
	Filters    FilterGroup        `json:"filters"`
	Sorts      []SortDefinition   `json:"sorts"`
	TimeConfig *TimeConfig        `json:"time_config,omitempty"`
	Layout     *LayoutConfig      `json:"layout,omitempty"`
	PageSize   int                `json:"page_size"`
}

type ExecuteRequest struct {
	Definition *Definition `json:"definition,omitempty"`
	ReportID   string      `json:"report_id,omitempty"`
	Page       int         `json:"page,omitempty"`
	Preview    bool        `json:"preview,omitempty"`
}

type ResultColumn struct {
	Key       string `json:"key"`
	Field     string `json:"field"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Format    string `json:"format"`
	Aggregate string `json:"aggregate,omitempty"`
	Group     bool   `json:"group,omitempty"`
}

type TimelinePoint struct {
	Bucket time.Time `json:"bucket"`
	Count  int64     `json:"count"`
}

type QueryResult struct {
	Columns    []ResultColumn   `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	Pagination Pagination       `json:"pagination"`
	Timeline   []TimelinePoint  `json:"timeline"`
	Meta       QueryMeta        `json:"meta"`
}

type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

type QueryMeta struct {
	DatasetKey  string `json:"dataset_key"`
	DurationMS  int64  `json:"duration_ms"`
	Truncated   bool   `json:"truncated"`
	TimeBucket  string `json:"time_bucket,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	ResultGrain string `json:"result_grain"`
}

type SavedReport struct {
	ID                string     `json:"id"`
	OwnerUserID       string     `json:"owner_user_id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	DefinitionVersion int        `json:"definition_version"`
	Definition        Definition `json:"definition"`
	IsPinned          bool       `json:"is_pinned"`
	PinOrder          *int       `json:"pin_order"`
	// PinTargetKey is the NavigationItem key of the page this report
	// renders on when pinned (A6). Defaults to "generate_report" — pinned
	// back onto this same page, matching the pin system's original behavior.
	PinTargetKey string    `json:"pin_target_key"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type SaveReportRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
}

type PinRequest struct {
	Pinned bool `json:"pinned"`
	// PinTargetKey selects which page (NavigationItem key) the report pins
	// to; required when Pinned is true. Ignored when unpinning.
	PinTargetKey string `json:"pin_target_key,omitempty"`
}

type ReorderPinsRequest struct {
	ReportIDs []string `json:"report_ids"`
	// PinTargetKey scopes the reorder to that page's pin stack — each pin
	// target keeps its own independent order.
	PinTargetKey string `json:"pin_target_key,omitempty"`
}
