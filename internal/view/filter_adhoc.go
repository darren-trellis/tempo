package view

import (
	"strings"

	"github.com/galaxy-io/tempo/internal/config"
)

// adHocFilter holds the clauses added with f on top of whatever filter is
// active. They run right away but are never written to the config, so the
// active chip carries an asterisk while they are in play.
type adHocFilter struct {
	clauses []config.FilterClause
	base    string // the query the clauses are layered on top of
}

func (a adHocFilter) active() bool { return len(a.clauses) > 0 }

func (a adHocFilter) clone() adHocFilter {
	return adHocFilter{
		clauses: append([]config.FilterClause(nil), a.clauses...),
		base:    a.base,
	}
}

// showAdHocClauseEditor opens the clause editor and layers whatever comes back
// on top of the running query.
func (wl *WorkflowList) showAdHocClauseEditor() {
	if wl == nil {
		return
	}
	wl.showClauseEditor(newFilterClause(), wl.appendAdHocClauses)
}

func (wl *WorkflowList) appendAdHocClauses(clauses []config.FilterClause) {
	if wl == nil || len(clauses) == 0 {
		return
	}
	if !wl.adHoc.active() {
		wl.adHoc.base = wl.visibilityQuery
	}
	wl.adHoc.clauses = append(wl.adHoc.clauses, clauses...)
	wl.runVisibilityQuery(wl.adHocQuery())
	wl.revealActiveFilterChip()
}

func (wl *WorkflowList) clearAdHocFilter() {
	if wl == nil {
		return
	}
	wl.adHoc = adHocFilter{}
}

func (wl *WorkflowList) adHocQuery() string {
	if wl == nil {
		return ""
	}
	return andFilterQueries(wl.adHoc.base, compileFilterClausesFor(wl, wl.adHoc.clauses))
}

// adHocSummary reads the unsaved clauses back for the status bar.
func (wl *WorkflowList) adHocSummary() string {
	if wl == nil || !wl.adHoc.active() {
		return ""
	}
	parts := make([]string, 0, len(wl.adHoc.clauses))
	for _, clause := range wl.adHoc.clauses {
		if isRawFilterClause(clause) {
			if raw := strings.TrimSpace(clause.Value); raw != "" {
				parts = append(parts, raw)
			}
			continue
		}
		if summary := filterClauseSummary(clause); summary != "" {
			parts = append(parts, summary)
		}
	}
	return strings.Join(parts, " AND ")
}

// andFilterQueries joins queries with AND, parenthesising any part that holds a
// top-level OR so the added clause cannot bind tighter than the disjunction.
func andFilterQueries(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) < 2 {
		return strings.Join(kept, "")
	}
	for i, part := range kept {
		if queryHasTopLevelOr(part) {
			kept[i] = "(" + part + ")"
		}
	}
	return strings.Join(kept, " AND ")
}

func queryHasTopLevelOr(query string) bool {
	runes := []rune(query)
	var quote rune
	depth := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			continue
		case '(':
			depth++
			continue
		case ')':
			depth--
			continue
		}
		if depth > 0 {
			continue
		}
		word, width, ok := queryKeywordAt(runes, i)
		if !ok {
			continue
		}
		if word == "or" {
			return true
		}
		i += width - 1
	}
	return false
}
