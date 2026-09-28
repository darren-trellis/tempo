package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

const (
	filterOpEq            = "eq"
	filterOpNeq           = "neq"
	filterOpStartsWith    = "starts_with"
	filterOpNotStartsWith = "not_starts_with"
	filterOpAfter         = "after"
	filterOpOnOrAfter     = "on_or_after"
	filterOpBefore        = "before"
	filterOpOnOrBefore    = "on_or_before"
	filterOpIn            = "in"
	filterOpNotIn         = "not_in"
	filterOpBetween       = "between"
	filterOpIsNull        = "is_null"
	filterOpIsNotNull     = "is_not_null"
	filterOpRaw           = "raw"

	// filterValueSep splits the values of an IN or BETWEEN clause inside the
	// single Value string. A visibility value cannot contain it.
	filterValueSep = "\x1f"

	filterTimeCustom = "Custom"
)

type filterKeyKind int

const (
	filterKeyText filterKeyKind = iota
	filterKeyCatalog
	filterKeyStatus
	filterKeyTime
	filterKeyNumber
	filterKeyBool
	// filterKeyNone is used by operators that take no value, so the editor
	// renders no value widget at all.
	filterKeyNone
	// filterKeyList is a comma-separated set of values, for IN and NOT IN.
	filterKeyList
	// filterKeyRange is a pair of bounds, for BETWEEN.
	filterKeyRange
)

type filterKeySpec struct {
	key   string
	label string
	kind  filterKeyKind
	ops   []string
}

var filterKeySpecs = []filterKeySpec{
	{key: "WorkflowId", label: "Workflow ID", kind: filterKeyText, ops: filterKeywordOps()},
	{key: "RunId", label: "Run ID", kind: filterKeyText, ops: filterKeywordOps()},
	{key: "WorkflowType", label: "Workflow Type", kind: filterKeyCatalog, ops: filterKeywordOps()},
	{key: "TaskQueue", label: "Task Queue", kind: filterKeyCatalog, ops: filterKeywordOps()},
	{key: "ExecutionStatus", label: "Execution Status", kind: filterKeyStatus, ops: filterSetOps()},
	{key: "StartTime", label: "Start Time", kind: filterKeyTime, ops: filterRangeOps()},
	{key: "CloseTime", label: "Close Time", kind: filterKeyTime, ops: filterRangeOps()},
}

// filterKeywordOps are the operators a Keyword attribute accepts: exact and
// prefix matching, set membership, and a range, which is how a suffix match
// is written.
func filterKeywordOps() []string {
	return []string{filterOpEq, filterOpNeq, filterOpStartsWith, filterOpNotStartsWith, filterOpIn, filterOpNotIn, filterOpBetween}
}

// filterSetOps are the operators an enum such as ExecutionStatus accepts.
func filterSetOps() []string {
	return []string{filterOpEq, filterOpNeq, filterOpIn, filterOpNotIn}
}

// filterRangeOps are the operators a time accepts, strict and inclusive.
func filterRangeOps() []string {
	return []string{filterOpAfter, filterOpOnOrAfter, filterOpBefore, filterOpOnOrBefore, filterOpEq, filterOpNeq, filterOpBetween}
}

var filterOpLabels = map[string]string{
	filterOpEq:            "Equals",
	filterOpNeq:           "Not Equals",
	filterOpStartsWith:    "Starts With",
	filterOpNotStartsWith: "Not Starts With",
	filterOpAfter:         "After",
	filterOpOnOrAfter:     "On or After",
	filterOpBefore:        "Before",
	filterOpOnOrBefore:    "On or Before",
	filterOpIn:            "In",
	filterOpNotIn:         "Not In",
	filterOpBetween:       "Between",
	filterOpIsNull:        "Is Null",
	filterOpIsNotNull:     "Is Not Null",
}

// filterOpNeedsValue reports whether an operator takes a value. IS NULL and
// IS NOT NULL stand alone, so the editor hides the value field for them.
func filterOpNeedsValue(op string) bool {
	switch strings.TrimSpace(op) {
	case filterOpIsNull, filterOpIsNotNull:
		return false
	default:
		return true
	}
}

var filterStatusValues = []string{
	"Running", "Completed", "Failed", "Canceled", "Terminated", "TimedOut", "ContinuedAsNew",
}

var filterBoolValues = []string{"true", "false"}

type filterTimePreset struct {
	label string
	value string
}

var filterTimePresets = []filterTimePreset{
	{label: "Last 30 min", value: "$MINUTES_AGO_30"},
	{label: "Last hour", value: "$HOUR_AGO"},
	{label: "Last 24 hours", value: "$HOURS_AGO_24"},
	{label: "Today", value: "$TODAY"},
	{label: "Yesterday", value: "$YESTERDAY"},
	{label: "This week", value: "$THIS_WEEK"},
	{label: "Last 7 days", value: "$DAYS_AGO_7"},
	{label: "Last 30 days", value: "$DAYS_AGO_30"},
	{label: "6 hours ago", value: "$HOURS_AGO_6"},
	{label: filterTimeCustom, value: ""},
}

func filterKeyNames() []string {
	names := make([]string, len(filterKeySpecs))
	for i, spec := range filterKeySpecs {
		names[i] = spec.key
	}
	return names
}

func filterKeyNamesFor(wl *WorkflowList) []string {
	names := filterKeyNames()
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[strings.ToLower(name)] = true
	}
	for _, spec := range customFilterSpecs(wl) {
		if seen[strings.ToLower(spec.key)] {
			continue
		}
		seen[strings.ToLower(spec.key)] = true
		names = append(names, spec.key)
	}
	return names
}

func lookupFilterKey(key string) (filterKeySpec, bool) {
	for _, spec := range filterKeySpecs {
		if strings.EqualFold(spec.key, strings.TrimSpace(key)) {
			return spec, true
		}
	}
	return filterKeySpec{}, false
}

func customFilterSpecs(wl *WorkflowList) []filterKeySpec {
	if wl == nil || wl.app == nil {
		return nil
	}
	ns := wl.namespace
	attrs := wl.app.catalog.getAttrs(ns)
	if len(attrs) == 0 {
		if alt := wl.app.catalogNamespace(); alt != "" && alt != ns {
			attrs = wl.app.catalog.getAttrs(alt)
		}
	}
	out := make([]filterKeySpec, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, specFromSearchAttribute(attr))
	}
	return out
}

func specFromSearchAttribute(attr temporal.SearchAttribute) filterKeySpec {
	spec := filterKeySpec{key: attr.Name, label: attr.Name}
	switch attr.Type {
	case temporal.SearchAttributeDatetime:
		spec.kind = filterKeyTime
		spec.ops = filterRangeOps()
	case temporal.SearchAttributeInt, temporal.SearchAttributeDouble:
		spec.kind = filterKeyNumber
		spec.ops = []string{filterOpEq, filterOpNeq, filterOpAfter, filterOpOnOrAfter, filterOpBefore, filterOpOnOrBefore, filterOpIn, filterOpNotIn, filterOpBetween}
	case temporal.SearchAttributeBool:
		spec.kind = filterKeyBool
		spec.ops = filterSetOps()
	case temporal.SearchAttributeKeywordList:
		spec.kind = filterKeyText
		spec.ops = filterSetOps()
	default:
		spec.kind = filterKeyText
		spec.ops = filterKeywordOps()
	}
	return spec
}

func resolveFilterKey(wl *WorkflowList, key string) filterKeySpec {
	key = strings.TrimSpace(key)
	if spec, ok := lookupFilterKey(key); ok {
		return withNullOps(spec)
	}
	for _, spec := range customFilterSpecs(wl) {
		if strings.EqualFold(spec.key, key) {
			return withNullOps(spec)
		}
	}
	if key == "" {
		return withNullOps(filterKeySpecs[0])
	}
	return withNullOps(filterKeySpec{
		key:  key,
		kind: filterKeyText,
		ops:  []string{filterOpEq, filterOpNeq, filterOpStartsWith, filterOpNotStartsWith},
	})
}

// withNullOps offers the value-less operators on every key. The server accepts
// IS NULL against any attribute, and copying the operators into a fresh slice
// keeps the shared spec tables untouched.
func withNullOps(spec filterKeySpec) filterKeySpec {
	ops := make([]string, 0, len(spec.ops)+2)
	for _, op := range spec.ops {
		if op == filterOpIsNull || op == filterOpIsNotNull {
			continue
		}
		ops = append(ops, op)
	}
	spec.ops = append(ops, filterOpIsNull, filterOpIsNotNull)
	return spec
}

// newFilterClause is the clause a fresh row starts from.
func newFilterClause() config.FilterClause {
	return config.FilterClause{Key: "WorkflowId", Op: filterOpEq}
}

func defaultFilterOp(key string) string {
	return defaultFilterOpFor(nil, key)
}

func defaultFilterOpFor(wl *WorkflowList, key string) string {
	spec := resolveFilterKey(wl, key)
	if len(spec.ops) == 0 {
		return filterOpEq
	}
	return spec.ops[0]
}

func filterOpAllowedFor(wl *WorkflowList, key, op string) bool {
	spec := resolveFilterKey(wl, key)
	for _, allowed := range spec.ops {
		if allowed == op {
			return true
		}
	}
	return false
}

func filterOpLabel(op string) string {
	if label, ok := filterOpLabels[op]; ok {
		return label
	}
	return op
}

func filterOpFromLabel(label string) string {
	for op, name := range filterOpLabels {
		if strings.EqualFold(name, label) {
			return op
		}
	}
	return strings.TrimSpace(label)
}

func filterOpLabelsForKeyFor(wl *WorkflowList, key string) []string {
	spec := resolveFilterKey(wl, key)
	if len(spec.ops) == 0 {
		return []string{filterOpLabels[filterOpEq]}
	}
	out := make([]string, 0, len(spec.ops))
	for _, op := range spec.ops {
		out = append(out, filterOpLabels[op])
	}
	return out
}

func filterTimePresetLabels() []string {
	out := make([]string, len(filterTimePresets))
	for i, p := range filterTimePresets {
		out[i] = p.label
	}
	return out
}

func filterTimePresetLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return filterTimeCustom
	}
	for _, p := range filterTimePresets {
		if p.value == value {
			return p.label
		}
	}
	return filterTimeCustom
}

func filterTimePresetValue(label string) string {
	for _, p := range filterTimePresets {
		if p.label == label {
			return p.value
		}
	}
	return ""
}

func compiledFilterQueryFor(wl *WorkflowList, f config.SavedFilter) string {
	if q := strings.TrimSpace(f.Query); q != "" {
		return q
	}
	return compileFilterClausesFor(wl, f.Clauses)
}

// savedFilterClauses parses a saved filter into the rows the builder edits.
// Filters are stored as a query string, so the clauses only exist at runtime.
func savedFilterClauses(f config.SavedFilter) []config.FilterClause {
	if q := strings.TrimSpace(f.Query); q != "" {
		return filterClausesFromQuery(q)
	}
	return append([]config.FilterClause(nil), f.Clauses...)
}

// migrateSavedFilters folds the key/op/value clauses of older configs into the
// query that filters are stored as now. It reports whether anything changed.
func migrateSavedFilters(cfg *config.Config) bool {
	changed := migrateFilterList(cfg.SavedFilters)
	for _, list := range cfg.ProfileFilters {
		changed = migrateFilterList(list) || changed
	}
	return changed
}

func migrateFilterList(filters []config.SavedFilter) bool {
	changed := false
	for i, f := range filters {
		if len(f.Clauses) == 0 {
			continue
		}
		if strings.TrimSpace(f.Query) == "" {
			filters[i].Query = compileFilterClauses(f.Clauses)
		}
		filters[i].Clauses = nil
		changed = true
	}
	return changed
}

func compileFilterClauses(clauses []config.FilterClause) string {
	return compileFilterClausesFor(nil, clauses)
}

func compileFilterClausesFor(wl *WorkflowList, clauses []config.FilterClause) string {
	parts := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		part := compileFilterClauseWith(clause, resolveFilterKey(wl, clause.Key))
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) > 1 {
		for i, part := range parts {
			if root := parseFilterTree(part); root != nil && root.op == filterGroupOr && root.source == part {
				parts[i] = "(" + part + ")"
			}
		}
	}
	return strings.Join(parts, " AND ")
}

func isRawFilterClause(clause config.FilterClause) bool {
	return strings.EqualFold(strings.TrimSpace(clause.Op), filterOpRaw) ||
		strings.EqualFold(strings.TrimSpace(clause.Key), filterOpRaw)
}

func compileFilterClauseWith(clause config.FilterClause, spec filterKeySpec) string {
	key := strings.TrimSpace(clause.Key)
	op := strings.TrimSpace(clause.Op)
	value := strings.TrimSpace(clause.Value)
	if isRawFilterClause(clause) {
		return value
	}
	if key == "" || op == "" {
		return ""
	}
	switch op {
	case filterOpIsNull:
		return key + " IS NULL"
	case filterOpIsNotNull:
		return key + " IS NOT NULL"
	}
	if value == "" {
		return ""
	}
	switch op {
	case filterOpEq:
		return key + " = " + formatFilterValue(spec, value)
	case filterOpNeq:
		return key + " != " + formatFilterValue(spec, value)
	case filterOpStartsWith:
		return key + " STARTS_WITH " + quoteVisibilityValue(value)
	case filterOpNotStartsWith:
		return key + " NOT STARTS_WITH " + quoteVisibilityValue(value)
	case filterOpAfter:
		return key + " > " + formatFilterCompare(spec, value)
	case filterOpOnOrAfter:
		return key + " >= " + formatFilterCompare(spec, value)
	case filterOpBefore:
		return key + " < " + formatFilterCompare(spec, value)
	case filterOpOnOrBefore:
		return key + " <= " + formatFilterCompare(spec, value)
	case filterOpIn, filterOpNotIn:
		items := filterListValues(value)
		if len(items) == 0 {
			return ""
		}
		quoted := make([]string, len(items))
		for i, item := range items {
			quoted[i] = formatFilterValue(spec, item)
		}
		word := "IN"
		if op == filterOpNotIn {
			word = "NOT IN"
		}
		return key + " " + word + " (" + strings.Join(quoted, ", ") + ")"
	case filterOpBetween:
		bounds := filterListValues(value)
		if len(bounds) != 2 {
			return ""
		}
		return key + " BETWEEN " + formatFilterCompare(spec, bounds[0]) + " AND " + formatFilterCompare(spec, bounds[1])
	default:
		return ""
	}
}

// filterOpValueShape reports how many values an operator takes, so the form
// can show one field, a comma-separated list, or a From and a To.
func filterOpValueShape(op string) int {
	switch strings.TrimSpace(op) {
	case filterOpIsNull, filterOpIsNotNull:
		return 0
	case filterOpIn, filterOpNotIn:
		return -1
	case filterOpBetween:
		return 2
	default:
		return 1
	}
}

func filterListValues(value string) []string {
	var out []string
	for _, part := range strings.Split(value, filterValueSep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func joinFilterValues(values []string) string {
	return strings.Join(values, filterValueSep)
}

// splitFilterListInput reads what was typed into the Values field. A comma
// splits items, and quotes keep a comma that belongs to a value.
func splitFilterListInput(raw string) []string {
	var items []string
	var current []rune
	inQuote := rune(0)
	flush := func() {
		item := strings.TrimSpace(string(current))
		current = nil
		if item != "" {
			if q := rune(item[0]); (q == '\'' || q == '"') && strings.HasSuffix(item, string(q)) && len([]rune(item)) >= 2 {
				item = strings.TrimSpace(string([]rune(item)[1 : len([]rune(item))-1]))
			}
		}
		if item != "" {
			items = append(items, item)
		}
	}
	for _, r := range raw {
		switch {
		case inQuote != 0:
			current = append(current, r)
			if r == inQuote {
				inQuote = 0
			}
		case r == '\'' || r == '"':
			inQuote = r
			current = append(current, r)
		case r == ',':
			flush()
		default:
			current = append(current, r)
		}
	}
	flush()
	return items
}

func rangeBoundPlaceholder(kind filterKeyKind) string {
	if kind == filterKeyTime {
		return "2024-01-02 15:04"
	}
	return "Value"
}

func formatFilterValue(spec filterKeySpec, value string) string {
	switch spec.kind {
	case filterKeyNumber, filterKeyBool:
		return value
	default:
		return quoteVisibilityValue(value)
	}
}

func formatFilterCompare(spec filterKeySpec, value string) string {
	if spec.kind == filterKeyNumber {
		return value
	}
	return quoteVisibilityTime(value)
}

func quoteVisibilityValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quoteVisibilityTime(value string) string {
	if strings.HasPrefix(value, "$") {
		return value
	}
	return quoteVisibilityValue(value)
}

func filterClauseSummary(clause config.FilterClause) string {
	if strings.TrimSpace(clause.Key) == "" {
		return ""
	}
	if !filterOpNeedsValue(clause.Op) {
		return clause.Key + " " + filterOpLabel(clause.Op)
	}
	value := strings.TrimSpace(clause.Value)
	switch filterOpValueShape(clause.Op) {
	case -1:
		value = strings.Join(filterListValues(value), ", ")
	case 2:
		if bounds := filterListValues(value); len(bounds) == 2 {
			value = displayFilterBound(bounds[0]) + " and " + displayFilterBound(bounds[1])
		}
	default:
		if label := filterTimePresetLabel(value); label != filterTimeCustom {
			value = label
		}
	}
	return fmt.Sprintf("%s %s %s", clause.Key, filterOpLabel(clause.Op), value)
}

func displayFilterBound(value string) string {
	if label := filterTimePresetLabel(value); label != filterTimeCustom {
		return label
	}
	return value
}

func parseFilterDateTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("time is required")
	}
	if strings.HasPrefix(value, "$") {
		return value, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	// A bare date is parsed in UTC so it means the same wherever the config is
	// read, while a clock time is the reader's local time.
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("use YYYY-MM-DD HH:MM")
}

func displayFilterDateTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "$") {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return value
}
