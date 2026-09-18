package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
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

func TestLayoutFilterBarOverflow(t *testing.T) {
	items := []filterBarChip{
		{label: "All"},
		{label: "Running Workflows"},
		{label: "Failed Workflows"},
		{label: "Completed Workflows"},
	}
	shown, extra := layoutFilterBar(items, 28)
	if extra <= 0 || len(shown) == 0 || shown[0].label != "All" {
		t.Fatalf("shown=%v extra=%d", shown, extra)
	}
	shown, extra = layoutFilterBar(items, 80)
	if extra != 0 || len(shown) != 4 {
		t.Fatalf("wide shown=%d extra=%d", len(shown), extra)
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
	if h := wl.filterBarHeight(); h < 2 {
		t.Fatalf("wrap height=%d", h)
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

func TestShowFilterBuilderOpensModal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.showFilterBuilder()
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("F should open the filter builder")
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
