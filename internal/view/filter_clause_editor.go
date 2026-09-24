package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (wl *WorkflowList) showClauseEditor(initial config.FilterClause, onSave func([]config.FilterClause)) {
	var modal *overlayModal
	var pushed bool
	activeTab := 0
	if isRawFilterClause(initial) {
		activeTab = 1
	}
	rawText := strings.TrimSpace(initial.Value)
	if !isRawFilterClause(initial) {
		if q := compileFilterClauseFor(wl, initial); q != "" {
			rawText = q
		}
	}

	clauseHints := func() []components.KeyHint {
		return []components.KeyHint{
			{Key: "Ctrl+[", Description: "Prev"},
			{Key: "Ctrl+]", Description: "Next"},
			{Key: "Ctrl+T", Description: "Test"},
			{Key: "Enter", Description: "Save"},
			{Key: "Esc", Description: "Cancel"},
		}
	}
	applyHints := func() {
		if modal == nil {
			return
		}
		modal.SetHints(clauseHints())
		if wl.app != nil {
			wl.app.syncModalHints(modal)
		}
	}

	var rebuild func(config.FilterClause)
	rebuild = func(current config.FilterClause) {
		key := strings.TrimSpace(current.Key)
		if key == "" || isRawFilterClause(current) {
			key = filterKeySpecs[0].key
			current.Key = key
			if isRawFilterClause(initial) && current.Op == filterOpRaw {
				current.Op = defaultFilterOpFor(wl, key)
				current.Value = ""
			}
		}
		spec := resolveFilterKey(wl, key)
		current.Key = key
		if current.Op == "" || current.Op == filterOpRaw || !filterOpAllowedFor(wl, key, current.Op) {
			current.Op = defaultFilterOpFor(wl, key)
		}

		keyField := newOrderedDropdownField("key", "Key", filterKeyNamesFor(wl)).SetValue(key)
		wl.refreshFilterKeyOptions(keyField)
		opField := newOrderedDropdownField("op", "Operator", filterOpLabelsForKeyFor(wl, key)).SetValue(filterOpLabel(current.Op))
		keyField.SetChangedFunc(func(v string) {
			if strings.EqualFold(v, current.Key) {
				return
			}
			current.Key = v
			current.Op = defaultFilterOpFor(wl, v)
			current.Value = ""
			rebuild(current)
		})
		opField.SetChangedFunc(func(v string) {
			op := filterOpFromLabel(v)
			if filterOpNeedsValue(op) != filterOpNeedsValue(current.Op) {
				current.Op = op
				rebuild(current)
				return
			}
			current.Op = op
		})

		fields := []*dropdownField{keyField, opField}
		builder := components.NewFormBuilder().AddField(keyField).AddField(opField)
		var valueField *dropdownField
		var presetField *dropdownField

		// IS NULL and IS NOT NULL stand alone, so the form stops at the
		// operator.
		kind := spec.kind
		if !filterOpNeedsValue(current.Op) {
			kind = filterKeyNone
		}

		switch kind {
		case filterKeyCatalog:
			valueField = newDropdownField("value", "Value", catalogOptionsForFilterKey(wl, key)).
				SetPlaceholder("Value").
				SetValue(current.Value)
			builder.AddField(valueField)
			fields = append(fields, valueField)
		case filterKeyStatus:
			status := current.Value
			if status == "" {
				status = filterStatusValues[0]
			}
			valueField = newOrderedDropdownField("value", "Value", filterStatusValues).SetValue(status)
			builder.AddField(valueField)
			fields = append(fields, valueField)
		case filterKeyBool:
			status := current.Value
			if status == "" {
				status = filterBoolValues[0]
			}
			valueField = newOrderedDropdownField("value", "Value", filterBoolValues).SetValue(status)
			builder.AddField(valueField)
			fields = append(fields, valueField)
		case filterKeyTime:
			presetLabel := filterTimePresetLabel(current.Value)
			presetField = newOrderedDropdownField("preset", "Value", filterTimePresetLabels()).SetValue(presetLabel)
			presetField.SetChangedFunc(func(v string) {
				wasCustom := filterTimePresetLabel(current.Value) == filterTimeCustom
				nowCustom := v == filterTimeCustom
				if nowCustom {
					if strings.HasPrefix(strings.TrimSpace(current.Value), "$") {
						current.Value = ""
					}
				} else {
					current.Value = filterTimePresetValue(v)
				}
				if wasCustom != nowCustom {
					rebuild(current)
				}
			})
			builder.AddField(presetField)
			fields = append(fields, presetField)
			if presetLabel == filterTimeCustom {
				builder.Text("custom", "Custom (YYYY-MM-DD HH:MM)").
					Placeholder("2024-01-02 15:04").
					Value(displayFilterDateTime(current.Value)).
					Done()
			}
		case filterKeyNone:
		default:
			builder.Text("value", "Value").
				Placeholder("Value").
				Value(current.Value).
				Done()
		}

		form := builder.
			OnSubmit(func(values map[string]any) {
				clause, err := readFilterClauseForm(spec.kind, keyField, opField, valueField, presetField, values)
				if err != nil {
					wl.app.ToastWarning(err.Error())
					return
				}
				wl.closeModal()
				if onSave != nil {
					onSave([]config.FilterClause{clause})
				}
			}).
			OnCancel(func() {
				if collapseOpenDropdowns(fields...) {
					return
				}
				wl.closeModal()
			}).
			Build()

		rawForm := components.NewFormBuilder().
			Text("raw", "Filter").
			Placeholder("CustomerId = 'abc'").
			Value(rawText).
			OnChange(func(event *components.ChangeEvent[string]) {
				if event != nil {
					rawText = event.NewValue
				}
			}).
			Done().
			OnSubmit(func(values map[string]any) {
				query := strings.TrimSpace(stringValue(values, "raw"))
				if query == "" {
					wl.app.ToastWarning("Filter is empty")
					return
				}
				clauses := filterClausesFromQuery(query)
				wl.closeModal()
				if onSave != nil {
					onSave(clauses)
				}
			}).
			OnCancel(func() {
				wl.closeModal()
			}).
			Build()

		testRaw := func() {
			wl.testVisibilityQuery(formString(rawForm, "raw"))
		}

		tabs := components.NewTabs().
			SetShowIcons(false).
			SetShowBadges(false).
			AddTab("Form", form).
			AddTab("Raw", rawForm).
			SetActive(activeTab).
			SetOnChange(func(index int, _ string) {
				activeTab = index
				if index == 1 {
					if q := compileFilterClauseFor(wl, current); q != "" {
						if field, ok := rawForm.GetTextField("raw"); ok && strings.TrimSpace(field.GetValue()) == "" {
							field.SetValue(q)
							rawText = q
						}
					}
				}
				applyHints()
				if wl.app != nil && wl.app.JigApp() != nil {
					if index == 1 {
						wl.app.JigApp().SetFocus(rawForm)
						return
					}
					wl.app.JigApp().SetFocus(form)
				}
			})

		if modal == nil {
			modal = newOverlayModal(components.ModalConfig{
				Title:  fmt.Sprintf("%s Clause", theme.IconFilter),
				Width:  72,
				Height: 24,
			}, wl)
			modal.SetOnCancel(func() {
				if collapseOpenDropdowns(fields...) {
					return
				}
				wl.closeModal()
			})
		}
		applyHints()
		modal.bindDropdowns(form, fields...)
		switchClauseTab := func(delta int) {
			next := activeTab + delta
			if next < 0 {
				next = 1
			}
			if next > 1 {
				next = 0
			}
			if next != activeTab {
				tabs.SetActive(next)
			}
		}
		clauseCapture := func(next func(*tcell.EventKey) *tcell.EventKey) func(*tcell.EventKey) *tcell.EventKey {
			return func(event *tcell.EventKey) *tcell.EventKey {
				if isClauseTabPrev(event) {
					switchClauseTab(-1)
					return nil
				}
				if isClauseTabNext(event) {
					switchClauseTab(1)
					return nil
				}
				if activeTab == 1 && isFilterTestKey(event) {
					testRaw()
					return nil
				}
				if next != nil {
					return next(event)
				}
				return event
			}
		}
		form.SetInputCapture(clauseCapture(dropdownFormCapture(fields...)))
		rawForm.SetInputCapture(clauseCapture(nil))
		tabs.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if ev := clauseCapture(nil)(event); ev == nil {
				return nil
			}
			if !isJigTabsNavKey(event) {
				return event
			}
			target := form
			if activeTab == 1 {
				target = rawForm
			}
			if capture := target.GetInputCapture(); capture != nil {
				if ev := capture(event); ev == nil {
					return nil
				}
			}
			if handler := target.InputHandler(); handler != nil {
				handler(event, func(tview.Primitive) {})
			}
			return nil
		})
		modal.SetContent(tabs)
		if !pushed {
			pushed = true
			wl.app.PushModal(modal)
		}
		if wl.app != nil && wl.app.JigApp() != nil {
			if activeTab == 1 {
				wl.app.JigApp().SetFocus(rawForm)
			} else {
				wl.app.JigApp().SetFocus(form)
			}
		}
	}
	rebuild(initial)
}

func compileFilterClauseFor(wl *WorkflowList, clause config.FilterClause) string {
	return compileFilterClauseWith(clause, resolveFilterKey(wl, clause.Key))
}

func isFilterTestKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if event.Key() == tcell.KeyCtrlT {
		return true
	}
	return event.Key() == tcell.KeyRune && (event.Rune() == 't' || event.Rune() == 'T') && event.Modifiers()&tcell.ModCtrl != 0
}

func isClauseTabPrev(event *tcell.EventKey) bool {
	if event == nil || event.Key() == tcell.KeyEscape {
		return false
	}
	if event.Key() == tcell.KeyCtrlLeftSq {
		return true
	}
	return event.Key() == tcell.KeyRune && event.Rune() == '[' && event.Modifiers()&tcell.ModCtrl != 0
}

func isClauseTabNext(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Key() {
	case tcell.KeyCtrlRightSq, tcell.KeyGS:
		return true
	}
	return event.Key() == tcell.KeyRune && event.Rune() == ']' && event.Modifiers()&tcell.ModCtrl != 0
}

func formString(form *components.Form, name string) string {
	if form == nil {
		return ""
	}
	if tf, ok := form.GetTextField(name); ok {
		return strings.TrimSpace(tf.GetValue())
	}
	if v, ok := form.GetField(name).(interface{ GetValue() string }); ok {
		return strings.TrimSpace(v.GetValue())
	}
	return ""
}

func (wl *WorkflowList) testVisibilityQuery(query string) {
	if wl == nil {
		return
	}
	query = strings.TrimSpace(query)
	if query == "" {
		if wl.app != nil {
			wl.app.ToastWarning("Filter is empty")
		}
		return
	}
	if _, err := resolveTimePlaceholders(query); err != nil {
		if wl.app != nil {
			wl.app.ToastError(err.Error())
		}
		return
	}
	wl.filterTestSavedName = wl.activeFilterName
	wl.filterTestSavedQuery = wl.visibilityQuery
	wl.filterTestSavedClauses = append([]config.FilterClause(nil), wl.filterClauses...)
	wl.filterTestSavedAdHoc = wl.adHoc.clone()
	wl.activeFilterName = ""
	wl.filterClauses = nil
	wl.filterText = ""
	wl.filterTestPending = true
	wl.applyVisibilityQuery(query)
	wl.revealActiveFilterChip()
}

func (wl *WorkflowList) restoreFilterTest() {
	if wl == nil {
		return
	}
	wl.filterTestPending = false
	wl.activeFilterName = wl.filterTestSavedName
	wl.visibilityQuery = wl.filterTestSavedQuery
	wl.filterClauses = append([]config.FilterClause(nil), wl.filterTestSavedClauses...)
	wl.adHoc = wl.filterTestSavedAdHoc.clone()
	wl.revealActiveFilterChip()
}

func (wl *WorkflowList) refreshFilterKeyOptions(keyField *dropdownField) {
	if wl == nil || keyField == nil {
		return
	}
	keyField.SetOptionsOrdered(filterKeyNamesFor(wl))
	if wl.app == nil {
		return
	}
	provider := wl.app.Provider()
	ns := wl.namespace
	if ns == "" {
		ns = wl.app.CurrentNamespace()
	}
	if provider == nil || ns == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		attrs, err := provider.ListCustomSearchAttributes(ctx, ns)
		if err != nil {
			if wl.app.catalog.reportAttrError(ns) {
				wl.app.ShowToastError("Custom search attributes: " + err.Error())
			}
			return
		}
		wl.app.catalog.putAttrs(ns, attrs)
		if jig := wl.app.JigApp(); jig != nil {
			jig.QueueUpdateDraw(func() {
				keyField.SetOptionsOrdered(filterKeyNamesFor(wl))
			})
		}
	}()
}
