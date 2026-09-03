package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestSelectByScheduledID(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end1 := start.Add(2 * time.Second)
	end2 := start.Add(5 * time.Second)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end1, ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start.Add(3 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: end2, ScheduledEventID: 8},
	}))
	if tv.LaneCount() != 2 {
		t.Fatalf("lanes=%d", tv.LaneCount())
	}
	if !tv.SelectByScheduledID(8) {
		t.Fatal("should select second activity")
	}
	lane := tv.SelectedLane()
	if lane == nil || timelineLaneScheduledID(*lane) != 8 {
		t.Fatal("selected lane should be the second activity")
	}
}

func TestTimelineFailedUsesStatusColor(t *testing.T) {
	tv := NewTimelineView()
	_, color := tv.barStyle(TimelineLane{Type: temporal.GroupActivity, Status: "Failed"})
	if color != temporal.StatusFailed.Color() {
		t.Fatalf("failed bar %v, status %v", color, temporal.StatusFailed.Color())
	}
	if timelineStatusColor("Failed") != temporal.StatusFailed.Color() {
		t.Fatal("failed should use the same red as the rest of the app")
	}
	if timelineStatusColor("TimedOut") != temporal.StatusTimedOut.Color() {
		t.Fatal("timed out should use the status color")
	}
	if timelineStatusColor("Fired") == timelineStatusColor("Completed") {
		t.Fatal("fired timers should not share the completed color")
	}
	if timelineStatusColor("Received") == timelineStatusColor("Pending") {
		t.Fatal("signaled events should not share the pending color")
	}
}

func TestTimelineTypeGlyphsAreUnique(t *testing.T) {
	types := []temporal.EventGroupType{
		temporal.GroupActivity,
		temporal.GroupTimer,
		temporal.GroupChildWorkflow,
		temporal.GroupSignal,
		temporal.GroupMarker,
		temporal.GroupOther,
	}
	seen := map[rune]temporal.EventGroupType{}
	for _, typ := range types {
		g := timelineTypeGlyph(typ)
		if g == 0 || g == ' ' {
			t.Fatalf("empty glyph for %s", typ)
		}
		if prev, ok := seen[g]; ok {
			t.Fatalf("%s and %s share %q", prev, typ, string(g))
		}
		seen[g] = typ
	}
}

func TestTimelineBarName(t *testing.T) {
	if got := timelineBarName(TimelineLane{Name: "Activity: ValidateOrder"}); got != "ValidateOrder" {
		t.Fatalf("activity: %q", got)
	}
	if got := timelineBarName(TimelineLane{Name: "Timer: sleep"}); got != "sleep" {
		t.Fatalf("timer: %q", got)
	}
}

func TestTimelineBarContentsPlacesGlyphOnce(t *testing.T) {
	cells := timelineBarContents('●', "Hi There", 12)
	if len(cells) != 12 {
		t.Fatalf("len: %d", len(cells))
	}
	if cells[0] != timelineBarFill {
		t.Fatal("bar should have one blank before the glyph")
	}
	if cells[1] != '●' {
		t.Fatalf("glyph: %q", string(cells[1]))
	}
	if cells[2] != timelineBarFill {
		t.Fatal("glyph should be followed by one blank")
	}
	if string(cells[3:5]) != "Hi" {
		t.Fatalf("label: %q", string(cells[3:5]))
	}
	if cells[5] != timelineBarFill {
		t.Fatal("spaces in the label should keep the bar fill")
	}
	if string(cells[6:11]) != "There" {
		t.Fatalf("rest of label: %q", string(cells[6:11]))
	}
	if cells[11] != timelineBarFill {
		t.Fatalf("trailing fill: %q", string(cells[11]))
	}
	count := 0
	for _, r := range cells {
		if r == '●' {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("glyph count: %d", count)
	}
}

func TestFitTimelineName(t *testing.T) {
	if got := fitTimelineName("ValidateOrder", 20); got != "ValidateOrder" {
		t.Fatalf("short name: %q", got)
	}
	if got := fitTimelineName("ValidateOrder", 8); got != "Validat…" {
		t.Fatalf("truncated: %q", got)
	}
	if got := fitTimelineName("ValidateOrder", 0); got != "" {
		t.Fatalf("empty width: %q", got)
	}
}

func TestTimelineEndMarkerKeepsBarFill(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "Short"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: start.Add(20 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start, ActivityType: "Wide"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: start.Add(time.Minute), ScheduledEventID: 8},
	}))
	if !tv.SelectByScheduledID(5) {
		t.Fatal("should select the short activity")
	}
	tv.SetRect(0, 0, 80, 10)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 10)
	tv.Draw(screen)

	timeRange := tv.endTime.Sub(tv.startTime)
	_, shortEnd := tv.laneBarSpan(tv.lanes[tv.selectedLane], 80, timeRange)
	endCol := shortEnd - 1
	if endCol < 0 {
		t.Fatal("short bar should have an end cell")
	}

	selectedCh, _, _, _ := screen.GetContent(endCol, 2)
	if selectedCh == ' ' || selectedCh == '·' || selectedCh == 0 {
		t.Fatalf("selected bar end should stay filled, got %q", string(selectedCh))
	}
	if selectedCh != '│' {
		t.Fatalf("selected bar should keep the end cap, got %q", string(selectedCh))
	}

	wideCh, _, _, _ := screen.GetContent(endCol, 3)
	if wideCh == '│' {
		t.Fatal("end marker should not cut through the overlapping bar")
	}
	if wideCh == ' ' || wideCh == '·' || wideCh == 0 {
		t.Fatalf("overlapping bar should stay filled at the end marker, got %q", string(wideCh))
	}
}

func TestTimelineCursorSkipsOtherBars(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start.Add(20 * time.Second), ActivityType: "Late"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: start.Add(40 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start, ActivityType: "Wide"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: start.Add(time.Minute), ScheduledEventID: 8},
	}))
	if !tv.SelectByScheduledID(5) {
		t.Fatal("should select the late activity")
	}
	tv.SetRect(0, 0, 80, 10)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 10)
	tv.Draw(screen)

	timeRange := tv.endTime.Sub(tv.startTime)
	startPos, _ := tv.laneBarSpan(tv.lanes[tv.selectedLane], 80, timeRange)
	wideStart, wideEnd := tv.laneBarSpan(tv.lanes[1], 80, timeRange)
	if !barContainsCol(wideStart, wideEnd, startPos) {
		t.Fatalf("expected the wide bar to cover the cursor column %d (%d-%d)", startPos, wideStart, wideEnd)
	}
	ch, _, _, _ := screen.GetContent(startPos, 3)
	if ch == '│' {
		t.Fatal("cursor should not cut through another bar")
	}
}

func TestTimelineCursorSkipsSelectedGlyph(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5},
	}))
	tv.SetRect(0, 0, 80, 10)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 10)
	tv.Draw(screen)

	pad, _, _, _ := screen.GetContent(0, 2)
	if pad == '│' {
		t.Fatal("cursor should not cover the selected bar")
	}
	if pad != timelineBarFill {
		t.Fatalf("selected bar should start with padding, got %q", string(pad))
	}
	ch, _, _, _ := screen.GetContent(1, 2)
	if ch == '│' {
		t.Fatal("cursor should not cover the selected bar glyph")
	}
	if ch != timelineTypeGlyph(temporal.GroupActivity) {
		t.Fatalf("selected bar glyph: %q", string(ch))
	}
}

func TestTimelineDrawsNameInsideBar(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "ValidateOrder"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5},
	}))
	tv.SetRect(0, 0, 80, 10)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 10)
	tv.Draw(screen)

	var header, row strings.Builder
	for x := 0; x < 80; x++ {
		ch, _, _, _ := screen.GetContent(x, 0)
		header.WriteRune(ch)
		ch, _, _, _ = screen.GetContent(x, 2)
		row.WriteRune(ch)
	}
	if strings.Contains(header.String(), "Event") {
		t.Fatalf("header should not list activities, got %q", header.String())
	}
	if strings.Contains(row.String(), "Activity:") {
		t.Fatalf("bar should not repeat the activity list prefix, got %q", row.String())
	}
	if !strings.Contains(row.String(), "ValidateOrder") {
		t.Fatalf("bar should contain the activity name, got %q", row.String())
	}
}

func TestTimelineMouseScrollsHorizontally(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5},
	}))
	tv.SetRect(0, 0, 80, 10)
	tv.zoomLevel = 2

	handler := tv.MouseHandler()
	event := tcell.NewEventMouse(10, 4, tcell.WheelRight, tcell.ModNone)
	consumed, _ := handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("horizontal wheel should be consumed")
	}
	if tv.scrollX != 1 {
		t.Fatalf("scrollX=%d", tv.scrollX)
	}

	event = tcell.NewEventMouse(10, 4, tcell.WheelDown, tcell.ModShift)
	consumed, _ = handler(tview.MouseScrollDown, event, func(tview.Primitive) {})
	if !consumed || tv.scrollX != 2 {
		t.Fatalf("shift+wheel should scroll horizontally, consumed=%v scrollX=%d", consumed, tv.scrollX)
	}
}

func TestTimelineMouseScrollStep(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetMouseScrollStep(func() int { return 4 })
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5},
	}))
	tv.SetRect(0, 0, 80, 10)
	tv.zoomLevel = 2

	handler := tv.MouseHandler()
	event := tcell.NewEventMouse(10, 4, tcell.WheelRight, tcell.ModNone)
	consumed, _ := handler(tview.MouseScrollRight, event, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("horizontal wheel should be consumed")
	}
	if tv.scrollX != 4 {
		t.Fatalf("scrollX=%d", tv.scrollX)
	}
}

func TestTimelineScrollStopsAtLastBar(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: end, ScheduledEventID: 5},
	}))
	tv.SetRect(0, 0, 80, 10)

	tv.scroll(20)
	if tv.scrollX != 0 {
		t.Fatalf("unzoomed timeline should not scroll, scrollX=%d", tv.scrollX)
	}

	tv.zoomLevel = 2
	tv.scroll(10_000)
	if want := tv.maxScrollX(); tv.scrollX != want || want <= 0 {
		t.Fatalf("scrollX=%d max=%d", tv.scrollX, want)
	}
	if tv.contentWidth(80)-tv.scrollX != 80 {
		t.Fatalf("fully scrolled view should end at the last bar, content=%d scroll=%d", tv.contentWidth(80), tv.scrollX)
	}

	tv.scroll(-10_000)
	if tv.scrollX != 0 {
		t.Fatalf("left scroll should stop at 0, scrollX=%d", tv.scrollX)
	}

	tv.scrollX = 10_000
	tv.zoomLevel = 1
	tv.clampScrollX()
	if tv.scrollX != 0 {
		t.Fatalf("zooming out should pull scroll back, scrollX=%d", tv.scrollX)
	}
}
