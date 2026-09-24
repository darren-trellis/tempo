package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func TestTimelineLegendListsStatusColors(t *testing.T) {
	text := formatTimelineLegendColumn("Status", timelineLegendStatuses(), false)
	for _, label := range []string{"Completed", "Failed", "Fired", "Signaled", "Timed Out", "Canceled", "Terminated", "Running", "Pending"} {
		if !strings.Contains(text, label) {
			t.Fatalf("missing status %q in %q", label, text)
		}
	}
	if !strings.Contains(text, theme.ColorToHex(timelineStatusColor("Failed"))) {
		t.Fatal("failed should use the timeline status color")
	}
	if !strings.Contains(text, "┄┄") {
		t.Fatal("pending should use a dashed marker")
	}
}

func TestTimelineLegendListsTypeGlyphs(t *testing.T) {
	items := timelineLegendTypeItems()
	text := formatTimelineLegendColumn("Event Types", items, true)
	for _, typ := range timelineLegendTypes() {
		if !strings.Contains(text, timelineTypeLabel(typ)) {
			t.Fatalf("missing type %q in %q", timelineTypeLabel(typ), text)
		}
		if !strings.Contains(text, string(timelineTypeGlyph(typ))) {
			t.Fatalf("missing glyph for %s in %q", typ, text)
		}
		if !strings.Contains(text, theme.ColorToHex(timelineTypeColor(typ))) {
			t.Fatalf("missing color for %s in %q", typ, text)
		}
	}
	if timelineTypeGlyph(temporal.GroupActivity) == timelineTypeGlyph(temporal.GroupTimer) {
		t.Fatal("activity and timer glyphs should differ")
	}
}

func TestTimelineLegendModalDrawsColumns(t *testing.T) {
	modal := NewTimelineLegendModal()
	modal.SetRect(0, 0, 80, 24)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 24)
	modal.Draw(screen)

	var body strings.Builder
	for y := 0; y < 24; y++ {
		for x := 0; x < 80; x++ {
			ch, _, _, _ := screen.GetContent(x, y)
			body.WriteRune(ch)
		}
		body.WriteByte('\n')
	}
	got := body.String()
	for _, label := range []string{"Status", "Event Types", "Completed", "Activity"} {
		if !strings.Contains(got, label) {
			t.Fatalf("modal should show %q, got:\n%s", label, got)
		}
	}
}

func TestTimelineChartDrawsNoInlineLegend(t *testing.T) {
	// Three lanes in a ten row panel is where the inline legend used to fit.
	tv := timelineWithLanes(3)
	screen := timelineScreen(t, tv, 80, 10)

	var body strings.Builder
	for y := 0; y < 10; y++ {
		for x := 0; x < 80; x++ {
			ch, _, _, _ := screen.GetContent(x, y)
			body.WriteRune(ch)
		}
		body.WriteByte('\n')
	}
	got := body.String()
	for _, label := range []string{"Activity", "Timer", "Signal", "Child"} {
		if strings.Contains(got, label) {
			t.Fatalf("the legend belongs in the modal, but the chart still shows %q:\n%s", label, got)
		}
	}
}

func TestTimelineQuestionMarkOpensTheLegend(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)
	wl.toggleTimeline()

	capture := wl.timelineView.GetInputCapture()
	if capture == nil {
		t.Fatal("timeline should have an input capture")
	}
	if ev := capture(tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone)); ev != nil {
		t.Fatal("? should be consumed by the timeline")
	}
	if _, ok := a.app.Pages().Current().(*TimelineLegendModal); !ok {
		t.Fatalf("? should open the legend modal, got %T", a.app.Pages().Current())
	}
}

func TestTimelineHintsIncludeLegend(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.focusPane = focusTimeline
	if desc := hintDescription(wl.Hints(), "?"); desc != "Legend" {
		t.Fatalf("timeline hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "L"); desc != "" {
		t.Fatalf("the legend moved off L, got %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "z"); desc != "" {
		t.Fatalf("timeline should not offer z, got %q", desc)
	}
}
