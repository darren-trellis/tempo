package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

// applyFilter filters allWorkflows based on filterText and updates the display.
func (wl *WorkflowList) applyFilter() {
	wl.applyFilterWithFallback(false)
}

// applyFilterWithFallback filters locally, optionally falling back to server-side search.
func (wl *WorkflowList) applyFilterWithFallback(serverFallback bool) {
	var filtered []temporal.Workflow
	if wl.filterText == "" {
		filtered = wl.allWorkflows
	} else {
		filtered = wl.searchMatches(wl.filterText)
		if len(filtered) == 0 && serverFallback && (wl.visibilityQuery == "" || wl.searchOwnsQuery()) {
			wl.convertFilterToVisibilityQuery()
			return
		}
		filtered = wl.expandFilteredWorkflows(filtered)
	}
	wl.applyWorkflowOrder(filtered)
	wl.populateTable()
	wl.updateStats()
}

func (wl *WorkflowList) convertFilterToVisibilityQuery() {
	if wl.filterText == "" {
		return
	}

	wl.activeFilterName = ""
	wl.filterClauses = nil
	wl.clearAdHocFilter()
	wl.visibilityQuery = workflowIDFilterQuery(wl.filterText)
	wl.updatePanelTitle()
	wl.loadData()
}

func (wl *WorkflowList) searchFilterTerm() string {
	if wl.filterText != "" {
		return wl.filterText
	}
	return searchTermFromVisibilityQuery(wl.visibilityQuery)
}

func searchTermFromVisibilityQuery(query string) string {
	const prefix = "WorkflowId STARTS_WITH '"
	if !strings.HasPrefix(query, prefix) || !strings.HasSuffix(query, "'") {
		return ""
	}
	term := strings.TrimSuffix(strings.TrimPrefix(query, prefix), "'")
	if term == "" || strings.ContainsAny(term, "'") {
		return ""
	}
	return term
}

func (wl *WorkflowList) showFilter() {
	if wl.originalWorkflows == nil {
		wl.originalWorkflows = wl.allWorkflows
	}
	wl.lastCompletionQuery = ""
	previous := wl.filterText
	wl.app.ShowSearchPrompt(previous, func(text string) {
		if text == previous {
			wl.restoreSearch(text)
			return
		}
		wl.filterText = text
		wl.applyFilterWithServerSearch(text)
	}, wl.commitFilter)
}

func (wl *WorkflowList) commitFilter(text string) {
	if text == "" {
		wl.clearSearch()
		return
	}
	wl.filterText = text
	wl.applyFilterWithFallback(true)
	wl.updatePanelTitle()
}

// searchOwnsQuery reports whether the visibility query only exists because a
// search found nothing locally and moved to the server.
func (wl *WorkflowList) searchOwnsQuery() bool {
	return wl.visibilityQuery != "" && wl.activeFilterName == "" && len(wl.filterClauses) == 0 &&
		!wl.adHoc.active() && searchTermFromVisibilityQuery(wl.visibilityQuery) != ""
}

// localSearchActive reports whether the rows shown are a search over the loaded
// pages, which paging or reloading would change underneath it.
func (wl *WorkflowList) localSearchActive() bool {
	return wl.filterText != "" && wl.visibilityQuery != workflowIDFilterQuery(wl.filterText)
}

func (wl *WorkflowList) restoreSearch(text string) {
	wl.filterText = text
	wl.lastCompletionQuery = ""
	wl.serverCompletions = nil
	if text == "" {
		wl.searchFill = nil
	}
	if text == "" && !wl.searchOwnsQuery() {
		wl.originalWorkflows = nil
	}
	wl.applyFilter()
	wl.updateFilterTitle("", "")
}

// clearSearch drops the / search and leaves the selected filter in place.
func (wl *WorkflowList) clearSearch() {
	if wl.searchOwnsQuery() {
		wl.clearAllFilters()
		return
	}
	wl.restoreSearch("")
}

// applyFilterWithServerSearch filters the loaded rows, then asks the server
// for more matches when they hold less than a page.
func (wl *WorkflowList) applyFilterWithServerSearch(text string) {
	if text == "" {
		wl.applyWorkflowOrder(wl.allWorkflows)
		wl.populateTable()
		wl.updateStats()
		wl.updateFilterTitle("", "")
		return
	}

	wl.showSearchMatches(text)
	wl.requestSearchFill(text)
}

func (wl *WorkflowList) showSearchMatches(text string) {
	wl.applyWorkflowOrder(wl.expandFilteredWorkflows(wl.searchMatches(text)))

	// Show top match hint
	topHint := ""
	if len(wl.workflows) > 0 {
		topHint = wl.workflows[0].ID
	}
	wl.updateFilterTitle(text, topHint)

	wl.populateTable()
	wl.updateStats()
}

// searchMatches is the loaded rows that match text, topped up with what the
// server found until there is a page of them. Rows found for a shorter term
// still count while the request for the longer one is out.
func (wl *WorkflowList) searchMatches(text string) []temporal.Workflow {
	matches := matchingWorkflows(wl.allWorkflows, text)
	if text == "" || !strings.HasPrefix(text, wl.searchFillTerm) || wl.searchFillBase != wl.searchBaseQuery() {
		return matches
	}
	return topUpSearchMatches(matches, wl.searchFill, text, wl.pageSize())
}

func topUpSearchMatches(matches, extra []temporal.Workflow, text string, limit int) []temporal.Workflow {
	if len(matches) >= limit {
		return matches
	}
	seen := make(map[string]bool, len(matches))
	for _, w := range matches {
		seen[workflowIdentityKey(w)] = true
	}
	filter := strings.ToLower(text)
	for _, w := range extra {
		if len(matches) >= limit {
			break
		}
		key := workflowIdentityKey(w)
		if seen[key] || !workflowMatchesFilter(w, filter) {
			continue
		}
		seen[key] = true
		matches = append(matches, w)
	}
	return matches
}

// searchBaseQuery is the query a / search narrows on the server: whatever the
// list is showing, unless the list only exists because of an earlier search.
func (wl *WorkflowList) searchBaseQuery() string {
	if wl.searchOwnsQuery() {
		return ""
	}
	return wl.pager.query
}

// requestSearchFill asks the server for more workflows matching text when the
// loaded rows hold less than a page of them. The server only matches IDs by
// prefix, so it can add rows but never stands in for the loaded ones.
func (wl *WorkflowList) requestSearchFill(text string) {
	if wl.preloaded || len(text) < 2 || wl.app == nil || wl.app.Provider() == nil {
		return
	}
	if len(matchingWorkflows(wl.allWorkflows, text)) >= wl.pageSize() {
		return
	}
	base := wl.searchBaseQuery()
	key := text + "\x00" + base
	if key == wl.lastCompletionQuery {
		return
	}
	wl.lastCompletionQuery = key
	wl.searchServer(text, base)
}

// searchServer fetches a page of workflows whose IDs start with searchTerm
// under the list's own query, to top the search results up with.
func (wl *WorkflowList) searchServer(searchTerm, base string) {
	provider := wl.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workflows, _, err := provider.ListWorkflows(ctx, wl.namespace, temporal.ListOptions{
			PageSize: wl.pageSize(),
			Query:    andFilterQueries(base, workflowIDFilterQuery(searchTerm)),
		})

		wl.app.JigApp().QueueUpdateDraw(func() {
			wl.acceptSearchFill(searchTerm, base, workflows, err)
		})
	}()
}

func (wl *WorkflowList) acceptSearchFill(searchTerm, base string, workflows []temporal.Workflow, err error) {
	// Only update if we're still searching the same term in the same list
	if err != nil || wl.filterText != searchTerm || wl.searchBaseQuery() != base {
		return
	}
	wl.searchFill = workflows
	wl.searchFillTerm = searchTerm
	wl.searchFillBase = base
	wl.serverCompletions = make([]string, 0, len(workflows))
	for _, w := range workflows {
		wl.serverCompletions = append(wl.serverCompletions, w.ID)
	}
	wl.showSearchMatches(searchTerm)
}

// updateFilterTitle updates the panel title with filter info and hint.
func (wl *WorkflowList) updateFilterTitle(filter, hint string) {
	if filter == "" {
		wl.updatePanelTitle()
		wl.app.SetFilterSuggestion("")
		return
	}

	wl.applyProfileTitle()

	// Set ghost text suggestion in command bar if we have a matching hint
	if hint != "" && strings.HasPrefix(strings.ToLower(hint), strings.ToLower(filter)) {
		wl.app.SetFilterSuggestion(hint)
	} else {
		wl.app.SetFilterSuggestion("")
	}
}

func (wl *WorkflowList) clearTransientWorkflowFilter() bool {
	if wl == nil {
		return false
	}
	if wl.filterText != "" || wl.searchOwnsQuery() {
		wl.clearSearch()
		return true
	}
	if wl.adHoc.active() {
		base := wl.adHoc.base
		name := wl.activeFilterName
		clauses := wl.filterClauses
		wl.clearAdHocFilter()
		wl.activeFilterName = name
		wl.filterClauses = clauses
		wl.runVisibilityQuery(base)
		wl.revealActiveFilterChip()
		return true
	}
	if wl.visibilityQuery != "" && wl.activeFilterName == "" {
		wl.clearAllFilters()
		return true
	}
	return false
}

func (wl *WorkflowList) clearAllFilters() {
	server := wl.visibilityQuery != ""
	wl.filterText = ""
	wl.visibilityQuery = ""
	wl.activeFilterName = ""
	wl.filterClauses = nil
	wl.clearAdHocFilter()
	wl.serverCompletions = nil
	wl.lastCompletionQuery = ""
	wl.searchFill = nil
	orig := wl.originalWorkflows
	wl.originalWorkflows = nil
	if !server {
		if orig != nil {
			wl.allWorkflows = orig
		}
		wl.applyFilter()
		wl.updatePanelTitle()
		return
	}
	wl.updatePanelTitle()
	if wl.preloaded {
		if orig != nil {
			wl.allWorkflows = orig
		}
		wl.applyFilter()
		return
	}
	wl.loadData()
}

func workflowIDFilterQuery(term string) string {
	return fmt.Sprintf("WorkflowId STARTS_WITH '%s'", term)
}

func workflowMatchesFilter(w temporal.Workflow, filter string) bool {
	return strings.Contains(strings.ToLower(w.ID), filter)
}

func matchingWorkflows(workflows []temporal.Workflow, text string) []temporal.Workflow {
	filter := strings.ToLower(text)
	if filter == "" {
		return append([]temporal.Workflow(nil), workflows...)
	}
	matches := make([]temporal.Workflow, 0, len(workflows))
	for _, w := range workflows {
		if workflowMatchesFilter(w, filter) {
			matches = append(matches, w)
		}
	}
	return matches
}

func (wl *WorkflowList) expandFilteredWorkflows(matches []temporal.Workflow) []temporal.Workflow {
	if wl == nil || !wl.workflowTreeMode {
		return matches
	}
	pool := mergeWorkflows(wl.originalWorkflows, wl.allWorkflows, matches)
	return expandWorkflowFilterTree(pool, matches)
}

func mergeWorkflows(sets ...[]temporal.Workflow) []temporal.Workflow {
	seen := make(map[string]bool)
	out := make([]temporal.Workflow, 0)
	for _, set := range sets {
		for _, w := range set {
			key := workflowIdentityKey(w)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, w)
		}
	}
	return out
}

func expandWorkflowFilterTree(all, matches []temporal.Workflow) []temporal.Workflow {
	if len(matches) == 0 {
		return nil
	}
	matchKeys := make(map[string]bool, len(matches))
	for _, w := range matches {
		matchKeys[workflowIdentityKey(w)] = true
	}
	children, _ := workflowChildLinks(all)
	include := make([]bool, len(all))
	var mark func(int)
	mark = func(i int) {
		if i < 0 || i >= len(all) || include[i] {
			return
		}
		include[i] = true
		for _, c := range children[i] {
			mark(c)
		}
	}
	for i, w := range all {
		if matchKeys[workflowIdentityKey(w)] {
			mark(i)
		}
	}
	out := make([]temporal.Workflow, 0, len(all))
	seen := make(map[string]bool, len(all))
	for i, w := range all {
		if !include[i] {
			continue
		}
		out = append(out, w)
		seen[workflowIdentityKey(w)] = true
	}
	for _, w := range matches {
		key := workflowIdentityKey(w)
		if seen[key] {
			continue
		}
		out = append(out, w)
		seen[key] = true
	}
	return out
}
