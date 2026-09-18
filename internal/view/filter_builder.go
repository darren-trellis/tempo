package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
)

type filterBuilderState struct {
	wl             *WorkflowList
	clauses        []config.FilterClause
	name           string
	rawQuery       string
	persistOnApply bool
}

func (wl *WorkflowList) showFilterBuilder() {
	if wl == nil {
		return
	}
	state := &filterBuilderState{
		wl:      wl,
		clauses: append([]config.FilterClause(nil), wl.filterClauses...),
		name:    wl.activeFilterName,
	}
	if len(state.clauses) == 0 && strings.TrimSpace(wl.visibilityQuery) != "" {
		state.rawQuery = wl.visibilityQuery
	}
	wl.openFilterBuilder(state)
}

func (wl *WorkflowList) openFilterBuilder(state *filterBuilderState) {
	if wl == nil || state == nil {
		return
	}
	table := components.NewTable()
	table.SetBorder(false)

	refresh := func() {
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("KEY", "OPERATOR", "VALUE")
		if len(state.clauses) == 0 {
			if q := strings.TrimSpace(state.rawQuery); q != "" {
				table.AddRow("(raw)", "", truncate(q, 48))
			}
		} else {
			for _, clause := range state.clauses {
				if isRawFilterClause(clause) {
					table.AddRow("(raw)", "", truncate(clause.Value, 52))
					continue
				}
				table.AddRow(clause.Key, filterOpLabel(clause.Op), filterClauseValueLabel(clause))
			}
		}
		if row < 0 {
			row = 0
		}
		if row >= table.RowCount() {
			row = table.RowCount() - 1
		}
		if row >= 0 {
			table.SelectRow(row)
		}
	}

	apply := func() {
		state.apply()
	}

	addClause := func() {
		wl.showClauseEditor(config.FilterClause{Key: "WorkflowId", Op: filterOpEq}, func(clause config.FilterClause) {
			state.clauses = append(state.clauses, clause)
			refresh()
		})
	}

	editClause := func() {
		row := table.SelectedRow()
		if row < 0 || row >= len(state.clauses) {
			addClause()
			return
		}
		current := state.clauses[row]
		wl.showClauseEditor(current, func(clause config.FilterClause) {
			state.clauses[row] = clause
			refresh()
		})
	}

	deleteClause := func() {
		row := table.SelectedRow()
		if row < 0 || row >= len(state.clauses) {
			return
		}
		state.clauses = append(state.clauses[:row], state.clauses[row+1:]...)
		refresh()
	}

	saveNamed := func() {
		wl.showFilterNamePrompt(state.name, func(name string) {
			state.name = name
			state.persist(name)
			wl.app.ToastSuccess("Saved filter " + name)
		})
	}

	bindings := input.NewKeyBindings().
		OnRune('n', func(e *tcell.EventKey) bool {
			addClause()
			return true
		}).
		OnRune('e', func(e *tcell.EventKey) bool {
			editClause()
			return true
		}).
		OnRune('d', func(e *tcell.EventKey) bool {
			deleteClause()
			return true
		}).
		OnRune('s', func(e *tcell.EventKey) bool {
			saveNamed()
			return true
		})

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			apply()
			return nil
		}
		if bindings.Handle(event) {
			return nil
		}
		return event
	})

	refresh()

	title := "New Filter"
	if strings.TrimSpace(state.name) != "" {
		title = "Edit Filter"
	}
	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s %s", theme.IconFilter, title),
		Width:  72,
		Height: 20,
	}, wl)
	modal.SetContent(table)
	modal.SetHints([]components.KeyHint{
		{Key: "n", Description: "Add"},
		{Key: "e", Description: "Edit"},
		{Key: "d", Description: "Delete"},
		{Key: "s", Description: "Save"},
		{Key: "Enter", Description: "Apply"},
		{Key: "Esc", Description: "Cancel"},
	})
	modal.SetOnCancel(func() {
		wl.closeModal()
	})

	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(table)
}

func (s *filterBuilderState) persist(name string) {
	if s == nil || s.wl == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.wl.persistSavedFilter(config.SavedFilter{
		Name:    name,
		Clauses: append([]config.FilterClause(nil), s.clauses...),
		Query:   s.rawQuery,
	})
}

func (s *filterBuilderState) apply() {
	if s == nil || s.wl == nil {
		return
	}
	f := config.SavedFilter{
		Name:    strings.TrimSpace(s.name),
		Clauses: append([]config.FilterClause(nil), s.clauses...),
		Query:   s.rawQuery,
	}
	if s.persistOnApply {
		if f.Name == "" {
			s.wl.showFilterNamePrompt("", func(name string) {
				s.name = name
				s.persist(name)
				s.wl.applySavedFilter(config.SavedFilter{
					Name:    name,
					Clauses: f.Clauses,
					Query:   f.Query,
				})
				s.wl.closeAllModals()
			})
			return
		}
		s.persist(f.Name)
		s.wl.applySavedFilter(f)
		s.wl.closeAllModals()
		return
	}
	s.wl.activeFilterName = ""
	s.wl.filterClauses = append([]config.FilterClause(nil), s.clauses...)
	s.wl.applyVisibilityQuery(compiledFilterQueryFor(s.wl, f))
	s.wl.closeModal()
}

func catalogOptionsForFilterKey(wl *WorkflowList, key string) []string {
	if wl == nil {
		return nil
	}
	types, queues := startWorkflowSuggestions(wl.app, wl.namespace)
	if strings.EqualFold(key, "TaskQueue") {
		return queues
	}
	return types
}

func readFilterClauseForm(kind filterKeyKind, keyField, opField, valueField, presetField *dropdownField, values map[string]any) (config.FilterClause, error) {
	clause := config.FilterClause{
		Key: strings.TrimSpace(keyField.GetValue()),
		Op:  filterOpFromLabel(opField.GetValue()),
	}
	if clause.Key == "" {
		return clause, fmt.Errorf("key is required")
	}
	if clause.Op == "" {
		clause.Op = defaultFilterOp(clause.Key)
	}
	switch kind {
	case filterKeyCatalog, filterKeyStatus, filterKeyBool:
		if valueField != nil {
			clause.Value = strings.TrimSpace(valueField.GetValue())
		}
	case filterKeyTime:
		preset := filterTimeCustom
		if presetField != nil {
			preset = presetField.GetValue()
		}
		if preset == filterTimeCustom {
			raw := strings.TrimSpace(stringValue(values, "custom"))
			parsed, err := parseFilterDateTime(raw)
			if err != nil {
				return clause, err
			}
			clause.Value = parsed
		} else {
			clause.Value = filterTimePresetValue(preset)
		}
	default:
		clause.Value = strings.TrimSpace(stringValue(values, "value"))
	}
	if clause.Value == "" {
		return clause, fmt.Errorf("value is required")
	}
	return clause, nil
}

func filterClauseValueLabel(clause config.FilterClause) string {
	value := strings.TrimSpace(clause.Value)
	if label := filterTimePresetLabel(value); label != filterTimeCustom {
		return label
	}
	if display := displayFilterDateTime(value); display != "" {
		return display
	}
	return value
}

func (wl *WorkflowList) showFilterNamePrompt(initial string, onSave func(string)) {
	form := components.NewFormBuilder().
		Text("name", "Filter Name").
		Placeholder("Enter a name").
		Value(initial).
		Done().
		OnSubmit(func(values map[string]any) {
			name := strings.TrimSpace(stringValue(values, "name"))
			if name == "" {
				wl.app.ToastWarning("Name is required")
				return
			}
			wl.closeModal()
			if onSave != nil {
				onSave(name)
			}
		}).
		OnCancel(func() {
			wl.closeModal()
		}).
		Build()

	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s Save Filter", theme.IconFilter),
		Width:  50,
		Height: 10,
	}, wl)
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Save"},
		{Key: "Esc", Description: "Cancel"},
	})
	modal.SetOnCancel(func() {
		wl.closeModal()
	})
	wl.app.PushModal(modal)
	wl.app.JigApp().SetFocus(form)
}

func (wl *WorkflowList) persistSavedFilter(f config.SavedFilter) {
	if wl == nil || wl.app == nil || wl.app.Config() == nil {
		return
	}
	wl.app.Config().SaveFilter(f)
	_ = wl.app.SaveConfig()
}

func (wl *WorkflowList) applySavedFilter(f config.SavedFilter) {
	if wl == nil {
		return
	}
	wl.activeFilterName = f.Name
	wl.filterClauses = append([]config.FilterClause(nil), f.Clauses...)
	wl.applyVisibilityQuery(compiledFilterQueryFor(wl, f))
	wl.revealActiveFilterChip()
}

func (wl *WorkflowList) applyFilterClauses(clauses []config.FilterClause) {
	if wl == nil {
		return
	}
	wl.activeFilterName = ""
	wl.filterClauses = append([]config.FilterClause(nil), clauses...)
	wl.applyVisibilityQuery(compileFilterClausesFor(wl, clauses))
}

func (wl *WorkflowList) applyAllWorkflowsFilter() {
	if wl == nil {
		return
	}
	wl.clearAllFilters()
	wl.revealActiveFilterChip()
}

func (wl *WorkflowList) revealActiveFilterChip() {
	if wl == nil || wl.filterBar == nil || wl.filtersOnSide() || wl.shouldWrapFilters() {
		return
	}
	wl.filterBar.revealChip(wl.activeFilterName)
}

func (wl *WorkflowList) closeAllModals() {
	if wl == nil || wl.app == nil || wl.app.JigApp() == nil {
		return
	}
	pages := wl.app.JigApp().Pages()
	for pages != nil && pages.CurrentIsModal() {
		pages.DismissModal()
	}
}
