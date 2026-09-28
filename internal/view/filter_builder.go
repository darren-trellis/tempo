package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type filterBuilderState struct {
	wl             *WorkflowList
	clauses        []config.FilterClause
	name           string
	persistOnApply bool
	onSaved        func()
}

func (wl *WorkflowList) openFilterBuilder(state *filterBuilderState) {
	if wl == nil || state == nil {
		return
	}
	table := components.NewTable()
	table.SetBorder(false)

	rawEditor := tview.NewTextArea().
		SetWrap(true).
		SetWordWrap(true).
		SetPlaceholder("ExecutionStatus = 'Running'")
	rawEditor.SetBackgroundColor(theme.Bg())
	rawEditor.SetBorderPadding(0, 0, 1, 1)
	rawEditor.SetTextStyle(tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.Fg()))
	rawEditor.SetPlaceholderStyle(tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgDim()))
	rawEditor.SetSelectedStyle(tcell.StyleDefault.Background(theme.Accent()).Foreground(theme.Bg()))
	// rawQuery is the query the editor was last filled with, so edits can be
	// told apart from an untouched editor whose clauses must stay as they are.
	rawQuery := ""

	refresh := func() {
		rawQuery = compileFilterClausesFor(wl, state.clauses)
		rawEditor.SetText(rawQuery, true)
		row := table.SelectedRow()
		table.ClearRows()
		table.SetHeaders("KEY", "OPERATOR", "VALUE")
		for _, clause := range state.clauses {
			if isRawFilterClause(clause) {
				table.AddRow("(raw)", "", tview.Escape(truncate(clause.Value, 52)))
				continue
			}
			table.AddRow(tview.Escape(clause.Key), filterOpLabel(clause.Op), tview.Escape(filterClauseValueLabel(clause)))
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

	// commitRaw folds the editor's text back into clauses. It reports false,
	// leaving the reader on the raw tab, when there is nothing to keep.
	commitRaw := func() bool {
		edited := strings.TrimSpace(singleLine.Replace(rawEditor.GetText()))
		if edited == rawQuery {
			return true
		}
		if edited == "" {
			wl.app.ToastWarning("Filter is empty")
			return false
		}
		state.clauses = filterClausesFromQuery(edited)
		refresh()
		return true
	}

	const rawTab = 1
	activeTab := 0
	onRawTab := func() bool {
		return activeTab == rawTab
	}

	apply := func() {
		if onRawTab() && !commitRaw() {
			return
		}
		state.apply()
	}

	addClause := func() {
		wl.showClauseEditor(newFilterClause(), func(clauses []config.FilterClause) {
			state.clauses = append(state.clauses, clauses...)
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
		wl.showClauseEditor(current, func(clauses []config.FilterClause) {
			replaced := make([]config.FilterClause, 0, len(state.clauses)+len(clauses)-1)
			replaced = append(replaced, state.clauses[:row]...)
			replaced = append(replaced, clauses...)
			replaced = append(replaced, state.clauses[row+1:]...)
			state.clauses = replaced
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
		if onRawTab() && !commitRaw() {
			return
		}
		wl.showFilterNamePrompt("Save Filter", state.name, func(name string) {
			state.name = name
			state.persist(name)
			wl.app.ToastSuccess("Saved filter " + name)
		})
	}

	editRaw := func() {
		if onRawTab() && !commitRaw() {
			return
		}
		query := compileFilterClausesFor(wl, state.clauses)
		edited, ok := editFilterQueryInEditor(wl.app, query)
		if !ok {
			return
		}
		edited = strings.TrimSpace(singleLine.Replace(edited))
		if edited == query {
			return
		}
		if edited == "" {
			wl.app.ToastWarning("Filter is empty")
			return
		}
		state.clauses = filterClausesFromQuery(edited)
		refresh()
	}

	clauseBindings := input.NewKeyBindings().
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
		})
	sharedBindings := input.NewKeyBindings().
		OnRune('s', func(e *tcell.EventKey) bool {
			saveNamed()
			return true
		})

	title := "New Filter"
	if strings.TrimSpace(state.name) != "" {
		title = "Edit Filter"
	}
	modal := newOverlayModal(components.ModalConfig{
		Title:  fmt.Sprintf("%s %s", theme.IconFilter, title),
		Width:  72,
		Height: 20,
	}, wl)

	tabs := components.NewTabs().
		SetShowIcons(false).
		SetShowBadges(false).
		AddTab("Clauses", table).
		AddTab("Raw", rawEditor)
	applyHints := func() {
		var hints []components.KeyHint
		if onRawTab() {
			hints = []components.KeyHint{
				{Key: "Ctrl+E", Description: "Open in Editor"},
				{Key: "Tab", Description: "Switch Tab"},
				{Key: "Ctrl+S", Description: "Save"},
			}
		} else {
			hints = []components.KeyHint{
				{Key: "n", Description: "Add"},
				{Key: "e", Description: "Edit"},
				{Key: "d", Description: "Delete"},
				{Key: "Ctrl+E", Description: "Open in Editor"},
				{Key: "[/]", Description: "Switch Tab"},
				{Key: "s", Description: "Save"},
			}
		}
		hints = append(hints,
			components.KeyHint{Key: "Enter", Description: "Apply"},
			components.KeyHint{Key: "Esc", Description: "Cancel"},
		)
		modal.SetHints(hints)
		wl.app.syncModalHints(modal)
	}
	focusActive := func() {
		if jig := wl.app.JigApp(); jig != nil {
			if onRawTab() {
				jig.SetFocus(rawEditor)
				return
			}
			jig.SetFocus(table)
		}
	}
	tabs.SetOnChange(func(index int, _ string) {
		if onRawTab() && index != rawTab && !commitRaw() {
			refresh()
		}
		activeTab = index
		applyHints()
		focusActive()
	})
	switchTab := func() {
		if onRawTab() && !commitRaw() {
			return
		}
		tabs.SetActive(1 - activeTab)
	}

	// On the raw tab every printable key is text, so [ and ] only switch tabs
	// from the clause list, and saving moves to Ctrl+S.
	capture := func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEnter:
			apply()
			return nil
		case isFilterEditKey(event):
			editRaw()
			return nil
		case event.Key() == tcell.KeyTab, event.Key() == tcell.KeyBacktab,
			isClauseTabNext(event), isClauseTabPrev(event):
			switchTab()
			return nil
		}
		if onRawTab() {
			if event.Key() == tcell.KeyCtrlS {
				saveNamed()
				return nil
			}
			return event
		}
		if event.Key() == tcell.KeyRune && event.Modifiers()&tcell.ModCtrl == 0 {
			switch event.Rune() {
			case '[', ']':
				switchTab()
				return nil
			}
		}
		if clauseBindings.Handle(event) || sharedBindings.Handle(event) {
			return nil
		}
		return event
	}
	table.SetInputCapture(capture)
	rawEditor.SetInputCapture(capture)
	// Tabs claims digits and H/L for itself before its content sees them;
	// hand those straight to the editor so they can be typed.
	tabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if capture(event) == nil {
			return nil
		}
		if onRawTab() && isJigTabsNavKey(event) {
			if handler := rawEditor.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		}
		return event
	})

	refresh()

	modal.SetContent(tabs)
	applyHints()
	modal.SetOnCancel(func() {
		wl.closeModal()
	})

	wl.app.PushModal(modal)
	focusActive()
}

var editFilterQueryInEditor = func(app *App, query string) (string, bool) {
	return editInEditor(app, "filter", ".sql", query+"\n")
}

func (s *filterBuilderState) persist(name string) {
	if s == nil || s.wl == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.wl.persistSavedFilter(s.savedFilter(name))
	if s.onSaved != nil {
		s.onSaved()
	}
}

// savedFilter compiles the builder rows back into the stored form, a single
// visibility query.
func (s *filterBuilderState) savedFilter(name string) config.SavedFilter {
	return config.SavedFilter{
		Name:  strings.TrimSpace(name),
		Query: compileFilterClausesFor(s.wl, s.clauses),
	}
}

func (s *filterBuilderState) apply() {
	if s == nil || s.wl == nil {
		return
	}
	f := s.savedFilter(s.name)
	if s.persistOnApply {
		if f.Name == "" {
			s.wl.showFilterNamePrompt("Save Filter", "", func(name string) {
				s.name = name
				s.persist(name)
				s.wl.applySavedFilter(s.savedFilter(name))
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

func readFilterClauseForm(kind filterKeyKind, spec filterKeySpec, keyField, opField, valueField, presetField *dropdownField, values map[string]any) (config.FilterClause, error) {
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
	if !filterOpNeedsValue(clause.Op) {
		clause.Value = ""
		return clause, nil
	}
	switch kind {
	case filterKeyCatalog, filterKeyStatus, filterKeyBool:
		if valueField != nil {
			clause.Value = strings.TrimSpace(valueField.GetValue())
		}
	case filterKeyList:
		items := splitFilterListInput(stringValue(values, "values"))
		if len(items) == 0 {
			return clause, fmt.Errorf("at least one value is required")
		}
		clause.Value = joinFilterValues(items)
		return clause, nil
	case filterKeyRange:
		from := strings.TrimSpace(stringValue(values, "from"))
		to := strings.TrimSpace(stringValue(values, "to"))
		if from == "" || to == "" {
			return clause, fmt.Errorf("both bounds are required")
		}
		if spec.kind == filterKeyTime {
			var err error
			if from, err = parseFilterDateTime(from); err != nil {
				return clause, err
			}
			if to, err = parseFilterDateTime(to); err != nil {
				return clause, err
			}
		}
		clause.Value = joinFilterValues([]string{from, to})
		return clause, nil
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

func (wl *WorkflowList) showFilterNamePrompt(title, initial string, onSave func(string)) {
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
		Title:  fmt.Sprintf("%s %s", theme.IconFilter, title),
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
	wl.app.Config().SaveFilterFor(wl.app.activeProfile, f)
	_ = wl.app.SaveConfig()
}

func (wl *WorkflowList) applySavedFilter(f config.SavedFilter) {
	if wl == nil {
		return
	}
	wl.activeFilterName = f.Name
	wl.filterClauses = savedFilterClauses(f)
	wl.applyVisibilityQuery(compiledFilterQueryFor(wl, f))
	wl.revealActiveFilterChip()
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
