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
		filtered = matchingWorkflows(wl.allWorkflows, wl.filterText)
		if len(filtered) == 0 && serverFallback && wl.visibilityQuery == "" {
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

	searchTerm := wl.filterText
	wl.visibilityQuery = workflowIDFilterQuery(searchTerm)
	wl.filterText = ""
	wl.updatePanelTitle()
	wl.loadData()
}

func (wl *WorkflowList) showFilter() {
	wl.originalWorkflows = wl.allWorkflows

	wl.app.ShowFilterMode(wl.filterText, FilterModeCallbacks{
		OnSubmit: func(text string) {
			wl.filterText = text
			if text != "" {
				// Apply filter with server fallback if no local results
				wl.applyFilterWithFallback(true)
			} else {
				wl.applyFilter()
			}
			wl.updatePanelTitle()
		},
		OnCancel: func() {
			wl.closeFilter()
		},
		OnChange: func(text string) {
			wl.filterText = text
			wl.applyFilterWithServerSearch(text)
		},
	})
}

// applyFilterWithServerSearch filters locally, and if no results, triggers server search.
func (wl *WorkflowList) applyFilterWithServerSearch(text string) {
	if text == "" {
		wl.applyWorkflowOrder(wl.allWorkflows)
		wl.populateTable()
		wl.updateStats()
		wl.updateFilterTitle("", "")
		return
	}

	filtered := wl.expandFilteredWorkflows(matchingWorkflows(wl.allWorkflows, text))
	wl.applyWorkflowOrder(filtered)

	// Show top match hint
	topHint := ""
	if len(wl.workflows) > 0 {
		topHint = wl.workflows[0].ID
	}
	wl.updateFilterTitle(text, topHint)

	// If no local results and query is long enough, search server
	if len(wl.workflows) == 0 && len(text) >= 2 {
		// Avoid duplicate requests
		if text == wl.lastCompletionQuery {
			return
		}
		wl.lastCompletionQuery = text
		wl.searchServer(text)
		return
	}

	wl.populateTable()
	wl.updateStats()
}

// searchServer performs a server-side search and updates the table.
func (wl *WorkflowList) searchServer(searchTerm string) {
	provider := wl.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		query := workflowIDFilterQuery(searchTerm)
		opts := temporal.ListOptions{
			PageSize: wl.pageSize(),
			Query:    query,
		}
		workflows, _, err := provider.ListWorkflows(ctx, wl.namespace, opts)

		wl.app.JigApp().QueueUpdateDraw(func() {
			// Only update if we're still filtering with the same term
			if wl.filterText != searchTerm {
				return
			}

			if err != nil {
				return
			}

			wl.applyWorkflowOrder(wl.expandFilteredWorkflows(workflows))
			wl.serverCompletions = make([]string, 0, len(workflows))
			for _, w := range workflows {
				wl.serverCompletions = append(wl.serverCompletions, w.ID)
			}

			// Update hint with top server result
			topHint := ""
			if len(workflows) > 0 {
				topHint = workflows[0].ID
			}
			wl.updateFilterTitle(searchTerm, topHint)

			wl.populateTable()
			wl.updateStats()
		})
	}()
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

func (wl *WorkflowList) closeFilter() {
	wl.serverCompletions = nil
	wl.lastCompletionQuery = ""

	if wl.filterText == "" && wl.visibilityQuery == "" && wl.originalWorkflows != nil {
		wl.allWorkflows = wl.originalWorkflows
		wl.workflows = wl.originalWorkflows
		wl.originalWorkflows = nil
		wl.populateTable()
		wl.updateStats()
		wl.updatePanelTitle()
	}
}

func (wl *WorkflowList) clearAllFilters() {
	wl.filterText = ""
	wl.visibilityQuery = ""
	wl.serverCompletions = nil
	wl.lastCompletionQuery = ""

	if wl.originalWorkflows != nil {
		wl.allWorkflows = wl.originalWorkflows
		wl.workflows = wl.originalWorkflows
		wl.originalWorkflows = nil
		wl.populateTable()
		wl.updateStats()
		wl.updatePanelTitle()
	} else {
		wl.loadData()
	}
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
