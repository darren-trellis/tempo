package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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

func TestShowFilterBuilderOpensModal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterBuilder()
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
	table, ok := om.body.(*components.Table)
	if !ok {
		t.Fatalf("content=%T", om.body)
	}
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
	table, ok := om.body.(*components.Table)
	if !ok {
		t.Fatalf("content=%T", om.body)
	}
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
		table, ok := om.body.(*components.Table)
		if !ok {
			t.Fatalf("content=%T", om.body)
		}
		return table
	}

	table := managerTable()
	before := table.RowCount()

	capture := table.GetInputCapture()
	if capture == nil {
		t.Fatal("manager table should have keys")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, 'n', tcell.ModNone)); ev != nil {
		t.Fatal("n should open the builder")
	}

	builderTable, ok := a.app.Pages().Current().(*overlayModal).body.(*components.Table)
	if !ok {
		t.Fatalf("builder body=%T", a.app.Pages().Current().(*overlayModal).body)
	}
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

	if len(cfg.GetSavedFilters()) != 2 {
		t.Fatalf("filter was not saved: %+v", cfg.GetSavedFilters())
	}

	wl.closeModal()
	if got := managerTable().RowCount(); got <= before {
		t.Fatalf("manager still shows the stale list: %d rows, want more than %d", got, before)
	}
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
