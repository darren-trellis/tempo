package view

import (
	"strings"
	"testing"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
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

func TestTimelineChartCarriesNoStartOrDurationLabel(t *testing.T) {
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

	for x := 0; x < 80; x++ {
		ch, _, style, _ := screen.GetContent(x, 0)
		if _, bg, _ := style.Decompose(); bg == theme.Accent() {
			t.Fatalf("header should carry no start label, got %q at column %d", string(ch), x)
		}
	}
	for x := 0; x < 80; x++ {
		ch, _, _, _ := screen.GetContent(x, 1)
		if ch != '─' {
			t.Fatalf("the scale rule should carry no duration label, got %q at column %d", string(ch), x)
		}
	}
}

func TestTimelineSelectionReportsOffsets(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: start.Add(10 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: start.Add(25 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: start.Add(40 * time.Second), ScheduledEventID: 8},
	}))

	if !tv.SelectByScheduledID(8) {
		t.Fatal("should select the second activity")
	}
	sel, ok := tv.Selection()
	if !ok {
		t.Fatal("a highlighted lane should report a selection")
	}
	if sel.Start != 25*time.Second {
		t.Fatalf("start=%s", sel.Start)
	}
	if sel.Duration != 15*time.Second {
		t.Fatalf("duration=%s", sel.Duration)
	}
	if sel.Gap != 15*time.Second {
		t.Fatalf("gap=%s", sel.Gap)
	}
	if sel.Running {
		t.Fatal("a completed activity should not read as running")
	}

	if !tv.SelectByScheduledID(5) {
		t.Fatal("should select the first activity")
	}
	sel, _ = tv.Selection()
	if sel.Start != 0 || sel.Gap != 0 {
		t.Fatalf("the first lane has no gap, got start=%s gap=%s", sel.Start, sel.Gap)
	}

	empty, ok := NewTimelineView().Selection()
	if ok || empty != (TimelineSelection{}) {
		t.Fatal("an empty timeline has no selection")
	}
}

func TestTimelineSelectionMarksRunningLanes(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree([]temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
		{ID: 5, Type: "ActivityTaskScheduled", Time: start, ActivityType: "Open"},
	}))
	sel, ok := tv.Selection()
	if !ok {
		t.Fatal("expected a lane")
	}
	if !sel.Running || sel.Duration != 0 {
		t.Fatalf("an open activity should read as running, got %+v", sel)
	}
}

func TestTimelineOffsetsLandOnTheStatusBar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	a.app.Pages().Push(wl)

	statusBar := func() string {
		var b strings.Builder
		for _, seg := range a.statusBarSegments() {
			b.WriteString(seg.text)
		}
		return b.String()
	}
	if strings.Contains(statusBar(), "Start") {
		t.Fatalf("a hidden timeline should stay off the status bar, got %q", statusBar())
	}

	wl.toggleTimeline()
	now := time.Now()
	wl.showPreviewEvents(temporal.Workflow{ID: "wf", RunID: "run"}, []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: now.Add(-time.Minute)},
		{ID: 5, Type: "ActivityTaskScheduled", Time: now.Add(-time.Minute), ActivityType: "First"},
		{ID: 6, Type: "ActivityTaskCompleted", Time: now.Add(-50 * time.Second), ScheduledEventID: 5},
		{ID: 8, Type: "ActivityTaskScheduled", Time: now.Add(-40 * time.Second), ActivityType: "Second"},
		{ID: 9, Type: "ActivityTaskCompleted", Time: now.Add(-30 * time.Second), ScheduledEventID: 8},
	})
	if wl.timelineView.LaneCount() != 2 {
		t.Fatalf("lanes=%d", wl.timelineView.LaneCount())
	}

	wl.timelineView.SelectByScheduledID(8)
	got := statusBar()
	if !strings.Contains(got, "Start 20s") {
		t.Fatalf("status bar should carry the lane start, got %q", got)
	}
	if !strings.Contains(got, "Dur 10s") {
		t.Fatalf("status bar should carry the lane duration, got %q", got)
	}
	if !strings.Contains(got, "Gap 10s") {
		t.Fatalf("status bar should carry the gap from the previous lane, got %q", got)
	}

	wl.toggleTimeline()
	if strings.Contains(statusBar(), "Start") {
		t.Fatalf("hiding the timeline should drop the offsets, got %q", statusBar())
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

func TestTimelineCursorSkipsSpilledLaneNames(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	instant := start
	activityEnd := start.Add(40 * time.Second)

	// An Upsert marker is instantaneous, so its bar is two cells wide and the
	// name spills across the chart instead of sitting inside the bar.
	tv := NewTimelineView()
	tv.lanes = []TimelineLane{
		{Name: "UpsertWorkflowSearchAttributes", Type: temporal.GroupMarker, Status: "Completed", StartTime: start, EndTime: &instant},
		{Name: "readGatewayOpsActivity", Type: temporal.GroupActivity, Status: "Completed", StartTime: start.Add(10 * time.Second), EndTime: &activityEnd},
	}
	tv.startTime = start
	tv.endTime = end
	tv.selectedLane = 1
	tv.SetRect(0, 0, 80, 10)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 10)
	tv.Draw(screen)

	timeRange := tv.endTime.Sub(tv.startTime)
	markerStart, markerEnd := tv.laneBarSpan(tv.lanes[0], 80, timeRange)
	if markerEnd-markerStart >= timelineBarLabelInside {
		t.Fatalf("the marker bar should be too narrow to hold its name, span %d-%d", markerStart, markerEnd)
	}
	spanStart, spanEnd := tv.laneRowSpan(tv.lanes[0], 80, timeRange)
	if spanEnd <= markerEnd {
		t.Fatalf("the spilled name should widen the painted span, got %d-%d", spanStart, spanEnd)
	}

	cursorCol, _ := tv.laneBarSpan(tv.lanes[1], 80, timeRange)
	if !barContainsCol(spanStart, spanEnd, cursorCol) {
		t.Fatalf("expected the spilled name to cover the cursor column %d (%d-%d)", cursorCol, spanStart, spanEnd)
	}
	ch, _, _, _ := screen.GetContent(cursorCol, 2)
	if ch == '│' {
		t.Fatal("the cursor should not cut through a name spilled outside its bar")
	}
}

// timelineWithLanes builds a timeline of n completed activities over a minute.
func timelineWithLanes(n int) *TimelineView {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tv := NewTimelineView()
	for i := 0; i < n; i++ {
		at := start.Add(time.Duration(i) * time.Second)
		end := at.Add(2 * time.Second)
		tv.lanes = append(tv.lanes, TimelineLane{
			Name:      "readGatewayOpsActivity",
			Type:      temporal.GroupActivity,
			Status:    "Completed",
			StartTime: at,
			EndTime:   &end,
		})
	}
	tv.startTime = start
	tv.endTime = start.Add(time.Minute)
	return tv
}

func timelineScreen(t *testing.T, tv *TimelineView, width, height int) tcell.SimulationScreen {
	t.Helper()
	tv.SetRect(0, 0, width, height)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(width, height)
	tv.Draw(screen)
	return screen
}

func TestTimelineDrawsVerticalScrollbarWhenLanesOverflow(t *testing.T) {
	tv := timelineWithLanes(30)
	screen := timelineScreen(t, tv, 80, 10)
	if !tv.laneScroll().overflow() {
		t.Fatal("30 lanes in 7 rows should overflow")
	}
	if !edgeHasGlyph(screen, 79, 2, tv.visibleLaneCount(), scrollbarThinVert) {
		t.Fatal("overflowing lanes should draw a vertical scrollbar on the right edge")
	}
	if tv.chartWidth() != 79 {
		t.Fatalf("the chart should give up a column to the scrollbar, got %d", tv.chartWidth())
	}

	// The bars must not run underneath the scrollbar column.
	for y := 2; y < 2+tv.visibleLaneCount(); y++ {
		mainc, _, _, _ := screen.GetContent(79, y)
		if mainc != scrollbarThinVert {
			t.Fatalf("row %d should leave the last column to the scrollbar, got %q", y, string(mainc))
		}
	}
}

func TestTimelineHidesVerticalScrollbarWhenLanesFit(t *testing.T) {
	tv := timelineWithLanes(3)
	screen := timelineScreen(t, tv, 80, 10)
	if tv.laneScroll().overflow() {
		t.Fatal("3 lanes in 7 rows should not overflow")
	}
	if edgeHasGlyph(screen, 79, 2, tv.visibleLaneCount(), scrollbarThinVert) {
		t.Fatal("lanes that fit should not draw a vertical scrollbar")
	}
	if tv.chartWidth() != 80 {
		t.Fatalf("the chart should keep the full width, got %d", tv.chartWidth())
	}
}

func TestTimelineDrawsHorizontalScrollbarWhenZoomed(t *testing.T) {
	tv := timelineWithLanes(3)
	tv.SetRect(0, 0, 80, 10)
	tv.zoomLevel = 2

	screen := timelineScreen(t, tv, 80, 10)
	if !tv.timeScroll(tv.chartWidth()).overflow() {
		t.Fatal("a zoomed chart should overflow horizontally")
	}
	mainc, _, _, _ := screen.GetContent(1, 9)
	if mainc != scrollbarThinHoriz {
		t.Fatalf("zoomed chart should draw a horizontal scrollbar on the bottom row, got %q", string(mainc))
	}
}

func TestTimelineHorizontalScrollbarTracksScrolling(t *testing.T) {
	tv := timelineWithLanes(3)
	tv.SetRect(0, 0, 80, 10)
	tv.zoomLevel = 2
	screen := timelineScreen(t, tv, 80, 10)

	thumbX := func() int {
		for x := 0; x < 80; x++ {
			mainc, _, style, _ := screen.GetContent(x, 9)
			if mainc != scrollbarThinHoriz {
				continue
			}
			if fg, _, _ := style.Decompose(); fg != theme.FgDim() {
				return x
			}
		}
		return -1
	}
	left := thumbX()
	if left < 0 {
		t.Fatal("expected a horizontal thumb")
	}

	tv.scroll(10_000)
	tv.Draw(screen)
	right := thumbX()
	if right <= left {
		t.Fatalf("the thumb should follow the scroll, left=%d right=%d", left, right)
	}
}

func TestTimelineScrollbarsRespectTheSetting(t *testing.T) {
	tv := timelineWithLanes(30)
	tv.SetShowScrollbars(func() bool { return false })
	screen := timelineScreen(t, tv, 80, 10)
	if edgeHasGlyph(screen, 79, 2, tv.visibleLaneCount(), scrollbarThinVert) {
		t.Fatal("scrollbars turned off should not draw a vertical bar")
	}
	if tv.chartWidth() != 80 {
		t.Fatalf("with scrollbars off the chart should keep the full width, got %d", tv.chartWidth())
	}
}

func TestTimelinePageAndEdgeKeys(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	events := []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: start},
	}
	id := int64(5)
	for i := 0; i < 20; i++ {
		at := start.Add(time.Duration(i) * time.Second)
		events = append(events,
			temporal.EnhancedHistoryEvent{ID: id, Type: "ActivityTaskScheduled", Time: at, ActivityType: "Step"},
			temporal.EnhancedHistoryEvent{ID: id + 1, Type: "ActivityTaskCompleted", Time: at.Add(time.Second), ScheduledEventID: id},
		)
		id += 2
	}
	tv := NewTimelineView()
	tv.SetNodes(temporal.BuildEventTree(events))
	if tv.LaneCount() != 20 {
		t.Fatalf("lanes=%d", tv.LaneCount())
	}
	tv.SetRect(0, 0, 80, 10)
	if tv.visibleLaneCount() != 7 {
		t.Fatalf("visible=%d", tv.visibleLaneCount())
	}

	handler := tv.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModNone), func(tview.Primitive) {})
	if tv.selectedLane != 7 {
		t.Fatalf("page down should move a screenful, selected=%d", tv.selectedLane)
	}
	if tv.scrollY != 1 {
		t.Fatalf("page down should keep the selection on screen, scrollY=%d", tv.scrollY)
	}

	handler(tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone), func(tview.Primitive) {})
	if tv.selectedLane != 0 {
		t.Fatalf("page up should move back a screenful, selected=%d", tv.selectedLane)
	}

	handler(tcell.NewEventKey(tcell.KeyRune, 'G', tcell.ModNone), func(tview.Primitive) {})
	if tv.selectedLane != 19 {
		t.Fatalf("G should jump to the last lane, selected=%d", tv.selectedLane)
	}
	if tv.scrollY != 13 {
		t.Fatalf("G should scroll the last lane into view, scrollY=%d", tv.scrollY)
	}

	handler(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone), func(tview.Primitive) {})
	if tv.selectedLane != 0 || tv.scrollY != 0 {
		t.Fatalf("g should jump to the first lane, selected=%d scrollY=%d", tv.selectedLane, tv.scrollY)
	}
}

func TestTimelineHomeAndEndScrollHorizontally(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	tv := NewTimelineView()
	tv.SetRect(0, 0, 40, 8)
	tv.lanes = []TimelineLane{
		{Name: "read", StartTime: start, EndTime: &end},
		{Name: "write", StartTime: start, EndTime: &end},
	}
	tv.startTime = start
	tv.endTime = end
	tv.zoomLevel = 8
	tv.selectedLane = 1
	tv.scrollX = 3

	handler := tv.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(tview.Primitive) {})
	if tv.scrollX <= 3 {
		t.Fatalf("end should scroll to the horizontal end, scrollX=%d", tv.scrollX)
	}
	if tv.selectedLane != 1 {
		t.Fatalf("end should leave the selected lane, selected=%d", tv.selectedLane)
	}
	endX := tv.scrollX
	handler(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), func(tview.Primitive) {})
	if tv.scrollX != 0 || tv.selectedLane != 1 {
		t.Fatalf("home should return to the first column, scrollX=%d selected=%d end=%d", tv.scrollX, tv.selectedLane, endX)
	}
}
