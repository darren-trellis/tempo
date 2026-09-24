package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

const (
	filterOpEq         = "eq"
	filterOpNeq        = "neq"
	filterOpStartsWith = "starts_with"
	filterOpAfter      = "after"
	filterOpBefore     = "before"
	filterOpIsNull     = "is_null"
	filterOpIsNotNull  = "is_not_null"
	filterOpRaw        = "raw"

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
)

type filterKeySpec struct {
	key   string
	label string
	kind  filterKeyKind
	ops   []string
}

var filterKeySpecs = []filterKeySpec{
	{key: "WorkflowId", label: "Workflow ID", kind: filterKeyText, ops: []string{filterOpEq, filterOpNeq, filterOpStartsWith}},
	{key: "RunId", label: "Run ID", kind: filterKeyText, ops: []string{filterOpEq, filterOpNeq, filterOpStartsWith}},
	{key: "WorkflowType", label: "Workflow Type", kind: filterKeyCatalog, ops: []string{filterOpEq, filterOpNeq, filterOpStartsWith}},
	{key: "TaskQueue", label: "Task Queue", kind: filterKeyCatalog, ops: []string{filterOpEq, filterOpNeq, filterOpStartsWith}},
	{key: "ExecutionStatus", label: "Execution Status", kind: filterKeyStatus, ops: []string{filterOpEq, filterOpNeq}},
	{key: "StartTime", label: "Start Time", kind: filterKeyTime, ops: []string{filterOpAfter, filterOpBefore}},
	{key: "CloseTime", label: "Close Time", kind: filterKeyTime, ops: []string{filterOpAfter, filterOpBefore}},
}

var filterOpLabels = map[string]string{
	filterOpEq:         "Equals",
	filterOpNeq:        "Not Equals",
	filterOpStartsWith: "Starts With",
	filterOpAfter:      "After",
	filterOpBefore:     "Before",
	filterOpIsNull:     "Is Null",
	filterOpIsNotNull:  "Is Not Null",
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
		spec.ops = []string{filterOpAfter, filterOpBefore, filterOpEq, filterOpNeq}
	case temporal.SearchAttributeInt, temporal.SearchAttributeDouble:
		spec.kind = filterKeyNumber
		spec.ops = []string{filterOpEq, filterOpNeq, filterOpAfter, filterOpBefore}
	case temporal.SearchAttributeBool:
		spec.kind = filterKeyBool
		spec.ops = []string{filterOpEq, filterOpNeq}
	default:
		spec.kind = filterKeyText
		spec.ops = []string{filterOpEq, filterOpNeq, filterOpStartsWith}
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
		ops:  []string{filterOpEq, filterOpNeq, filterOpStartsWith},
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
	changed := false
	for i, f := range cfg.GetSavedFilters() {
		if len(f.Clauses) == 0 {
			continue
		}
		if strings.TrimSpace(f.Query) == "" {
			cfg.SavedFilters[i].Query = compileFilterClauses(f.Clauses)
		}
		cfg.SavedFilters[i].Clauses = nil
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
	case filterOpAfter:
		return key + " > " + formatFilterCompare(spec, value)
	case filterOpBefore:
		return key + " < " + formatFilterCompare(spec, value)
	default:
		return ""
	}
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
	if label := filterTimePresetLabel(value); label != filterTimeCustom {
		value = label
	}
	return fmt.Sprintf("%s %s %s", clause.Key, filterOpLabel(clause.Op), value)
}

func savedFilterSummary(f config.SavedFilter) string {
	clauses := savedFilterClauses(f)
	if len(clauses) == 0 {
		return strings.TrimSpace(f.Query)
	}
	parts := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		if s := filterClauseSummary(clause); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " AND ")
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
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
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
