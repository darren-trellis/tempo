package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestWorkflowListStopKeepsStatusCounts(t *testing.T) {
	a := &App{}
	wl := NewWorkflowList(a, "default")
	a.SetWorkflowStats(WorkflowStats{Running: 4, Completed: 9, Failed: 1})
	wl.Stop()
	if !a.chromeStatsOn || a.chromeStats.Running != 4 {
		t.Fatal("stopping the list for a modal should leave the status counts")
	}
}

func TestModalKeepsWorkflowChrome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	a.SetWorkflowStats(WorkflowStats{Running: 4, Completed: 9, Failed: 1})
	a.updateCrumbs()

	a.showThemeSelector()
	if !a.app.Pages().CurrentIsModal() {
		t.Fatal("expected a modal")
	}
	if crumbs := a.app.Crumbs(); crumbs != nil {
		if path := crumbs.GetPath(); len(path) != 0 {
			t.Fatalf("breadcrumb path should stay empty, got %v", path)
		}
	}
	if !a.chromeStatsOn || a.chromeStats.Running != 4 {
		t.Fatal("workflow counts should stay visible while a modal is open")
	}

	a.app.Pages().DismissModal()
	if !a.chromeStatsOn {
		t.Fatal("workflow counts should still be there after the modal closes")
	}
}

func TestLeavingWorkflowsClearsStatusCounts(t *testing.T) {
	a := &App{}
	a.SetWorkflowStats(WorkflowStats{Running: 2})
	a.syncWorkflowStats(NewNamespaceList(a))
	if a.chromeStatsOn {
		t.Fatal("leaving the workflow list should clear the counts")
	}
}

func TestWorkflowCountsHideOffWorkflowsTab(t *testing.T) {
	a := &App{}
	wl := NewWorkflowList(a, "default")
	a.SetWorkflowStats(WorkflowStats{Running: 4, Completed: 9})
	wl.setListKind(listTaskQueues)
	if a.chromeStatsOn {
		t.Fatal("task queues should hide workflow counts")
	}
	a.syncWorkflowStats(wl)
	if a.chromeStatsOn {
		t.Fatal("staying on task queues should keep counts hidden")
	}
	wl.setListKind(listWorkflows)
	if !a.chromeStatsOn {
		t.Fatal("returning to workflows should show counts again")
	}
}

func TestWorkflowCountChromeOmitsLabels(t *testing.T) {
	a := &App{
		chromeProfile: "local",
		connected:     true,
		chromeStatsOn: true,
		chromeStats:   WorkflowStats{Running: 4, Completed: 9, Failed: 1},
	}
	var b strings.Builder
	for _, seg := range a.statusBarSegments() {
		b.WriteString(seg.text)
	}
	got := b.String()
	if strings.Contains(got, "connected") || strings.Contains(got, "disconnected") {
		t.Fatalf("connection should be a glyph only, got %q", got)
	}
	if strings.Contains(got, "Running") || strings.Contains(got, "Completed") || strings.Contains(got, "4 | 9") {
		t.Fatalf("counts should not live on the pane border, got %q", got)
	}
	if !strings.Contains(got, "local") {
		t.Fatalf("profile should be on the status bar, got %q", got)
	}
}

func TestWorkflowStatBadgesUseLabels(t *testing.T) {
	got := workflowStatBadges(WorkflowStats{Running: 1234, Completed: 23, Failed: 2, TimedOut: 4, ContinuedAsNew: 1000000})
	if len(got) != 5 || got[0].Text != "1,234 Running" || got[1].Text != "23 Completed" || got[2].Text != "2 Failed" || got[3].Text != "4 Timed Out" || got[4].Text != "1,000,000 Continued as New" {
		t.Fatalf("badges=%v", got)
	}
	if len(workflowStatBadges(WorkflowStats{})) != 0 {
		t.Fatal("zero counts should not render badges")
	}
}

func TestPaneChromeOmitsConnectionAndCounts(t *testing.T) {
	a := &App{connected: true, chromeStatsOn: true, chromeStats: WorkflowStats{Running: 3}}
	panel := newChromePanel(a)
	panel.SetTitle("")
	panel.SetRect(0, 0, 80, 6)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 6)
	panel.Draw(screen)
	var b strings.Builder
	for x := 0; x < 80; x++ {
		ch, _, _, _ := screen.GetContent(x, 0)
		if ch != 0 {
			b.WriteRune(ch)
		}
	}
	got := b.String()
	if strings.Contains(got, "prod") || strings.Contains(got, "(3)") {
		t.Fatalf("primary pane should have no title, got %q", got)
	}
	if strings.Contains(got, "Running") || strings.Contains(got, "3 | 0 | 0") {
		t.Fatalf("counts should not be on the pane border, got %q", got)
	}
	if strings.Contains(got, theme.IconConnected) {
		t.Fatalf("connection glyph should not be on the pane border, got %q", got)
	}
}

func TestStatBadgesDrawOnBottomBar(t *testing.T) {
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	a.SetWorkflowStats(WorkflowStats{Running: 100, Completed: 23})
	a.menu.SetRect(0, 0, 80, 1)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 1)
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	bottom := rowText(screen, 0, 80)
	if !strings.Contains(bottom, "100 Running") || !strings.Contains(bottom, "23 Completed") {
		t.Fatalf("bottom bar should show labeled badges, got %q", bottom)
	}

	a.setStatusMessage("Copied workflow ID")
	a.menu.Draw(screen)
	a.drawHintStatus(screen)
	a.drawBottomChrome(screen)
	bottom = rowText(screen, 0, 80)
	if !strings.Contains(bottom, "Copied workflow ID") {
		t.Fatalf("status should stay on the left, got %q", bottom)
	}
	if !strings.Contains(bottom, "100 Running") {
		t.Fatalf("counts should stay on the right with a status message, got %q", bottom)
	}

	a.modalHintsOn = true
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	bottom = rowText(screen, 0, 80)
	if strings.Contains(bottom, "Running") {
		t.Fatalf("counts should hide while modal hints are on, got %q", bottom)
	}
}

func TestStatusBarShowsLoadedDisplayedProfileAndTree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	wl.allWorkflows = []temporal.Workflow{
		{ID: "wf-1", RunID: "run-1", Status: "Running"},
		{ID: "wf-2", RunID: "run-2", Status: "Completed"},
		{ID: "wf-3", RunID: "run-3", Status: "Failed"},
	}
	wl.workflows = wl.allWorkflows[:2]
	a.app.Pages().Push(wl)
	a.SetWorkflowStats(wl.displayedStats())
	a.menu.SetRect(0, 0, 120, 1)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 1)
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	got := rowText(screen, 0, 120)
	if !strings.Contains(got, "3 Loaded") || !strings.Contains(got, "2 Displayed") {
		t.Fatalf("loaded/displayed should sit with status counts, got %q", got)
	}
	badges := a.workflowChromeBadges()
	if len(badges) < 2 || badges[0].Variant != components.BadgeDefault || badges[1].Variant != components.BadgeDefault {
		t.Fatalf("loaded and displayed should use the same badge color")
	}
	if !strings.Contains(got, "local") {
		t.Fatalf("profile should be on the status bar, got %q", got)
	}
	if !strings.Contains(got, theme.IconNamespace) {
		t.Fatalf("tree mode should use the tree glyph, got %q", got)
	}
	if strings.Contains(paneTitle(wl.workflowsPanel), "local") || strings.Contains(paneTitle(wl.workflowsPanel), "Loaded") {
		t.Fatalf("primary pane should stay untitled, got %q", paneTitle(wl.workflowsPanel))
	}

	wl.workflowTreeMode = false
	screen.Clear()
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	got = rowText(screen, 0, 120)
	if !strings.Contains(got, theme.IconList) {
		t.Fatalf("list mode should use the list glyph, got %q", got)
	}

	wl.setListKind(listTaskQueues)
	screen.Clear()
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	got = rowText(screen, 0, 120)
	if strings.Contains(got, "Loaded") || strings.Contains(got, "Displayed") {
		t.Fatalf("other tabs should hide loaded/displayed, got %q", got)
	}
	if strings.Contains(got, theme.IconList) || strings.Contains(got, theme.IconNamespace) {
		t.Fatalf("other tabs should hide the tree/list glyph, got %q", got)
	}
	if !strings.Contains(got, "local") {
		t.Fatalf("profile should stay on the status bar off workflows, got %q", got)
	}
}

func TestBottomBarDrawsConnectionAndCodec(t *testing.T) {
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	a.connected = true
	a.chromeCodecIcon = theme.IconCloud
	a.SetWorkflowStats(WorkflowStats{Running: 4})
	a.menu.SetRect(0, 0, 80, 1)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 1)
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	got := rowText(screen, 0, 80)
	if !strings.Contains(got, theme.IconConnected) {
		t.Fatalf("bottom bar should show the connection glyph, got %q", got)
	}
	if !strings.Contains(got, theme.IconCloud) {
		t.Fatalf("bottom bar should show the codec glyph, got %q", got)
	}
	if !strings.Contains(got, "4 Running") {
		t.Fatalf("counts should sit with the status glyphs, got %q", got)
	}
	if !strings.Contains(got, "local") {
		t.Fatalf("profile should sit on the status bar, got %q", got)
	}
	if !strings.Contains(got, theme.IconCloud+" | ") || !strings.Contains(got, "4 Running") {
		t.Fatalf("codec glyph should be pipe-separated from counts, got %q", got)
	}
	if !strings.Contains(got, theme.IconRefresh) {
		t.Fatalf("bottom bar should show the auto-refresh glyph, got %q", got)
	}

	a.modalHintsOn = true
	a.menu.Draw(screen)
	a.drawBottomChrome(screen)
	got = rowText(screen, 0, 80)
	if !strings.Contains(got, theme.IconConnected) {
		t.Fatalf("connection should stay visible with modal hints, got %q", got)
	}
	if strings.Contains(got, "Running") {
		t.Fatalf("counts should hide while modal hints are on, got %q", got)
	}
}

func TestTabChromeUsesConnectionGlyphs(t *testing.T) {
	a := &App{connected: true, chromeCodecIcon: theme.IconCloud}
	var b strings.Builder
	for _, seg := range a.statusBarSegments() {
		b.WriteString(seg.text)
	}
	got := b.String()
	if !strings.Contains(got, theme.IconConnected) {
		t.Fatalf("missing server glyph: %q", got)
	}
	if !strings.Contains(got, theme.IconCloud) {
		t.Fatalf("missing codec glyph: %q", got)
	}
	if !strings.Contains(got, theme.IconConnected+" | "+theme.IconCloud) {
		t.Fatalf("connectivity icons should be pipe-separated, got %q", got)
	}
	if !strings.Contains(got, theme.IconCloud+" | "+theme.IconPause) {
		t.Fatalf("codec and auto-refresh glyphs should be pipe-separated, got %q", got)
	}
	if strings.Contains(got, "connected") || strings.Contains(got, "codec") {
		t.Fatalf("glyphs should not include labels, got %q", got)
	}
}

func TestAutoRefreshGlyphFollowsCurrentView(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	if a.autoRefreshChromeText() != theme.IconPause {
		t.Fatal("workflows default to auto-refresh off")
	}
	wl.autoRefresh = true
	if a.autoRefreshChromeText() != theme.IconRefresh {
		t.Fatal("workflows should show refresh when auto-refresh is on")
	}
	wl.setListKind(listTaskQueues)
	if a.autoRefreshChromeText() != theme.IconPause {
		t.Fatal("task queues default to auto-refresh off")
	}
	wl.taskQueues.autoRefresh = true
	if a.autoRefreshChromeText() != theme.IconRefresh {
		t.Fatal("task queues should show refresh when auto-refresh is on")
	}
}

func TestModalFocusHighlightsModalPane(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.focusPane = focusWorkflows
	wl.applyFocusStyles()
	if wl.workflowsPanel == nil || !wl.workflowsPanel.IsFocused() {
		t.Fatal("workflow pane should be highlighted before a modal opens")
	}

	a.showThemeSelector()
	wl.applyFocusStyles()
	if wl.workflowsPanel.IsFocused() {
		t.Fatal("background pane should not stay highlighted while a modal is focused")
	}

	current := a.app.Pages().Current()
	panelOwner, ok := current.(interface{ GetPanel() *components.Panel })
	if !ok {
		t.Fatalf("modal type %T should expose a panel", current)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	current.Draw(screen)
	if panel := panelOwner.GetPanel(); panel == nil || !panel.IsFocused() {
		t.Fatal("focused modal pane should use the accent border")
	}
}
