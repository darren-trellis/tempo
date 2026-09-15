package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
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
	path := a.app.Crumbs().GetPath()
	if len(path) == 0 || path[len(path)-1] != "Workflows" {
		t.Fatalf("crumbs should stay on the workflow list, got %v", path)
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

func TestWorkflowCountChromeOmitsLabels(t *testing.T) {
	a := &App{
		chromeProfile: "local",
		connected:     true,
		chromeStatsOn: true,
		chromeStats:   WorkflowStats{Running: 4, Completed: 9, Failed: 1},
	}
	var b strings.Builder
	for _, seg := range a.tabChromeSegments() {
		b.WriteString(seg.text)
	}
	got := b.String()
	if strings.Contains(got, "connected") || strings.Contains(got, "disconnected") {
		t.Fatalf("connection should be a glyph only, got %q", got)
	}
	if strings.Contains(got, "Running") || strings.Contains(got, "Completed") || strings.Contains(got, "4 | 9") {
		t.Fatalf("counts should not live on the pane border, got %q", got)
	}
	if strings.Contains(got, "local") {
		t.Fatalf("profile should be the pane title, not tab chrome, got %q", got)
	}
}

func TestCrumbStatBadgesUseLabels(t *testing.T) {
	got := crumbStatBadges(WorkflowStats{Running: 1234, Completed: 23, Failed: 2, TimedOut: 4, ContinuedAsNew: 1000000})
	if len(got) != 5 || got[0].Text != "1,234 Running" || got[1].Text != "23 Completed" || got[2].Text != "2 Failed" || got[3].Text != "4 Timed Out" || got[4].Text != "1,000,000 Continued as New" {
		t.Fatalf("badges=%v", got)
	}
	if len(crumbStatBadges(WorkflowStats{})) != 0 {
		t.Fatal("zero counts should not render badges")
	}
}

func TestPaneChromeDrawsConnectionNotCounts(t *testing.T) {
	a := &App{connected: true, chromeStatsOn: true, chromeStats: WorkflowStats{Running: 3}}
	panel := newChromePanel(a)
	panel.SetTitle("prod (3)")
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
	if !strings.Contains(got, "prod") || !strings.Contains(got, "(3)") {
		t.Fatalf("pane title should stay on the border, got %q", got)
	}
	if strings.Contains(got, "Running") || strings.Contains(got, "3 | 0 | 0") {
		t.Fatalf("counts should not be on the pane border, got %q", got)
	}
	if !strings.Contains(got, theme.IconConnected) {
		t.Fatalf("connection glyph should stay on the pane border, got %q", got)
	}
}

func TestCrumbStatsDrawsBadges(t *testing.T) {
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	a.SetWorkflowStats(WorkflowStats{Running: 100, Completed: 23})
	a.updateCrumbs()
	crumbs := a.app.Crumbs()
	crumbs.SetRect(0, 0, 80, 1)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 1)
	crumbs.Draw(screen)
	a.drawCrumbStats(screen)
	var b strings.Builder
	for x := 0; x < 80; x++ {
		ch, _, _, _ := screen.GetContent(x, 0)
		if ch != 0 {
			b.WriteRune(ch)
		}
	}
	got := b.String()
	if !strings.Contains(got, "100 Running") || !strings.Contains(got, "23 Completed") {
		t.Fatalf("breadcrumb bar should show labeled badges, got %q", got)
	}
}

func TestTabChromeUsesConnectionGlyphs(t *testing.T) {
	a := &App{connected: true, chromeCodecIcon: theme.IconCloud}
	var b strings.Builder
	for _, seg := range a.tabChromeSegments() {
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
	if strings.Contains(got, "connected") || strings.Contains(got, "codec") {
		t.Fatalf("glyphs should not include labels, got %q", got)
	}
}
