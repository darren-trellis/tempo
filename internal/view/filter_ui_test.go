package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The digits number the chips on screen, so on the workflows list they pick a
// filter rather than switching the primary pane's tab.
func TestDigitsPickAFilterOnTheWorkflowsList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "Running", Query: "ExecutionStatus = 'Running'"},
		{Name: "Failed", Query: "ExecutionStatus = 'Failed'"},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '2', 0)) {
		t.Fatal("2 should be taken by the filter chips")
	}
	if wl.activeFilterName != "Running" {
		t.Fatalf("2 should apply the first saved filter, got %q", wl.activeFilterName)
	}
	if !wl.workflowsActive() {
		t.Fatal("a digit should not leave the workflows list")
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '3', 0)) {
		t.Fatal("3 should be taken by the filter chips")
	}
	if wl.activeFilterName != "Failed" {
		t.Fatalf("3 should apply the second saved filter, got %q", wl.activeFilterName)
	}

	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '1', 0)) {
		t.Fatal("1 should be taken by the filter chips")
	}
	if wl.activeFilterName != "" {
		t.Fatalf("1 should go back to All, got %q", wl.activeFilterName)
	}

	// Nothing is numbered 9, and the key must not fall through to the tabs.
	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '9', 0)) || !wl.workflowsActive() {
		t.Fatal("a digit past the last chip should be swallowed")
	}

	// The other lists have no chips, so there the digits still reach the tabs.
	wl.setListKind(listSchedules)
	if !wl.handleListTabKey(tcell.NewEventKey(tcell.KeyRune, '1', 0)) || !wl.workflowsActive() {
		t.Fatal("1 should return to workflows from another list")
	}
}

func TestFilterBarItemsAllVsNamed(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running"}}
	wl := &WorkflowList{app: &App{config: cfg}}
	items := filterBarItems(wl)
	if len(items) != 2 || items[0].label != "All" || !items[0].active || items[1].label != "Running" || items[1].active {
		t.Fatalf("idle chips=%+v", items)
	}

	wl.activeFilterName = "Running"
	wl.visibilityQuery = "ExecutionStatus = 'Running'"
	items = filterBarItems(wl)
	if items[0].active || !items[1].active {
		t.Fatalf("named filter should be active, got %+v", items)
	}

	wl.activeFilterName = ""
	items = filterBarItems(wl)
	if items[0].active || items[1].active {
		t.Fatalf("ad-hoc query should highlight no chip, got %+v", items)
	}
}

func TestFilterBarContentWidth(t *testing.T) {
	items := []filterBarChip{
		{label: "All"},
		{label: "Running Workflows"},
		{label: "Failed Workflows"},
		{label: "Completed Workflows"},
	}
	if got := filterBarContentWidth(items); got != 5+1+19+1+18+1+21 {
		t.Fatalf("width=%d", got)
	}
}

func TestFilterBarScrollsHorizontally(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "Running Workflows"},
		{Name: "Failed Workflows"},
		{Name: "Completed Workflows"},
	}
	wl := NewWorkflowList(&App{config: cfg}, "default")
	wl.filterBar.SetRect(0, 0, 24, 2)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(24, 2)
	wl.filterBar.Draw(screen)
	top := rowText(screen, 0, 24)
	if !strings.Contains(top, "All") {
		t.Fatalf("start=%q", top)
	}
	if !strings.Contains(top, " | ") {
		t.Fatalf("top chips should use a pipe separator, got %q", top)
	}
	if strings.Contains(top, "  |") || strings.Contains(top, "|  ") {
		t.Fatalf("pipe spacer should be a single space, got %q", top)
	}
	if strings.Contains(top, "Completed") {
		t.Fatalf("narrow bar should not show the last chip yet, got %q", top)
	}

	handler := wl.filterBar.MouseHandler()
	event := tcell.NewEventMouse(0, 0, tcell.WheelDown, tcell.ModNone)
	for i := 0; i < 40; i++ {
		handler(tview.MouseScrollDown, event, func(tview.Primitive) {})
	}
	if wl.filterBar.hOffset <= 0 {
		t.Fatal("wheel should scroll the chips")
	}
	screen.Clear()
	wl.filterBar.Draw(screen)
	top = rowText(screen, 0, 24)
	if strings.Contains(top, "+") {
		t.Fatalf("overflow +N should be gone, got %q", top)
	}
	if !strings.Contains(top, "Completed") {
		t.Fatalf("scrolled bar should show later chips, got %q", top)
	}
	divider := rowText(screen, 1, 24)
	if strings.ContainsAny(divider, "▁") {
		t.Fatalf("filter divider should stay centered, got %q", divider)
	}
	if !strings.Contains(divider, "─") {
		t.Fatalf("filter divider=%q", divider)
	}

	wl.filterBar.scrollHoriz(-1000)
	if wl.filterBar.hOffset != 0 {
		t.Fatalf("left clamp=%d", wl.filterBar.hOffset)
	}
}

func TestLayoutFilterBarWrap(t *testing.T) {
	items := []filterBarChip{
		{label: "All"},
		{label: "Running Workflows"},
		{label: "Failed Workflows"},
	}
	lines := layoutFilterBarLines(items, 20)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped rows, got %d", len(lines))
	}
	var n int
	for _, line := range lines {
		n += len(line)
	}
	if n != len(items) {
		t.Fatalf("wrap dropped chips: lines=%v", lines)
	}
	wide := layoutFilterBarLines(items, 80)
	if len(wide) != 1 || len(wide[0]) != 3 {
		t.Fatalf("wide wrap=%v", wide)
	}
}

func TestFilterBarHeightWraps(t *testing.T) {
	on := true
	wl := NewWorkflowList(&App{config: &config.Config{
		FilterWrap: &on,
		SavedFilters: []config.SavedFilter{
			{Name: "Running Workflows"},
			{Name: "Failed Workflows"},
			{Name: "Completed Workflows"},
		},
	}}, "default")
	wl.filterBar.SetRect(0, 0, 20, 1)
	if h := wl.filterBarHeight(); h < 3 {
		t.Fatalf("wrap height=%d", h)
	}
}

func TestFilterBarDrawsDivider(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if wl.filterBarHeight() != 2 {
		t.Fatalf("default height=%d", wl.filterBarHeight())
	}
	wl.filterBar.SetRect(0, 0, 40, 2)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 2)
	wl.filterBar.Draw(screen)
	if !strings.Contains(rowText(screen, 0, 40), "All") {
		t.Fatalf("chips=%q", rowText(screen, 0, 40))
	}
	if !strings.Contains(rowText(screen, 1, 40), "─") {
		t.Fatalf("divider=%q", rowText(screen, 1, 40))
	}
}

func TestFilterSidebarLayoutAndFocus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFiltersPosition = config.SavedFiltersPositionSide
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running"}, {Name: "Failed"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	if !wl.filtersOnSide() {
		t.Fatal("side position should use the sidebar")
	}
	if !wl.filterBarSide {
		t.Fatal("workflow stack should mount as a column")
	}

	wl.focusPane = focusWorkflows
	wl.cycleFocus(-1)
	if wl.focusPane != focusFilters {
		t.Fatalf("shift-tab from the list should focus filters, got %d", wl.focusPane)
	}
	wl.cycleFocus(1)
	if wl.focusPane != focusWorkflows {
		t.Fatalf("tab from filters should return to the list, got %d", wl.focusPane)
	}

	wl.filterBar.SetRect(0, 0, 20, 8)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(20, 8)
	wl.filterBar.Draw(screen)
	if !strings.Contains(rowText(screen, 0, 20), "All") {
		t.Fatalf("row0=%q", rowText(screen, 0, 20))
	}
	if !strings.Contains(rowText(screen, 1, 20), "Running") {
		t.Fatalf("row1=%q", rowText(screen, 1, 20))
	}
	if !strings.Contains(rowText(screen, 2, 20), "Failed") {
		t.Fatalf("row2=%q", rowText(screen, 2, 20))
	}
	if !strings.Contains(rowText(screen, 0, 20), "│") {
		t.Fatalf("sidebar should draw a vertical divider, got %q", rowText(screen, 0, 20))
	}

	wl.SetRect(0, 0, 80, 16)
	screen.Clear()
	screen.SetSize(80, 16)
	wl.Draw(screen)
	fx, _, fw, _ := wl.filterBar.GetRect()
	tx, _, _, _ := wl.tableScroll.GetRect()
	if fw <= 0 || fx >= tx {
		t.Fatalf("sidebar should sit left of the table, filter x=%d w=%d table x=%d", fx, fw, tx)
	}
}

func TestFilterSidebarEscapeReturnsToList(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.SavedFiltersPosition = config.SavedFiltersPositionSide
	wl := NewWorkflowList(&App{config: cfg}, "default")
	wl.focusPane = focusFilters
	if !wl.HandleEscape() || wl.focusPane != focusWorkflows {
		t.Fatalf("escape should leave the sidebar, pane=%d", wl.focusPane)
	}
}

func screenHasAllAndRunning(screen tcell.SimulationScreen, height, width int) bool {
	for y := 0; y < height; y++ {
		row := rowText(screen, y, width)
		if strings.Contains(row, "All") && strings.Contains(row, "Running") {
			return true
		}
	}
	return false
}

func TestFilterBarDrawsOnWorkflowsOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	wl.SetRect(0, 0, 80, 16)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 16)
	wl.Draw(screen)
	if !screenHasAllAndRunning(screen, 16, 80) {
		t.Fatal("workflow tab should draw filter chips under the tab bar")
	}

	wl.setListKind(listTaskQueues)
	screen.Clear()
	wl.Draw(screen)
	if screenHasAllAndRunning(screen, 16, 80) {
		t.Fatal("other tabs should not draw workflow filter chips")
	}
}

func TestFilterValueWidgetKind(t *testing.T) {
	cases := map[string]filterKeyKind{
		"WorkflowId":      filterKeyText,
		"RunId":           filterKeyText,
		"WorkflowType":    filterKeyCatalog,
		"TaskQueue":       filterKeyCatalog,
		"ExecutionStatus": filterKeyStatus,
		"StartTime":       filterKeyTime,
		"CloseTime":       filterKeyTime,
	}
	for key, want := range cases {
		spec, ok := lookupFilterKey(key)
		if !ok || spec.kind != want {
			t.Fatalf("%s kind=%v ok=%v", key, spec.kind, ok)
		}
	}
}

func TestFilterKeyNamesIncludeCustomSearchAttributes(t *testing.T) {
	a := &App{}
	a.catalog.putAttrs("default", []temporal.SearchAttribute{
		{Name: "CustomerId", Type: temporal.SearchAttributeKeyword},
		{Name: "Amount", Type: temporal.SearchAttributeInt},
		{Name: "ClosedAt", Type: temporal.SearchAttributeDatetime},
	})
	wl := &WorkflowList{app: a, namespace: "default"}
	names := filterKeyNamesFor(wl)
	if !containsString(names, "CustomerId") || !containsString(names, "Amount") || !containsString(names, "ClosedAt") {
		t.Fatalf("custom keys missing: %v", names)
	}
	if names[0] != "WorkflowId" {
		t.Fatalf("builtins should stay first, got %q", names[0])
	}
	spec := resolveFilterKey(wl, "Amount")
	if spec.kind != filterKeyNumber {
		t.Fatalf("Amount kind=%v", spec.kind)
	}
	spec = resolveFilterKey(wl, "ClosedAt")
	if spec.kind != filterKeyTime {
		t.Fatalf("ClosedAt kind=%v", spec.kind)
	}
	got := compileFilterClausesFor(wl, []config.FilterClause{{Key: "Amount", Op: filterOpEq, Value: "42"}})
	if got != "Amount = 42" {
		t.Fatalf("int compile=%q", got)
	}
}

func TestCompileRawFilterClause(t *testing.T) {
	got := compileFilterClause(config.FilterClause{Key: "raw", Op: filterOpRaw, Value: "CustomerId = 'abc'"})
	if got != "CustomerId = 'abc'" {
		t.Fatalf("raw compile=%q", got)
	}
}

func TestDropdownExactValueShowsAllOptions(t *testing.T) {
	opts := []string{"WorkflowId", "CustomerId", "StartTime"}
	got := filterDropdownOptions(opts, "Work")
	if len(got) == 0 || got[0] != "WorkflowId" {
		t.Fatalf("prefix should still match keys, got %v", got)
	}
}

func TestFilterKeyDropdownShowsCustomSearchAttributes(t *testing.T) {
	names := append(filterKeyNames(), "AssetNames", "CustomerId", "EntityId", "EventIds", "ProjectId", "RowId", "TransformId")
	field := newOrderedDropdownField("key", "Key", names)
	field.openList()
	start, end := field.visibleMatches()
	window := field.matches[start:end]
	if !containsString(window, "CustomerId") {
		t.Fatalf("CustomerId should be visible without scrolling, window=%v", window)
	}
}

func TestClauseEditorHasFormAndRawTabs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showClauseEditor(config.FilterClause{Key: "WorkflowId", Op: filterOpEq}, nil)
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	om.SetRect(0, 0, 80, 24)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	om.Draw(screen)
	foundForm, foundRaw := false, false
	for y := 0; y < 24; y++ {
		line := rowText(screen, y, 80)
		if strings.Contains(line, "Form") {
			foundForm = true
		}
		if strings.Contains(line, "Raw") {
			foundRaw = true
		}
	}
	if !foundForm || !foundRaw {
		t.Fatal("clause editor should show Form and Raw tabs")
	}
	if hintDescription(a.menu.GetHints(), "Ctrl+T") != "Test" {
		t.Fatalf("Ctrl+T should be on the status bar, got %+v", a.menu.GetHints())
	}
	if hintDescription(a.menu.GetHints(), "Ctrl+[") != "Prev" || hintDescription(a.menu.GetHints(), "Ctrl+]") != "Next" {
		t.Fatalf("tab switch hints should be on the status bar, got %+v", a.menu.GetHints())
	}
	if hintDescription(a.menu.GetHints(), "Enter") != "Save" || hintDescription(a.menu.GetHints(), "Esc") != "Cancel" {
		t.Fatalf("save/cancel should stay on the status bar, got %+v", a.menu.GetHints())
	}
	if hintDescription(a.menu.GetHints(), "t") != "" {
		t.Fatal("bare t should not be advertised")
	}
}

func TestTestVisibilityQueryClearsActiveFilter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.activeFilterName = "Running"
	wl.filterClauses = []config.FilterClause{{Key: "ExecutionStatus", Op: filterOpEq, Value: "Running"}}
	wl.testVisibilityQuery("CustomerId = 'abc'")
	if wl.activeFilterName != "" {
		t.Fatalf("test should unselect the saved filter, got %q", wl.activeFilterName)
	}
	if wl.visibilityQuery != "CustomerId = 'abc'" {
		t.Fatalf("query=%q", wl.visibilityQuery)
	}
}

func TestFilterTestKey(t *testing.T) {
	if !isFilterTestKey(tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModCtrl)) {
		t.Fatal("Ctrl+T should test the raw filter")
	}
	if !isFilterTestKey(tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModCtrl)) {
		t.Fatal("Ctrl+T as a rune should test the raw filter")
	}
	if isFilterTestKey(tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModNone)) {
		t.Fatal("plain t should type into the filter")
	}
}

func TestClauseTabKeys(t *testing.T) {
	if isClauseTabPrev(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) {
		t.Fatal("Esc should still cancel")
	}
	if !isClauseTabPrev(tcell.NewEventKey(tcell.KeyCtrlLeftSq, 0, tcell.ModCtrl)) {
		t.Fatal("Ctrl+[ should switch to the previous tab")
	}
	if !isClauseTabNext(tcell.NewEventKey(tcell.KeyCtrlRightSq, 0, tcell.ModCtrl)) {
		t.Fatal("Ctrl+] should switch to the next tab")
	}
	if !isClauseTabNext(tcell.NewEventKey(tcell.KeyGS, ']', tcell.ModCtrl)) {
		t.Fatal("Ctrl+] from the terminal should switch tabs")
	}
	if isClauseTabNext(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) || isClauseTabPrev(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) {
		t.Fatal("Tab should move between fields, not Form/Raw")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestFilterBuilderHasRawTab(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	query := `WorkflowType = 'Order' AND ExecutionStatus = 'Running'`
	wl.openFilterBuilder(&filterBuilderState{wl: wl, name: "Orders", clauses: filterClausesFromQuery(query)})
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	tabs, ok := om.body.(*components.Tabs)
	if !ok {
		t.Fatalf("builder body=%T, want tabs", om.body)
	}
	screen := drawCurrentModal(t, a)
	if !strings.Contains(screen, "Clauses") || !strings.Contains(screen, "Raw") {
		t.Fatalf("builder should show Clauses and Raw tabs, got:\n%s", screen)
	}
	if hintDescription(om.Hints(), "Ctrl+E") != "Open in Editor" {
		t.Fatalf("Ctrl+E should be advertised, got %+v", om.Hints())
	}

	tabs.SetActive(1)
	if got := tabs.GetActiveTab().Content.(*tview.TextArea).GetText(); got != query {
		t.Fatalf("raw tab = %q, want the full query", got)
	}
	if hintDescription(om.Hints(), "n") != "" || hintDescription(om.Hints(), "e") != "" {
		t.Fatalf("clause keys should hide on the raw tab, got %+v", om.Hints())
	}

	orig := editFilterQueryInEditor
	t.Cleanup(func() { editFilterQueryInEditor = orig })
	editFilterQueryInEditor = func(_ *App, got string) (string, bool) {
		if got != query {
			t.Fatalf("editor should receive the compiled query, got %q", got)
		}
		return `CustomerId = 'acme' AND ExecutionStatus = 'Failed'`, true
	}
	if ev := tabs.GetInputCapture()(tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)); ev != nil {
		t.Fatal("Ctrl+E should open the editor")
	}
	want := `CustomerId = 'acme' AND ExecutionStatus = 'Failed'`
	if got := tabs.GetActiveTab().Content.(*tview.TextArea).GetText(); got != want {
		t.Fatalf("raw tab should refresh from the editor, got %q", got)
	}
	tabs.SetActive(0)
	if overlayModalTable(t, om).GetDataRowCount() != 2 {
		t.Fatalf("clauses should update from the editor, rows=%d", overlayModalTable(t, om).GetDataRowCount())
	}
}

func TestFilterEditKey(t *testing.T) {
	if !isFilterEditKey(tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModCtrl)) {
		t.Fatal("Ctrl+E should open the editor")
	}
	if !isFilterEditKey(tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModCtrl)) {
		t.Fatal("Ctrl+E as a rune should open the editor")
	}
	if isFilterEditKey(tcell.NewEventKey(tcell.KeyRune, 'e', tcell.ModNone)) {
		t.Fatal("plain e should still edit a clause")
	}
}

func TestShowFilterBuilderOpensModal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.openFilterBuilder(&filterBuilderState{wl: wl})
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("new filter should open the builder")
	}
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	om.SetRect(0, 0, 80, 24)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	om.Draw(screen)
	found := false
	for y := 0; y < 24; y++ {
		if strings.Contains(rowText(screen, y, 80), "New Filter") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("builder title should be New Filter")
	}
}

func TestShowFilterManagerReorderAndDelete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterManager()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	table := overlayModalTable(t, om)
	table.SelectRow(0)
	capture := table.GetInputCapture()
	if capture == nil {
		t.Fatal("manager table should have keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'J', tcell.ModNone)); ev != nil {
		t.Fatal("J should reorder")
	}
	if cfg.SavedFilters[0].Name != "b" || cfg.SavedFilters[1].Name != "a" {
		t.Fatalf("reorder %+v", cfg.SavedFilters)
	}
	table.SelectRow(2)
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone)); ev != nil {
		t.Fatal("d should delete")
	}
	if len(cfg.SavedFilters) != 2 || cfg.SavedFilters[1].Name != "a" {
		t.Fatalf("delete %+v", cfg.SavedFilters)
	}
}

func TestDeleteActiveFilterKeepsManagerFocus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Running"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.activeFilterName = "Running"
	wl.visibilityQuery = "ExecutionStatus = 'Running'"
	a.app.Pages().Push(wl)
	wl.showFilterManager()
	if !a.modalHasFocus() {
		t.Fatal("manager should be the current modal")
	}
	if wl.shouldFocusWorkflowTable() {
		t.Fatal("a modal should keep the workflow table from taking focus")
	}
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	table := overlayModalTable(t, om)
	table.SelectRow(0)
	capture := table.GetInputCapture()
	if capture == nil {
		t.Fatal("manager table should have keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'd', tcell.ModNone)); ev != nil {
		t.Fatal("d should delete")
	}
	if wl.activeFilterName != "" {
		t.Fatalf("active filter should clear, got %q", wl.activeFilterName)
	}
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("Filters modal should stay open")
	}
	if wl.shouldFocusWorkflowTable() {
		t.Fatal("reloading after delete should not focus the workflow table")
	}
}

func TestFilterManagerOpensOnTheActiveFilter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	for _, name := range []string{"Running", "Failed", "Orders"} {
		a.Config().SaveFilterFor("local", config.SavedFilter{Name: name, Query: "WorkflowType = '" + name + "'"})
	}
	filters := a.Config().SavedFiltersFor("local")
	wl.applySavedFilter(filters[2])

	wl.showFilterManager()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	if got := overlayModalTable(t, om).SelectedRow(); got != 4 {
		t.Fatalf("the active filter's row, below two filters of one clause each, should be selected, got row %d", got)
	}
	wl.closeModal()

	wl.applyAllWorkflowsFilter()
	wl.showFilterManager()
	om = a.app.Pages().Current().(*overlayModal)
	if got := overlayModalTable(t, om).SelectedRow(); got != 0 {
		t.Fatalf("with no saved filter active the first row should be selected, got %d", got)
	}
}

func TestFilterManagerShowsBracketsInNamesAndQueries(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	a.Config().SaveFilterFor("local", config.SavedFilter{Name: "Prod [EU]", Query: "WorkflowId = '[red]x'"})

	wl.showFilterManager()
	for _, want := range []string{"Prod [EU]", "[red]x"} {
		if got := drawCurrentModal(t, a); !strings.Contains(got, want) {
			t.Fatalf("filters modal should show %q, got:\n%s", want, got)
		}
	}
	wl.closeModal()

	wl.openFilterBuilder(&filterBuilderState{wl: wl, clauses: savedFilterClauses(a.Config().SavedFiltersFor("local")[0])})
	if got := drawCurrentModal(t, a); !strings.Contains(got, "[red]x") {
		t.Fatalf("filter builder should show the bracketed value, got:\n%s", got)
	}
}

func drawCurrentModal(t *testing.T, a *App) string {
	t.Helper()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	om.SetRect(0, 0, 120, 30)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 30)
	om.Draw(screen)
	var body strings.Builder
	for y := 0; y < 30; y++ {
		body.WriteString(rowText(screen, y, 120))
		body.WriteByte('\n')
	}
	return body.String()
}

func TestFilterManagerHintIsNewFilter(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterManager()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	if hintDescription(om.Hints(), "n") != "New Filter" {
		t.Fatalf("n hint=%q", hintDescription(om.Hints(), "n"))
	}
}

func TestSearchPromptPlaceholder(t *testing.T) {
	a := &App{}
	a.ShowFilterMode("", FilterModeCallbacks{})
	p := a.prompt()
	p.input.SetRect(0, 0, 40, 1)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(40, 1)
	p.input.Draw(screen)
	got := rowText(screen, 0, 40)
	if !strings.Contains(got, "Search") {
		t.Fatalf("placeholder=%q", got)
	}
}

func TestFilterManagerRefreshesAfterSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "existing"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterManager()

	managerTable := func() *components.Table {
		t.Helper()
		om, ok := a.app.Pages().Current().(*overlayModal)
		if !ok {
			t.Fatalf("current=%T", a.app.Pages().Current())
		}
		return overlayModalTable(t, om)
	}

	table := managerTable()
	before := table.RowCount()

	capture := table.GetInputCapture()
	if capture == nil {
		t.Fatal("manager table should have keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone)); ev != nil {
		t.Fatal("n should be handled")
	}

	// n opens the clause editor; saving a clause hands off to the builder.
	editor := a.app.Pages().Current().(*overlayModal)
	clauseForm, ok := editor.body.(*components.Tabs).GetActiveTab().Content.(*components.Form)
	if !ok {
		t.Fatalf("clause editor body=%T", editor.body)
	}
	if field, found := clauseForm.GetTextField("value"); found {
		field.SetValue("order-42")
	} else {
		t.Fatal("clause editor should have a value field")
	}
	clauseForm.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})

	builderTable := overlayModalTable(t, a.app.Pages().Current().(*overlayModal))
	builderKeys := builderTable.GetInputCapture()
	if builderKeys == nil {
		t.Fatal("builder table should have keys")
	}
	if ev := builderKeys(tcell.NewEventKey(tcell.KeyRune, 's', tcell.ModNone)); ev != nil {
		t.Fatal("s should open the name prompt")
	}

	prompt, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("prompt current=%T", a.app.Pages().Current())
	}
	form, ok := prompt.body.(*components.Form)
	if !ok {
		t.Fatalf("prompt body=%T", prompt.body)
	}
	nameField, ok := form.GetTextField("name")
	if !ok {
		t.Fatal("name prompt should have a name field")
	}
	nameField.SetValue("added")
	form.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})

	if len(cfg.SavedFiltersFor("local")) != 2 {
		t.Fatalf("filter was not saved: %+v", cfg.SavedFiltersFor("local"))
	}

	wl.closeModal()
	if got := managerTable().RowCount(); got <= before {
		t.Fatalf("manager still shows the stale list: %d rows, want more than %d", got, before)
	}
}

// overlayModalTable unwraps a modal body that may be a table or a scroll
// wrapper around one.
func overlayModalTable(t *testing.T, om *overlayModal) *components.Table {
	t.Helper()
	switch body := om.body.(type) {
	case *components.Table:
		return body
	case *charScrollView:
		table, ok := body.content.(*components.Table)
		if !ok {
			t.Fatalf("scroll content=%T", body.content)
		}
		return table
	case *components.Tabs:
		table, ok := body.GetActiveTab().Content.(*components.Table)
		if !ok {
			t.Fatalf("tab content=%T", body.GetActiveTab().Content)
		}
		return table
	default:
		t.Fatalf("content=%T", om.body)
		return nil
	}
}

func overlayModalScroll(t *testing.T, om *overlayModal) *charScrollView {
	t.Helper()
	scroll, ok := om.body.(*charScrollView)
	if !ok {
		t.Fatalf("content=%T, want a scroll view", om.body)
	}
	return scroll
}

// filterManagerTable opens the Filters dialog and hands back its table.
func filterManagerTable(t *testing.T, a *App, wl *WorkflowList) *components.Table {
	t.Helper()
	wl.showFilterManager()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	return overlayModalTable(t, om)
}

// submitNamePrompt fills in the open name prompt and accepts it.
func submitNamePrompt(t *testing.T, a *App, name string) {
	t.Helper()
	prompt, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("prompt current=%T", a.app.Pages().Current())
	}
	form, ok := prompt.body.(*components.Form)
	if !ok {
		t.Fatalf("prompt body=%T", prompt.body)
	}
	field, ok := form.GetTextField("name")
	if !ok {
		t.Fatal("name prompt should have a name field")
	}
	field.SetValue(name)
	form.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
}

func TestCloneFilterCopiesQueryBelowTheOriginal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "first"},
		{Name: "recent", Query: "StartTime > '$HOURS_AGO_24'", IsDefault: true},
		{Name: "last"},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	table.SelectRow(1)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)); ev != nil {
		t.Fatal("c should clone the selected filter")
	}
	submitNamePrompt(t, a, "recent (copy)")

	filters := cfg.SavedFiltersFor("local")
	if len(filters) != 4 {
		t.Fatalf("clone should add one filter, got %+v", filters)
	}
	clone := filters[2]
	if clone.Name != "recent (copy)" {
		t.Fatalf("clone should sit right below its original, got %+v", filters)
	}
	if clone.Query != "StartTime > '$HOURS_AGO_24'" {
		t.Fatalf("clone should carry the query over verbatim, got %q", clone.Query)
	}
	if clone.IsDefault {
		t.Fatal("a clone should not inherit the default flag")
	}
	if !filters[1].IsDefault {
		t.Fatal("cloning should leave the original alone")
	}
}

func TestCloneFilterSuggestsAFreeName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "recent"}, {Name: "recent (copy)"}}

	if got := copyFilterName(cfg, "local", "recent"); got != "recent (copy 2)" {
		t.Fatalf("a taken name should count up, got %q", got)
	}
	if got := copyFilterName(cfg, "local", "other"); got != "other (copy)" {
		t.Fatalf("a free name should be used as is, got %q", got)
	}

	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	table.SelectRow(0)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)); ev != nil {
		t.Fatal("c should clone the selected filter")
	}
	prompt := a.app.Pages().Current().(*overlayModal)
	form, ok := prompt.body.(*components.Form)
	if !ok {
		t.Fatalf("prompt body=%T", prompt.body)
	}
	field, _ := form.GetTextField("name")
	if got := field.GetValue(); got != "recent (copy 2)" {
		t.Fatalf("prompt should suggest a free name, got %q", got)
	}
}

func TestCloneFilterRefusesToOverwriteAnExistingName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "recent", Query: "StartTime > '$HOURS_AGO_24'"},
		{Name: "failures", Query: "ExecutionStatus = 'Failed'"},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	table.SelectRow(0)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)); ev != nil {
		t.Fatal("c should clone the selected filter")
	}
	submitNamePrompt(t, a, "failures")

	filters := cfg.SavedFiltersFor("local")
	if len(filters) != 2 {
		t.Fatalf("clone should not have been saved, got %+v", filters)
	}
	if filters[1].Query != "ExecutionStatus = 'Failed'" {
		t.Fatalf("the existing filter should be untouched, got %q", filters[1].Query)
	}
}

func TestCloneFilterWithNothingSelectedDoesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = nil
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)); ev != nil {
		t.Fatal("c should be handled even with an empty list")
	}
	if _, isPrompt := a.app.Pages().Current().(*overlayModal).body.(*components.Form); isPrompt {
		t.Fatal("an empty list has nothing to clone, so no prompt should open")
	}
	if len(cfg.SavedFiltersFor("local")) != 0 {
		t.Fatalf("nothing should have been saved, got %+v", cfg.SavedFiltersFor("local"))
	}
}

func TestRenameFilterKeepsQueryDefaultAndActiveChip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "first"},
		{Name: "recent", Query: "StartTime > '$HOURS_AGO_24'", IsDefault: true},
		{Name: "last"},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.activeFilterName = "recent"
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	table.SelectRow(1)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone)); ev != nil {
		t.Fatal("r should rename the selected filter")
	}
	prompt := a.app.Pages().Current().(*overlayModal)
	form, ok := prompt.body.(*components.Form)
	if !ok {
		t.Fatalf("prompt body=%T", prompt.body)
	}
	field, _ := form.GetTextField("name")
	if got := field.GetValue(); got != "recent" {
		t.Fatalf("prompt should start from the current name, got %q", got)
	}
	submitNamePrompt(t, a, "today")

	filters := cfg.SavedFiltersFor("local")
	if len(filters) != 3 {
		t.Fatalf("rename should not add a filter, got %+v", filters)
	}
	got := filters[1]
	if got.Name != "today" || got.Query != "StartTime > '$HOURS_AGO_24'" || !got.IsDefault {
		t.Fatalf("rename should keep query, default, and position, got %+v", filters)
	}
	if wl.activeFilterName != "today" {
		t.Fatalf("the active chip should follow the new name, got %q", wl.activeFilterName)
	}
}

func TestRenameFilterRefusesToOverwriteAnExistingName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{
		{Name: "recent", Query: "StartTime > '$HOURS_AGO_24'", IsDefault: true},
		{Name: "failures", Query: "ExecutionStatus = 'Failed'"},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	table.SelectRow(0)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone)); ev != nil {
		t.Fatal("r should rename the selected filter")
	}
	submitNamePrompt(t, a, "failures")

	filters := cfg.SavedFiltersFor("local")
	if filters[0].Name != "recent" || !filters[0].IsDefault || filters[1].Query != "ExecutionStatus = 'Failed'" {
		t.Fatalf("a refused rename should not change anything, got %+v", filters)
	}
}

func TestRenameFilterWithNothingSelectedDoesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = nil
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	table := filterManagerTable(t, a, wl)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone)); ev != nil {
		t.Fatal("r should be handled even with an empty list")
	}
	if _, isPrompt := a.app.Pages().Current().(*overlayModal).body.(*components.Form); isPrompt {
		t.Fatal("an empty list has nothing to rename, so no prompt should open")
	}
}

func TestFilterManagerShowsFullQueryAndScrolls(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	longQuery := "WorkflowType = 'VeryLongWorkflowTypeNameThatShouldNotBeTruncatedEvenWhenItIsWiderThanTheDialog' AND ExecutionStatus = 'Running' AND WorkflowId STARTS_WITH 'order-'"
	cfg := config.DefaultConfig()
	cfg.SavedFilters = nil
	for i := 0; i < 40; i++ {
		cfg.SavedFilters = append(cfg.SavedFilters, config.SavedFilter{
			Name:  fmt.Sprintf("filter-%02d", i),
			Query: longQuery,
		})
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	wl.showFilterManager()
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	table := overlayModalTable(t, om)
	scroll := overlayModalScroll(t, om)

	row := table.GetRowData(1)
	if len(row) < 1 || !strings.Contains(row[0], "VeryLongWorkflowTypeNameThatShouldNotBeTruncatedEvenWhenItIsWiderThanTheDialog") {
		t.Fatalf("clause rows should keep the full clause, got %q", row)
	}
	if strings.Contains(row[0], "...") {
		t.Fatalf("clause rows should not truncate, got %q", row[0])
	}

	om.SetRect(0, 0, 80, 24)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	om.Draw(screen)
	if tableContentWidth(table) <= scroll.viewport() {
		t.Fatalf("a long query should be wider than the dialog, content=%d viewport=%d", tableContentWidth(table), scroll.viewport())
	}

	capture := table.GetInputCapture()
	if ev := capture(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)); ev != nil {
		t.Fatal("right should scroll horizontally")
	}
	om.Draw(screen)
	if scroll.offset == 0 {
		t.Fatal("right should move the horizontal offset")
	}

	table.SelectRow(39)
	om.Draw(screen)
	if rowOff, _ := table.GetOffset(); rowOff == 0 {
		t.Fatal("selecting the last row should scroll the table vertically")
	}
	if !filterManagerHasVerticalScrollbar(screen, 80, 24) {
		t.Fatal("a long list should show a vertical scrollbar")
	}
}

func filterManagerHasVerticalScrollbar(screen tcell.SimulationScreen, width, height int) bool {
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			mainc, _, _, _ := screen.GetContent(x, y)
			if mainc == scrollbarThinVert {
				return true
			}
		}
	}
	return false
}

func clauseEditorScreen(t *testing.T, a *App, wl *WorkflowList, clause config.FilterClause) tcell.SimulationScreen {
	t.Helper()
	wl.showClauseEditor(clause, nil)
	om, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	om.SetRect(0, 0, 80, 24)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	om.Draw(screen)
	return screen
}

func screenContains(screen tcell.SimulationScreen, want string) bool {
	for y := 0; y < 24; y++ {
		if strings.Contains(rowText(screen, y, 80), want) {
			return true
		}
	}
	return false
}

func TestClauseEditorHidesValueForNullOperators(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	withValue := clauseEditorScreen(t, a, wl, config.FilterClause{Key: "CloseTime", Op: filterOpAfter})
	if !screenContains(withValue, "Value") {
		t.Fatal("an operator that takes a value should show the Value field")
	}
	wl.closeModal()

	for _, op := range []string{filterOpIsNull, filterOpIsNotNull} {
		screen := clauseEditorScreen(t, a, wl, config.FilterClause{Key: "CloseTime", Op: op})
		if screenContains(screen, "Value") {
			t.Errorf("%s should hide the Value field", filterOpLabel(op))
		}
		if !screenContains(screen, "Operator") {
			t.Errorf("%s should still show the Operator field", filterOpLabel(op))
		}
		wl.closeModal()
	}
}

func TestNewFilterOpensClauseEditorFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "existing"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterManager()

	manager, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("manager current=%T", a.app.Pages().Current())
	}
	table := overlayModalTable(t, manager)
	capture := table.GetInputCapture()
	if capture == nil {
		t.Fatal("manager table should have keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone)); ev != nil {
		t.Fatal("n should be handled")
	}

	// n lands on the clause editor, not on the builder.
	editor, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("editor current=%T", a.app.Pages().Current())
	}
	if _, isTable := editor.body.(*components.Table); isTable {
		t.Fatal("n should open the clause editor, not the filter builder")
	}
	tabs, ok := editor.body.(*components.Tabs)
	if !ok {
		t.Fatalf("clause editor body=%T, want tabs", editor.body)
	}
	form, ok := tabs.GetActiveTab().Content.(*components.Form)
	if !ok {
		t.Fatalf("clause editor tab body=%T", tabs.GetActiveTab().Content)
	}

	if field, found := form.GetTextField("value"); found {
		field.SetValue("order-42")
	} else {
		t.Fatal("clause editor should have a value field")
	}
	form.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})

	// Saving the clause hands off to the builder, carrying the clause.
	builder, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("builder current=%T", a.app.Pages().Current())
	}
	builderTable := overlayModalTable(t, builder)
	if builderTable.GetDataRowCount() != 1 {
		t.Fatalf("builder should show the clause just entered, got %d rows", builderTable.GetDataRowCount())
	}
	if got := builderTable.GetCell(1, 0).Text; got != "WorkflowId" {
		t.Errorf("clause key = %q, want WorkflowId", got)
	}
	if got := builderTable.GetCell(1, 2).Text; !strings.Contains(got, "order-42") {
		t.Errorf("clause value = %q, want the value just entered", got)
	}
}

func TestNewFilterCancelledClauseReturnsToManager(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "existing"}}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterManager()

	manager := a.app.Pages().Current().(*overlayModal)
	overlayModalTable(t, manager).GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone))

	wl.closeModal()

	back, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("current=%T", a.app.Pages().Current())
	}
	if _, isScroll := back.body.(*charScrollView); !isScroll {
		t.Fatalf("cancelling the clause should land back on the Filters modal, got %T", back.body)
	}
}

func TestFilterManagerIsScopedToTheActiveProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.SavedFilters = []config.SavedFilter{{Name: "Shared", Query: "ExecutionStatus = 'Running'"}}
	cfg.ProfileFilters = map[string][]config.SavedFilter{
		"local": {{Name: "Mine", Query: "WorkflowType = 'A'"}},
		"prod":  {{Name: "Theirs", Query: "WorkflowType = 'B'"}},
	}
	a := NewAppWithProvider(nil, "default", cfg, "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	var chips []string
	for _, c := range filterBarItems(wl) {
		chips = append(chips, c.label)
	}
	if got := strings.Join(chips, ","); strings.Contains(got, "Theirs") || !strings.Contains(got, "Mine") || !strings.Contains(got, "Shared") {
		t.Fatalf("chips should be the profile's own plus global filters, got %s", got)
	}

	table := filterManagerTable(t, a, wl)
	got := drawCurrentModal(t, a)
	if strings.Contains(got, "Theirs") || !strings.Contains(got, "Mine") {
		t.Fatalf("manager should only list local's filters:\n%s", got)
	}
	if !strings.Contains(got, "Shared · global") || strings.Contains(got, "Mine · global") {
		t.Fatalf("only global filters should be marked:\n%s", got)
	}

	table.SelectRow(0)
	if ev := table.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone)); ev != nil {
		t.Fatal("g should toggle the filter's scope")
	}
	if names := cfg.SavedFiltersFor("prod"); len(names) != 3 {
		t.Fatalf("a filter made global should reach prod, got %+v", names)
	}
	if _, ok := cfg.ProfileFilters["local"]; ok {
		t.Fatalf("local should have no filters of its own left, got %+v", cfg.ProfileFilters)
	}
}

func openRawFilterBuilder(t *testing.T, query string) (*App, *WorkflowList, *filterBuilderState, *components.Tabs) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	state := &filterBuilderState{wl: wl, name: "Orders", clauses: filterClausesFromQuery(query)}
	wl.openFilterBuilder(state)
	om := a.app.Pages().Current().(*overlayModal)
	tabs := om.body.(*components.Tabs)
	return a, wl, state, tabs
}

func typeIntoModal(a *App, text string) {
	for _, r := range text {
		pressModal(a, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
}

func TestFilterBuilderBracketsSwitchTabs(t *testing.T) {
	a, _, _, tabs := openRawFilterBuilder(t, "WorkflowType = 'Order'")
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, ']', tcell.ModNone))
	if tabs.GetActive() != 1 {
		t.Fatalf("] should move to the raw tab, active=%d", tabs.GetActive())
	}
	pressModal(a, tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if tabs.GetActive() != 0 {
		t.Fatalf("Tab should leave the raw tab, active=%d", tabs.GetActive())
	}
	pressModal(a, tcell.NewEventKey(tcell.KeyRune, '[', tcell.ModNone))
	if tabs.GetActive() != 1 {
		t.Fatalf("[ should wrap around to the raw tab, active=%d", tabs.GetActive())
	}
}

func TestFilterBuilderRawTabEditsTheQuery(t *testing.T) {
	a, _, state, tabs := openRawFilterBuilder(t, "WorkflowType = 'Order'")
	tabs.SetActive(1)
	editor := tabs.GetActiveTab().Content.(*tview.TextArea)

	typeIntoModal(a, " AND CustomerId = '[1]H'")
	want := "WorkflowType = 'Order' AND CustomerId = '[1]H'"
	if got := editor.GetText(); got != want {
		t.Fatalf("keys should be typed into the editor, got %q", got)
	}
	if tabs.GetActive() != 1 {
		t.Fatal("brackets, digits, and H are text on the raw tab, not tab switches")
	}

	pressModal(a, tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if tabs.GetActive() != 0 {
		t.Fatal("Tab should switch back to the clauses")
	}
	if len(state.clauses) != 2 || state.clauses[1].Key != "CustomerId" || state.clauses[1].Value != "[1]H" {
		t.Fatalf("leaving the raw tab should parse the edit into clauses, got %+v", state.clauses)
	}
}

func TestFilterBuilderRawTabRefusesAnEmptyQuery(t *testing.T) {
	a, _, state, tabs := openRawFilterBuilder(t, "WorkflowType = 'Order'")
	tabs.SetActive(1)
	editor := tabs.GetActiveTab().Content.(*tview.TextArea)
	editor.SetText("   ", true)

	pressModal(a, tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if tabs.GetActive() != 1 {
		t.Fatal("an empty query should keep the reader on the raw tab")
	}
	pressModal(a, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("an empty query should not be applied")
	}
	if len(state.clauses) != 1 || state.clauses[0].Value != "Order" {
		t.Fatalf("clauses should be untouched, got %+v", state.clauses)
	}
}

func TestFilterBuilderRawTabAppliesAndSavesEdits(t *testing.T) {
	a, wl, _, tabs := openRawFilterBuilder(t, "WorkflowType = 'Order'")
	tabs.SetActive(1)
	tabs.GetActiveTab().Content.(*tview.TextArea).SetText("ExecutionStatus = 'Failed'", true)

	pressModal(a, tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	prompt, ok := a.app.Pages().Current().(*overlayModal)
	if !ok {
		t.Fatalf("Ctrl+S should open the name prompt, current=%T", a.app.Pages().Current())
	}
	if _, isForm := prompt.body.(*components.Form); !isForm {
		t.Fatalf("Ctrl+S should open the name prompt, body=%T", prompt.body)
	}
	submitNamePrompt(t, a, "Orders")
	f, found := a.Config().SavedFilterFor("local", "Orders")
	if !found || f.Query != "ExecutionStatus = 'Failed'" {
		t.Fatalf("saving from the raw tab should store the edited query, got %+v", f)
	}

	tabs.GetActiveTab().Content.(*tview.TextArea).SetText("ExecutionStatus = 'Running'", true)
	pressModal(a, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if wl.visibilityQuery != "ExecutionStatus = 'Running'" {
		t.Fatalf("Enter should apply the edited query, got %q", wl.visibilityQuery)
	}
}
