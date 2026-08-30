export type ReportDataType = "text" | "number" | "integer" | "boolean" | "date" | "datetime" | "uuid";

export type ReportField = {
  key: string;
  label: string;
  group: string;
  type: ReportDataType;
  relation_key?: string;
  relation_depth: number;
  filterable: boolean;
  sortable: boolean;
  groupable: boolean;
  aggregates: string[];
  formats: string[];
};

export type ReportRelation = {
  key: string;
  label: string;
  description: string;
  cardinality: string;
  depth: number;
};

export type ReportDataset = {
  key: string;
  label: string;
  description: string;
  grain: string;
  default_columns: string[];
  default_time_field?: string;
  fields: ReportField[];
  relations: ReportRelation[];
};

export type ReportChoice = { key: string; label: string };
export type ReportOperator = ReportChoice & { value_count: number; types: ReportDataType[] };

export type ReportCatalog = {
  definition_version: number;
  datasets: ReportDataset[];
  operators: ReportOperator[];
  aggregates: ReportChoice[];
  formats: ReportChoice[];
  limits: Record<string, number>;
  page_sizes: number[];
  timeline_buckets: ReportChoice[];
  filter_logic: ReportChoice[];
};

export type ReportColumnDefinition = {
  field: string;
  label?: string;
  format?: string;
  aggregate?: string;
  group?: boolean;
};

export type ReportFilterRule = {
  id?: string;
  field: string;
  operator: string;
  value?: unknown;
  values?: unknown[];
};

export type ReportFilterGroup = {
  id?: string;
  logic: "and" | "or";
  rules: ReportFilterRule[];
  groups: ReportFilterGroup[];
};

export type ReportSort = {
  field: string;
  direction: "asc" | "desc";
  aggregate?: string;
};

export type ReportTimeConfig = {
  field?: string;
  range_type?: "all" | "relative" | "custom";
  relative?: "today" | "last_7_days" | "this_week" | "this_month" | "last_30_days";
  start?: string;
  end?: string;
  bucket?: "auto" | "hour" | "day" | "week" | "month";
};

export type ReportDefinition = {
  version: number;
  dataset_key: string;
  columns: ReportColumnDefinition[];
  filters: ReportFilterGroup;
  sorts: ReportSort[];
  time_config?: ReportTimeConfig;
  layout?: {
    field_sidebar_width: number;
  };
  page_size: number;
};

export type SavedReport = {
  id: string;
  owner_user_id: string;
  name: string;
  description: string;
  definition_version: number;
  definition: ReportDefinition;
  is_pinned: boolean;
  pin_order?: number;
  /** NavigationItem key of the page this renders on when pinned (A6). Defaults to "generate_report". */
  pin_target_key: string;
  created_at: string;
  updated_at: string;
};

export type ReportResultColumn = {
  key: string;
  field: string;
  label: string;
  type: ReportDataType;
  format: string;
  aggregate?: string;
  group?: boolean;
};

export type ReportTimelinePoint = { bucket: string; count: number };

export type ReportResult = {
  columns: ReportResultColumn[];
  rows: Array<Record<string, unknown>>;
  pagination: { page: number; page_size: number; total: number; total_pages: number };
  timeline: ReportTimelinePoint[];
  meta: {
    dataset_key: string;
    duration_ms: number;
    truncated: boolean;
    time_bucket?: string;
    read_only: boolean;
    result_grain: string;
  };
};
