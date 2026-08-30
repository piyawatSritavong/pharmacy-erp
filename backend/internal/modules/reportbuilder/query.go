package reportbuilder

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

const (
	maxColumns       = 50
	maxFilters       = 25
	maxSorts         = 5
	maxRelationDepth = 4
	minSidebarWidth  = 220
	maxSidebarWidth  = 560
	queryTimeout     = 8 * time.Second
)

var bangkok = time.FixedZone("Asia/Bangkok", 7*60*60)

type compiledQuery struct {
	dataset        Dataset
	definition     Definition
	columns        []ResultColumn
	selectSQL      string
	countSQL       string
	timelineSQL    string
	args           []any
	pageArgs       []any
	timelineArgs   []any
	timelineBucket string
}

type queryCompiler struct {
	dataset Dataset
	args    []any
}

func ValidateDefinition(input Definition) (Definition, error) {
	if input.Version == 0 {
		input.Version = DefinitionVersion
	}
	if input.Version != DefinitionVersion {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report definition version is not supported")
	}
	input.DatasetKey = normalizeKey(input.DatasetKey)
	dataset, ok := datasetByKey(input.DatasetKey)
	if !ok {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report dataset is not allowed")
	}
	if len(input.Columns) == 0 {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report columns are required")
	}
	if len(input.Columns) > maxColumns {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report has too many columns")
	}
	if input.PageSize == 0 {
		input.PageSize = 100
	}
	if !allowedPageSize(input.PageSize) {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report page size is not allowed")
	}
	if input.Layout != nil && (input.Layout.FieldSidebarWidth < minSidebarWidth || input.Layout.FieldSidebarWidth > maxSidebarWidth) {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report field sidebar width is not allowed")
	}

	grouped := false
	maxDepth := 0
	selectedGroups := map[string]bool{}
	for index := range input.Columns {
		column := &input.Columns[index]
		column.Field = normalizeKey(column.Field)
		field, exists := fieldByKey(dataset, column.Field)
		if !exists {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report field is not allowed")
		}
		if field.RelationDepth > maxDepth {
			maxDepth = field.RelationDepth
		}
		column.Aggregate = normalizeKey(column.Aggregate)
		column.Format = normalizeKey(column.Format)
		column.Label = strings.TrimSpace(column.Label)
		if len([]rune(column.Label)) > 80 {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report column label is too long")
		}
		if column.Format == "" {
			column.Format = field.Formats[0]
		}
		if !contains(field.Formats, column.Format) {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report column format is not allowed")
		}
		if column.Aggregate != "" {
			if !contains(field.Aggregates, column.Aggregate) {
				return Definition{}, platform.NewError(http.StatusBadRequest, "report aggregate is not allowed")
			}
			if column.Group {
				return Definition{}, platform.NewError(http.StatusBadRequest, "report column cannot be grouped and aggregated together")
			}
			grouped = true
		}
		if column.Group {
			if !field.Groupable {
				return Definition{}, platform.NewError(http.StatusBadRequest, "report field cannot be grouped")
			}
			grouped = true
			selectedGroups[column.Field] = true
		}
	}
	if grouped {
		for _, column := range input.Columns {
			if column.Aggregate == "" && !column.Group {
				return Definition{}, platform.NewError(http.StatusBadRequest, "all non-aggregate report columns must be grouped")
			}
		}
	}

	filterCount, filterDepth, err := validateFilterGroup(dataset, &input.Filters, 0)
	if err != nil {
		return Definition{}, err
	}
	if filterCount > maxFilters {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report has too many filters")
	}
	if filterDepth > maxDepth {
		maxDepth = filterDepth
	}
	if len(input.Sorts) > maxSorts {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report has too many sort fields")
	}
	for index := range input.Sorts {
		sort := &input.Sorts[index]
		sort.Field = normalizeKey(sort.Field)
		sort.Direction = normalizeKey(sort.Direction)
		sort.Aggregate = normalizeKey(sort.Aggregate)
		field, exists := fieldByKey(dataset, sort.Field)
		if !exists || !field.Sortable {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report sort field is not allowed")
		}
		if sort.Direction != "asc" && sort.Direction != "desc" {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report sort direction is not allowed")
		}
		if sort.Aggregate != "" && !contains(field.Aggregates, sort.Aggregate) {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report sort aggregate is not allowed")
		}
		if grouped && sort.Aggregate == "" && !selectedGroups[sort.Field] {
			return Definition{}, platform.NewError(http.StatusBadRequest, "grouped report can only sort grouped or aggregate fields")
		}
		if field.RelationDepth > maxDepth {
			maxDepth = field.RelationDepth
		}
	}
	if input.TimeConfig != nil {
		input.TimeConfig.Field = normalizeKey(input.TimeConfig.Field)
		input.TimeConfig.RangeType = normalizeKey(input.TimeConfig.RangeType)
		input.TimeConfig.Relative = normalizeKey(input.TimeConfig.Relative)
		input.TimeConfig.Bucket = normalizeKey(input.TimeConfig.Bucket)
		if input.TimeConfig.Field != "" {
			field, exists := fieldByKey(dataset, input.TimeConfig.Field)
			if !exists || (field.Type != "date" && field.Type != "datetime") {
				return Definition{}, platform.NewError(http.StatusBadRequest, "report time field is not allowed")
			}
			if field.RelationDepth > maxDepth {
				maxDepth = field.RelationDepth
			}
		}
		if input.TimeConfig.Bucket == "" {
			input.TimeConfig.Bucket = "auto"
		}
		if !contains([]string{"auto", "hour", "day", "week", "month"}, input.TimeConfig.Bucket) {
			return Definition{}, platform.NewError(http.StatusBadRequest, "report timeline bucket is not allowed")
		}
		if _, _, err := resolveTimeRange(*input.TimeConfig, time.Now().In(bangkok)); err != nil {
			return Definition{}, err
		}
	}
	if maxDepth > maxRelationDepth {
		return Definition{}, platform.NewError(http.StatusBadRequest, "report relation depth is too deep")
	}
	return input, nil
}

func validateFilterGroup(dataset Dataset, group *FilterGroup, nesting int) (int, int, error) {
	if group.Rules == nil {
		group.Rules = []FilterRule{}
	}
	if group.Groups == nil {
		group.Groups = []FilterGroup{}
	}
	group.Logic = normalizeKey(group.Logic)
	if group.Logic == "" {
		group.Logic = "and"
	}
	if group.Logic != "and" && group.Logic != "or" {
		return 0, 0, platform.NewError(http.StatusBadRequest, "report filter logic is not allowed")
	}
	if nesting > 8 {
		return 0, 0, platform.NewError(http.StatusBadRequest, "report filter nesting is too deep")
	}
	count := 0
	maxDepth := 0
	for index := range group.Rules {
		rule := &group.Rules[index]
		rule.Field = normalizeKey(rule.Field)
		rule.Operator = normalizeKey(rule.Operator)
		field, ok := fieldByKey(dataset, rule.Field)
		if !ok || !field.Filterable {
			return 0, 0, platform.NewError(http.StatusBadRequest, "report filter field is not allowed")
		}
		if !operatorAllowed(field.Type, rule.Operator) {
			return 0, 0, platform.NewError(http.StatusBadRequest, "report filter operator is not allowed")
		}
		if err := validateRuleValues(field, *rule); err != nil {
			return 0, 0, err
		}
		count++
		if field.RelationDepth > maxDepth {
			maxDepth = field.RelationDepth
		}
	}
	for index := range group.Groups {
		nestedCount, nestedDepth, err := validateFilterGroup(dataset, &group.Groups[index], nesting+1)
		if err != nil {
			return 0, 0, err
		}
		count += nestedCount
		if nestedDepth > maxDepth {
			maxDepth = nestedDepth
		}
	}
	return count, maxDepth, nil
}

func validateRuleValues(field Field, rule FilterRule) error {
	switch rule.Operator {
	case "is_null", "not_null":
		return nil
	case "in":
		if len(rule.Values) == 0 || len(rule.Values) > 100 {
			return platform.NewError(http.StatusBadRequest, "report filter list must contain between 1 and 100 values")
		}
		for _, value := range rule.Values {
			if _, err := normalizeValue(field.Type, value); err != nil {
				return err
			}
		}
		return nil
	case "between":
		if len(rule.Values) != 2 {
			return platform.NewError(http.StatusBadRequest, "report between filter requires two values")
		}
		for _, value := range rule.Values {
			if _, err := normalizeValue(field.Type, value); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := normalizeValue(field.Type, rule.Value)
		return err
	}
}

func operatorAllowed(dataType, operator string) bool {
	common := []string{"eq", "neq", "in", "is_null", "not_null"}
	if contains(common, operator) {
		return true
	}
	if dataType == "text" {
		return operator == "contains" || operator == "starts_with"
	}
	if contains([]string{"number", "integer", "date", "datetime"}, dataType) {
		return contains([]string{"gt", "gte", "lt", "lte", "between"}, operator)
	}
	return false
}

func compileDefinition(input Definition, page int, preview bool, now time.Time) (compiledQuery, error) {
	definition, err := ValidateDefinition(input)
	if err != nil {
		return compiledQuery{}, err
	}
	dataset, _ := datasetByKey(definition.DatasetKey)
	compiler := &queryCompiler{dataset: dataset, args: []any{}}
	whereSQL, err := compiler.compileFilters(definition.Filters)
	if err != nil {
		return compiledQuery{}, err
	}
	whereParts := []string{}
	if whereSQL != "" {
		whereParts = append(whereParts, whereSQL)
	}

	var timeField Field
	hasTimeField := false
	var rangeStart, rangeEnd *time.Time
	if definition.TimeConfig != nil && definition.TimeConfig.Field != "" {
		timeField, hasTimeField = fieldByKey(dataset, definition.TimeConfig.Field)
		rangeStart, rangeEnd, err = resolveTimeRange(*definition.TimeConfig, now.In(bangkok))
		if err != nil {
			return compiledQuery{}, err
		}
		if rangeStart != nil {
			whereParts = append(whereParts, fmt.Sprintf("%s >= %s", timeField.Expression, compiler.addArg(rangeStart.UTC(), timeField.Type)))
		}
		if rangeEnd != nil {
			whereParts = append(whereParts, fmt.Sprintf("%s <= %s", timeField.Expression, compiler.addArg(rangeEnd.UTC(), timeField.Type)))
		}
	}
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	selectParts := make([]string, 0, len(definition.Columns))
	groupParts := []string{}
	resultColumns := make([]ResultColumn, 0, len(definition.Columns))
	grouped := false
	for index, column := range definition.Columns {
		field, _ := fieldByKey(dataset, column.Field)
		expression := field.Expression
		resultType := field.Type
		if column.Aggregate != "" {
			expression = aggregateExpression(column.Aggregate, expression)
			grouped = true
			if column.Aggregate == "count" {
				resultType = "integer"
			} else if column.Aggregate == "avg" || column.Aggregate == "sum" {
				resultType = "number"
			}
		} else if column.Group {
			groupParts = append(groupParts, expression)
			grouped = true
		}
		alias := fmt.Sprintf("c%d", index)
		selectParts = append(selectParts, expression+" AS "+alias)
		label := column.Label
		if label == "" {
			label = field.Label
		}
		resultColumns = append(resultColumns, ResultColumn{Key: alias, Field: field.Key, Label: label, Type: resultType, Format: column.Format, Aggregate: column.Aggregate, Group: column.Group})
	}
	groupClause := ""
	if len(groupParts) > 0 {
		groupClause = " GROUP BY " + strings.Join(groupParts, ", ")
	}
	baseSQL := " FROM " + dataset.Source + whereClause + groupClause

	orderParts := []string{}
	for _, sort := range definition.Sorts {
		field, _ := fieldByKey(dataset, sort.Field)
		expression := field.Expression
		if sort.Aggregate != "" {
			expression = aggregateExpression(sort.Aggregate, expression)
		}
		orderParts = append(orderParts, expression+" "+strings.ToUpper(sort.Direction)+" NULLS LAST")
	}
	if len(orderParts) == 0 && hasTimeField && !grouped {
		orderParts = append(orderParts, timeField.Expression+" DESC NULLS LAST")
	}
	orderClause := ""
	if len(orderParts) > 0 {
		orderClause = " ORDER BY " + strings.Join(orderParts, ", ")
	}

	if page < 1 {
		page = 1
	}
	pageSize := definition.PageSize
	if preview {
		pageSize = 20
	}
	pageArgs := append([]any{}, compiler.args...)
	limitPlaceholder := "$" + strconv.Itoa(len(pageArgs)+1) + "::integer"
	pageArgs = append(pageArgs, pageSize)
	offsetPlaceholder := "$" + strconv.Itoa(len(pageArgs)+1) + "::integer"
	pageArgs = append(pageArgs, (page-1)*pageSize)
	selectSQL := "SELECT " + strings.Join(selectParts, ", ") + baseSQL + orderClause + " LIMIT " + limitPlaceholder + " OFFSET " + offsetPlaceholder
	countBody := "SELECT 1" + baseSQL
	if grouped && len(groupParts) == 0 {
		countBody = "SELECT COUNT(*) FROM " + dataset.Source + whereClause
	}
	countSQL := "SELECT COUNT(*) FROM (" + countBody + ") report_count"

	timelineSQL := ""
	timelineArgs := []any{}
	bucket := ""
	if hasTimeField {
		bucket = timelineBucket(definition.TimeConfig.Bucket, rangeStart, rangeEnd)
		timelineWhere := whereClause
		if timelineWhere == "" {
			timelineWhere = " WHERE " + timeField.Expression + " IS NOT NULL"
		} else {
			timelineWhere += " AND " + timeField.Expression + " IS NOT NULL"
		}
		bucketExpression := fmt.Sprintf("date_trunc('%s', %s AT TIME ZONE 'Asia/Bangkok') AT TIME ZONE 'Asia/Bangkok'", bucket, timeField.Expression)
		if timeField.Type == "date" {
			bucketExpression = fmt.Sprintf("date_trunc('%s', %s::timestamp) AT TIME ZONE 'Asia/Bangkok'", bucket, timeField.Expression)
		}
		timelineSQL = "SELECT " + bucketExpression + " AS bucket, COUNT(*)::bigint AS count FROM " + dataset.Source + timelineWhere + " GROUP BY 1 ORDER BY 1"
		timelineArgs = append(timelineArgs, compiler.args...)
	}
	return compiledQuery{dataset: dataset, definition: definition, columns: resultColumns, selectSQL: selectSQL, countSQL: countSQL, timelineSQL: timelineSQL, args: compiler.args, pageArgs: pageArgs, timelineArgs: timelineArgs, timelineBucket: bucket}, nil
}

func (c *queryCompiler) compileFilters(group FilterGroup) (string, error) {
	parts := []string{}
	for _, rule := range group.Rules {
		part, err := c.compileRule(rule)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	for _, nested := range group.Groups {
		part, err := c.compileFilters(nested)
		if err != nil {
			return "", err
		}
		if part != "" {
			parts = append(parts, "("+part+")")
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	logic := strings.ToUpper(group.Logic)
	if logic != "OR" {
		logic = "AND"
	}
	return strings.Join(parts, " "+logic+" "), nil
}

func (c *queryCompiler) compileRule(rule FilterRule) (string, error) {
	field, _ := fieldByKey(c.dataset, rule.Field)
	expression := field.Expression
	switch rule.Operator {
	case "is_null":
		return expression + " IS NULL", nil
	case "not_null":
		return expression + " IS NOT NULL", nil
	case "contains", "starts_with":
		value := escapeLike(fmt.Sprint(rule.Value))
		if rule.Operator == "contains" {
			value = "%" + value + "%"
		} else {
			value += "%"
		}
		placeholder := c.addArg(value, "text")
		return fmt.Sprintf("LOWER(COALESCE(%s::text, '')) LIKE LOWER(%s) ESCAPE '\\'", expression, placeholder), nil
	case "in":
		placeholders := make([]string, 0, len(rule.Values))
		for _, raw := range rule.Values {
			value, err := normalizeValue(field.Type, raw)
			if err != nil {
				return "", err
			}
			placeholders = append(placeholders, c.addArg(value, field.Type))
		}
		return expression + " IN (" + strings.Join(placeholders, ", ") + ")", nil
	case "between":
		first, err := normalizeValue(field.Type, rule.Values[0])
		if err != nil {
			return "", err
		}
		second, err := normalizeValue(field.Type, rule.Values[1])
		if err != nil {
			return "", err
		}
		return expression + " BETWEEN " + c.addArg(first, field.Type) + " AND " + c.addArg(second, field.Type), nil
	default:
		value, err := normalizeValue(field.Type, rule.Value)
		if err != nil {
			return "", err
		}
		operator := map[string]string{"eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[rule.Operator]
		return expression + " " + operator + " " + c.addArg(value, field.Type), nil
	}
}

func (c *queryCompiler) addArg(value any, dataType string) string {
	c.args = append(c.args, value)
	placeholder := "$" + strconv.Itoa(len(c.args))
	switch dataType {
	case "number":
		return placeholder + "::numeric"
	case "integer":
		return placeholder + "::bigint"
	case "boolean":
		return placeholder + "::boolean"
	case "date":
		return placeholder + "::date"
	case "datetime":
		return placeholder + "::timestamptz"
	default:
		return placeholder + "::text"
	}
}

func normalizeValue(dataType string, raw any) (any, error) {
	if raw == nil {
		return nil, platform.NewError(http.StatusBadRequest, "report filter value is required")
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	if text == "" && dataType != "text" {
		return nil, platform.NewError(http.StatusBadRequest, "report filter value is required")
	}
	switch dataType {
	case "number":
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, platform.NewError(http.StatusBadRequest, "report number filter is invalid")
		}
		return value, nil
	case "integer":
		if value, err := strconv.ParseInt(text, 10, 64); err == nil {
			return value, nil
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || value != math.Trunc(value) {
			return nil, platform.NewError(http.StatusBadRequest, "report integer filter is invalid")
		}
		return int64(value), nil
	case "boolean":
		value, err := strconv.ParseBool(text)
		if err != nil {
			return nil, platform.NewError(http.StatusBadRequest, "report boolean filter is invalid")
		}
		return value, nil
	case "date":
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return nil, platform.NewError(http.StatusBadRequest, "report date filter is invalid")
		}
		return text, nil
	case "datetime":
		value, err := parseReportTime(text)
		if err != nil {
			return nil, platform.NewError(http.StatusBadRequest, "report datetime filter is invalid")
		}
		return value.UTC(), nil
	default:
		return text, nil
	}
}

func resolveTimeRange(config TimeConfig, now time.Time) (*time.Time, *time.Time, error) {
	if config.Field == "" {
		return nil, nil, nil
	}
	switch config.RangeType {
	case "", "all":
		return nil, nil, nil
	case "custom":
		var start, end *time.Time
		if strings.TrimSpace(config.Start) != "" {
			value, err := parseReportTime(config.Start)
			if err != nil {
				return nil, nil, platform.NewError(http.StatusBadRequest, "report start time is invalid")
			}
			start = &value
		}
		if strings.TrimSpace(config.End) != "" {
			value, err := parseReportTime(config.End)
			if err != nil {
				return nil, nil, platform.NewError(http.StatusBadRequest, "report end time is invalid")
			}
			end = &value
		}
		if start != nil && end != nil && start.After(*end) {
			return nil, nil, platform.NewError(http.StatusBadRequest, "report time range is invalid")
		}
		return start, end, nil
	case "relative":
		end := now
		var start time.Time
		switch config.Relative {
		case "today":
			start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, bangkok)
		case "last_7_days":
			start = now.AddDate(0, 0, -7)
		case "this_week":
			days := (int(now.Weekday()) + 6) % 7
			start = time.Date(now.Year(), now.Month(), now.Day()-days, 0, 0, 0, 0, bangkok)
		case "this_month":
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, bangkok)
		case "last_30_days", "":
			start = now.AddDate(0, 0, -30)
		default:
			return nil, nil, platform.NewError(http.StatusBadRequest, "report relative time range is not allowed")
		}
		return &start, &end, nil
	default:
		return nil, nil, platform.NewError(http.StatusBadRequest, "report time range type is not allowed")
	}
}

func parseReportTime(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02"} {
		if layout == "2006-01-02T15:04" || layout == "2006-01-02 15:04:05" || layout == "2006-01-02" {
			if value, err := time.ParseInLocation(layout, raw, bangkok); err == nil {
				return value, nil
			}
			continue
		}
		if value, err := time.Parse(layout, raw); err == nil {
			return value, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}

func timelineBucket(requested string, start, end *time.Time) string {
	if requested != "" && requested != "auto" {
		return requested
	}
	if start == nil || end == nil {
		return "day"
	}
	duration := end.Sub(*start)
	switch {
	case duration <= 48*time.Hour:
		return "hour"
	case duration <= 90*24*time.Hour:
		return "day"
	case duration <= 2*365*24*time.Hour:
		return "week"
	default:
		return "month"
	}
}

func aggregateExpression(aggregate, expression string) string {
	switch aggregate {
	case "count":
		return "COUNT(" + expression + ")::bigint"
	case "sum":
		return "SUM(" + expression + ")"
	case "avg":
		return "AVG(" + expression + ")"
	case "min":
		return "MIN(" + expression + ")"
	case "max":
		return "MAX(" + expression + ")"
	default:
		return expression
	}
}

func allowedPageSize(value int) bool {
	return value == 50 || value == 100 || value == 250 || value == 500
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func executeCompiled(ctx context.Context, db *sql.DB, compiled compiledQuery, page int, preview bool) (QueryResult, error) {
	started := time.Now()
	queryContext, cancel := context.WithTimeout(ctx, queryTimeout+time.Second)
	defer cancel()
	tx, err := db.BeginTx(queryContext, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return QueryResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(queryContext, `SET LOCAL statement_timeout = '8s'`); err != nil {
		return QueryResult{}, err
	}
	if _, err := tx.ExecContext(queryContext, `SET LOCAL TIME ZONE 'Asia/Bangkok'`); err != nil {
		return QueryResult{}, err
	}

	var total int64
	if err := tx.QueryRowContext(queryContext, compiled.countSQL, compiled.args...).Scan(&total); err != nil {
		return QueryResult{}, mapQueryError(err)
	}
	rows, err := tx.QueryContext(queryContext, compiled.selectSQL, compiled.pageArgs...)
	if err != nil {
		return QueryResult{}, mapQueryError(err)
	}
	items, err := scanRows(rows, compiled.columns)
	rows.Close()
	if err != nil {
		return QueryResult{}, err
	}

	timeline := []TimelinePoint{}
	if compiled.timelineSQL != "" {
		timelineRows, queryErr := tx.QueryContext(queryContext, compiled.timelineSQL, compiled.timelineArgs...)
		if queryErr != nil {
			return QueryResult{}, mapQueryError(queryErr)
		}
		for timelineRows.Next() {
			var point TimelinePoint
			if err := timelineRows.Scan(&point.Bucket, &point.Count); err != nil {
				timelineRows.Close()
				return QueryResult{}, err
			}
			timeline = append(timeline, point)
		}
		if err := timelineRows.Err(); err != nil {
			timelineRows.Close()
			return QueryResult{}, err
		}
		timelineRows.Close()
	}
	if err := tx.Commit(); err != nil {
		return QueryResult{}, err
	}
	if page < 1 {
		page = 1
	}
	pageSize := compiled.definition.PageSize
	if preview {
		pageSize = 20
	}
	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return QueryResult{
		Columns:    compiled.columns,
		Rows:       items,
		Pagination: Pagination{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages},
		Timeline:   timeline,
		Meta:       QueryMeta{DatasetKey: compiled.dataset.Key, DurationMS: time.Since(started).Milliseconds(), Truncated: int64(page*pageSize) < total, TimeBucket: compiled.timelineBucket, ReadOnly: true, ResultGrain: compiled.dataset.Grain},
	}, nil
}

func scanRows(rows *sql.Rows, columns []ResultColumn) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for index, column := range columns {
			item[column.Key] = normalizeResultValue(values[index], column.Type)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeResultValue(value any, dataType string) any {
	if value == nil {
		return nil
	}
	if raw, ok := value.([]byte); ok {
		text := string(raw)
		if dataType == "number" {
			if parsed, err := strconv.ParseFloat(text, 64); err == nil {
				return parsed
			}
		}
		if dataType == "integer" {
			if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
				return parsed
			}
		}
		return text
	}
	return value
}

func mapQueryError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "statement timeout") || strings.Contains(message, "deadline exceeded") || strings.Contains(message, "canceling statement") {
		return platform.NewError(http.StatusRequestTimeout, "report query is too broad or took too long")
	}
	return err
}
