package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
)

const (
	filterOpEq         = "eq"
	filterOpNeq        = "neq"
	filterOpStartsWith = "starts_with"
	filterOpAfter      = "after"
	filterOpBefore     = "before"

	filterTimeCustom = "Custom"
)

type filterKeyKind int

const (
	filterKeyText filterKeyKind = iota
	filterKeyCatalog
	filterKeyStatus
	filterKeyTime
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
}

var filterStatusValues = []string{
	"Running", "Completed", "Failed", "Canceled", "Terminated", "TimedOut", "ContinuedAsNew",
}

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

func lookupFilterKey(key string) (filterKeySpec, bool) {
	for _, spec := range filterKeySpecs {
		if strings.EqualFold(spec.key, strings.TrimSpace(key)) {
			return spec, true
		}
	}
	return filterKeySpec{}, false
}

func defaultFilterOp(key string) string {
	spec, ok := lookupFilterKey(key)
	if !ok || len(spec.ops) == 0 {
		return filterOpEq
	}
	return spec.ops[0]
}

func filterOpAllowed(key, op string) bool {
	spec, ok := lookupFilterKey(key)
	if !ok {
		return false
	}
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

func filterOpLabelsForKey(key string) []string {
	spec, ok := lookupFilterKey(key)
	if !ok {
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

func compiledFilterQuery(f config.SavedFilter) string {
	if q := compileFilterClauses(f.Clauses); q != "" {
		return q
	}
	return strings.TrimSpace(f.Query)
}

func compileFilterClauses(clauses []config.FilterClause) string {
	parts := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		part := compileFilterClause(clause)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " AND ")
}

func compileFilterClause(clause config.FilterClause) string {
	key := strings.TrimSpace(clause.Key)
	op := strings.TrimSpace(clause.Op)
	value := strings.TrimSpace(clause.Value)
	if key == "" || op == "" || value == "" {
		return ""
	}
	switch op {
	case filterOpEq:
		return key + " = " + quoteVisibilityValue(value)
	case filterOpNeq:
		return key + " != " + quoteVisibilityValue(value)
	case filterOpStartsWith:
		return key + " STARTS_WITH " + quoteVisibilityValue(value)
	case filterOpAfter:
		return key + " > " + quoteVisibilityTime(value)
	case filterOpBefore:
		return key + " < " + quoteVisibilityTime(value)
	default:
		return ""
	}
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
	value := strings.TrimSpace(clause.Value)
	if label := filterTimePresetLabel(value); label != filterTimeCustom {
		value = label
	}
	return fmt.Sprintf("%s %s %s", clause.Key, filterOpLabel(clause.Op), value)
}

func savedFilterSummary(f config.SavedFilter) string {
	if len(f.Clauses) == 0 {
		return strings.TrimSpace(f.Query)
	}
	parts := make([]string, 0, len(f.Clauses))
	for _, clause := range f.Clauses {
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
