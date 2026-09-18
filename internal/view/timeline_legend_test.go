package view

import (
	"strings"
	"testing"

	"github.com/atterpac/jig/theme"
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

func TestTimelineHasFocusWithoutApp(t *testing.T) {
	if (&App{}).timelineHasFocus() {
		t.Fatal("app without a tview runtime should not report timeline focus")
	}
}

func TestTimelineHintsIncludeLegend(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.focusPane = focusTimeline
	if desc := hintDescription(wl.Hints(), "L"); desc != "Legend" {
		t.Fatalf("timeline hint: %q", desc)
	}
	if desc := hintDescription(wl.Hints(), "z"); desc != "" {
		t.Fatalf("timeline should not offer z, got %q", desc)
	}
}
