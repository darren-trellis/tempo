package view

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// applyVisibilityQuery runs a query on its own, dropping any ad-hoc clauses
// since the new query replaces whatever they were layered on.
func (wl *WorkflowList) applyVisibilityQuery(query string) {
	wl.clearAdHocFilter()
	wl.runVisibilityQuery(query)
}

func (wl *WorkflowList) runVisibilityQuery(query string) {
	if query != "" && query != wl.visibilityQuery && !wl.filterTestPending {
		wl.addToHistory(query)
	}
	wl.visibilityQuery = query
	wl.filterText = ""
	wl.updatePanelTitle()
	wl.loadData()
}

func (wl *WorkflowList) addToHistory(query string) {
	if len(wl.searchHistory) > 0 && wl.searchHistory[len(wl.searchHistory)-1] == query {
		return
	}
	wl.searchHistory = append(wl.searchHistory, query)
	if len(wl.searchHistory) > wl.maxHistorySize {
		wl.searchHistory = wl.searchHistory[1:]
	}
	wl.historyIndex = -1
}

func (wl *WorkflowList) updatePanelTitle() {
	if wl.workflowTab != nil && !wl.selectionMode {
		wl.workflowTab.Name = workflowListTabName()
	}
	wl.applyProfileTitle()
}

func resolveTimePlaceholders(query string) (string, error) {
	now := time.Now()

	replacements := map[string]string{
		"$TODAY":     startOfDay(now).Format(time.RFC3339),
		"$YESTERDAY": startOfDay(now.AddDate(0, 0, -1)).Format(time.RFC3339),
		"$THIS_WEEK": startOfWeek(now).Format(time.RFC3339),
		"$HOUR_AGO":  now.Add(-1 * time.Hour).Format(time.RFC3339),
	}

	result := query
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, "'"+value+"'")
	}

	patterns := []struct {
		prefix string
		unit   time.Duration
		isDate bool
	}{
		{"$HOURS_AGO_", time.Hour, false},
		{"$MINUTES_AGO_", time.Minute, false},
		{"$DAYS_AGO_", 24 * time.Hour, true},
	}

	for _, p := range patterns {
		for {
			idx := strings.Index(result, p.prefix)
			if idx == -1 {
				break
			}

			endIdx := idx + len(p.prefix)
			for endIdx < len(result) && result[endIdx] >= '0' && result[endIdx] <= '9' {
				endIdx++
			}

			if endIdx == idx+len(p.prefix) {
				return "", fmt.Errorf("invalid placeholder: %s (missing number)", p.prefix)
			}

			numStr := result[idx+len(p.prefix) : endIdx]
			num, err := strconv.Atoi(numStr)
			if err != nil {
				return "", fmt.Errorf("invalid number in placeholder %s%s: %w", p.prefix, numStr, err)
			}

			var t time.Time
			if p.isDate {
				t = startOfDay(now.Add(-time.Duration(num) * p.unit))
			} else {
				t = now.Add(-time.Duration(num) * p.unit)
			}

			placeholder := p.prefix + numStr
			result = strings.Replace(result, placeholder, "'"+t.Format(time.RFC3339)+"'", 1)
		}
	}

	return result, nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysToMonday := weekday - 1
	monday := t.AddDate(0, 0, -daysToMonday)
	return startOfDay(monday)
}
