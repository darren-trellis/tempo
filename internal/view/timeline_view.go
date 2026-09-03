package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	timelineMinWidth = 40
	timelineBarFill  = '\u00a0'
)

// TimelineLane represents a horizontal lane in the timeline.
type TimelineLane struct {
	Name      string
	Type      temporal.EventGroupType
	Status    string
	StartTime time.Time
	EndTime   *time.Time
	Node      *temporal.EventTreeNode
}

// TimelineView displays workflow events as a horizontal Gantt-style timeline.
type TimelineView struct {
	*tview.Box
	lanes             []TimelineLane
	startTime         time.Time
	endTime           time.Time
	scrollX           int
	scrollY           int
	zoomLevel         float64
	selectedLane      int
	onSelect          func(lane *TimelineLane)
	onSelectionChange func(lane *TimelineLane)
	mouseScrollStep   func() int
}

// NewTimelineView creates a new timeline/Gantt chart view.
func NewTimelineView() *TimelineView {
	tv := &TimelineView{
		Box:          tview.NewBox(),
		lanes:        []TimelineLane{},
		zoomLevel:    1.0,
		selectedLane: 0,
	}

	tv.SetBackgroundColor(tcell.ColorDefault)
	tv.SetBorder(false)

	return tv
}

// Destroy is a no-op kept for backward compatibility.
func (tv *TimelineView) Destroy() {}

func (tv *TimelineView) SetMouseScrollStep(fn func() int) *TimelineView {
	tv.mouseScrollStep = fn
	return tv
}

// SetNodes populates the timeline from event tree nodes.
func (tv *TimelineView) SetNodes(nodes []*temporal.EventTreeNode) {
	tv.lanes = nil
	tv.selectedLane = 0

	if len(nodes) == 0 {
		return
	}

	// First pass: collect valid lanes and find time range
	var validLanes []TimelineLane
	var minStart, maxEnd time.Time
	firstValid := true

	for _, node := range nodes {
		// Skip workflow-level events, only show activities/timers/child workflows
		if node.Type == temporal.GroupWorkflow || node.Type == temporal.GroupWorkflowTask {
			continue
		}

		// Skip nodes with zero/invalid start time
		if node.StartTime.IsZero() {
			continue
		}

		lane := TimelineLane{
			Name:      node.Name,
			Type:      node.Type,
			Status:    node.Status,
			StartTime: node.StartTime,
			EndTime:   node.EndTime,
			Node:      node,
		}
		validLanes = append(validLanes, lane)

		// Update time range
		if firstValid || node.StartTime.Before(minStart) {
			minStart = node.StartTime
		}
		if node.EndTime != nil && (firstValid || node.EndTime.After(maxEnd)) {
			maxEnd = *node.EndTime
		}
		firstValid = false
	}

	if len(validLanes) == 0 {
		return
	}

	prevID := int64(0)
	if lane := tv.SelectedLane(); lane != nil {
		prevID = timelineLaneScheduledID(*lane)
	}

	tv.lanes = validLanes
	tv.selectedLane = 0
	tv.startTime = minStart

	// Set end time: use max end time, or now for running items
	if maxEnd.IsZero() || maxEnd.Before(minStart) {
		tv.endTime = time.Now()
	} else {
		tv.endTime = maxEnd
	}

	// Ensure we have at least some time range
	if tv.endTime.Sub(tv.startTime) < time.Second {
		tv.endTime = tv.startTime.Add(time.Minute)
	}

	if prevID != 0 {
		tv.SelectByScheduledID(prevID)
	}
}

// Draw renders the timeline view.
// Colors are read dynamically at draw time.
func (tv *TimelineView) Draw(screen tcell.Screen) {
	// Read colors dynamically
	bgColor := theme.Bg()
	tv.SetBackgroundColor(bgColor)

	tv.Box.DrawForSubclass(screen, tv)

	x, y, width, height := tv.GetInnerRect()
	if width < timelineMinWidth || height < 3 {
		return
	}

	tv.drawHeader(screen, x, y, width)

	timeRange := tv.endTime.Sub(tv.startTime)
	if timeRange <= 0 {
		timeRange = time.Minute
	}

	visibleLanes := height - 3
	startLane := tv.scrollY
	endLane := startLane + visibleLanes
	if endLane > len(tv.lanes) {
		endLane = len(tv.lanes)
	}

	for i := startLane; i < endLane; i++ {
		lane := tv.lanes[i]
		laneY := y + 2 + (i - startLane)
		tv.drawLaneBar(screen, x, laneY, width, lane, timeRange, i == tv.selectedLane)
	}

	// Draw cursor line for selected lane
	if tv.selectedLane >= 0 && tv.selectedLane < len(tv.lanes) {
		tv.drawCursor(screen, x, y, width, height, timeRange)
	}

	// Draw legend at bottom if space
	if height > len(tv.lanes)+4 {
		tv.drawLegend(screen, x, y+height-1, width)
	}
}

// drawHeader draws the time scale header.
func (tv *TimelineView) drawHeader(screen tcell.Screen, x, y, width int) {
	if width <= 0 {
		return
	}

	timeRange := tv.endTime.Sub(tv.startTime)
	if timeRange <= 0 {
		return
	}

	markerCount := 5
	if width < 60 {
		markerCount = 3
	}

	tickStyle := tcell.StyleDefault.Foreground(theme.PanelTitle()).Background(theme.Bg())
	zoomLevel := tv.zoomLevel
	if zoomLevel < 0.1 {
		zoomLevel = 0.1
	}

	for i := 0; i <= markerCount; i++ {
		rawPos := width * i / markerCount
		pos := x + int(float64(rawPos)*zoomLevel) - tv.scrollX
		if pos < x || pos >= x+width {
			continue
		}

		effectivePos := int(float64(pos-x+tv.scrollX) / zoomLevel)
		if effectivePos < 0 {
			effectivePos = 0
		}
		if effectivePos > width {
			effectivePos = width
		}
		offset := roundDuration(time.Duration(float64(timeRange) * float64(effectivePos) / float64(width)))
		tview.Print(screen, formatRelativeDuration(offset), pos, y, 10, tview.AlignLeft, theme.FgDim())
		screen.SetContent(pos, y+1, '│', nil, tickStyle)
	}

	lineStyle := tcell.StyleDefault.Foreground(theme.Border()).Background(theme.Bg())
	for i := x; i < x+width; i++ {
		screen.SetContent(i, y+1, '─', nil, lineStyle)
	}
}

func (tv *TimelineView) laneBarSpan(lane TimelineLane, width int, timeRange time.Duration) (barStart, barEnd int) {
	startOffset := lane.StartTime.Sub(tv.startTime)
	barStart = int(float64(width) * float64(startOffset) / float64(timeRange))

	if lane.EndTime != nil {
		endOffset := lane.EndTime.Sub(tv.startTime)
		barEnd = int(float64(width) * float64(endOffset) / float64(timeRange))
	} else {
		barEnd = width
	}

	if barEnd <= barStart {
		barEnd = barStart + 1
	}

	barStart = int(float64(barStart)*tv.zoomLevel) - tv.scrollX
	barEnd = int(float64(barEnd)*tv.zoomLevel) - tv.scrollX
	if lane.EndTime != nil && barEnd > barStart {
		barEnd++
	}
	if barEnd <= barStart {
		barEnd = barStart + 1
	}
	if barStart < 0 {
		barStart = 0
	}
	if barEnd > width {
		barEnd = width
	}
	return barStart, barEnd
}

func barContainsCol(barStart, barEnd, col int) bool {
	return col >= barStart && col < barEnd && barEnd > barStart
}

func (tv *TimelineView) drawCursorLine(screen tcell.Screen, x, y, width, lanesEnd, selectedRow, col int, timeRange time.Duration, style tcell.Style, capSelected bool) {
	if col < 0 || col >= width {
		return
	}
	for row := y + 2; row < lanesEnd; row++ {
		if row == selectedRow {
			if capSelected {
				screen.SetContent(x+col, row, '│', nil, style.Background(theme.SelectionBg()))
			}
			continue
		}
		laneIdx := tv.scrollY + (row - y - 2)
		if laneIdx >= 0 && laneIdx < len(tv.lanes) {
			start, end := tv.laneBarSpan(tv.lanes[laneIdx], width, timeRange)
			if barContainsCol(start, end, col) {
				continue
			}
		}
		screen.SetContent(x+col, row, '│', nil, style)
	}
}

// drawLaneBar draws the timeline bar for a lane.
func (tv *TimelineView) drawLaneBar(screen tcell.Screen, x, y, width int, lane TimelineLane, timeRange time.Duration, selected bool) {
	if timeRange <= 0 || width <= 0 {
		return
	}

	barStart, barEnd := tv.laneBarSpan(lane, width, timeRange)

	barChar, barColor := tv.barStyle(lane)
	barStyle := tcell.StyleDefault.Foreground(theme.Bg()).Background(barColor)
	if selected {
		barStyle = tcell.StyleDefault.Foreground(theme.SelectionFg()).Background(theme.SelectionBg()).Bold(true)
	}

	emptyStyle := tcell.StyleDefault.Foreground(theme.BgLight()).Background(theme.Bg())
	for i := 0; i < barStart && i < width; i++ {
		screen.SetContent(x+i, y, '·', nil, emptyStyle)
	}

	barWidth := barEnd - barStart
	label := timelineBarName(lane)
	contents := timelineBarContents(barChar, label, barWidth)
	for i, ch := range contents {
		pos := barStart + i
		if pos < 0 || pos >= width {
			continue
		}
		screen.SetContent(x+pos, y, ch, nil, barStyle)
	}

	if barWidth < 4 && label != "" {
		outside := timelineBarLabelRunes(label, width-barEnd-1)
		pos := barEnd + 1
		for _, r := range outside {
			if pos >= width {
				break
			}
			screen.SetContent(x+pos, y, r, nil, barStyle)
			pos++
		}
	}

	for i := barEnd; i < width; i++ {
		mainc, _, _, _ := screen.GetContent(x+i, y)
		if mainc != ' ' && mainc != 0 {
			continue
		}
		screen.SetContent(x+i, y, '·', nil, emptyStyle)
	}
}

// drawCursor draws a candlestick-style cursor showing gap and duration for selected lane.
func (tv *TimelineView) drawCursor(screen tcell.Screen, x, y, width, height int, timeRange time.Duration) {
	if timeRange <= 0 || width <= 0 {
		return
	}

	lane := tv.lanes[tv.selectedLane]
	startOffset := lane.StartTime.Sub(tv.startTime)
	startPos, barEnd := tv.laneBarSpan(lane, width, timeRange)
	endPos := barEnd - 1

	var prevEndPos int
	if tv.selectedLane > 0 {
		_, prevEndPos = tv.laneBarSpan(tv.lanes[tv.selectedLane-1], width, timeRange)
	}

	// Calculate the row for the selected lane
	selectedRow := y + 2 + (tv.selectedLane - tv.scrollY)
	lanesEnd := y + 2 + (len(tv.lanes) - tv.scrollY)
	if lanesEnd > y+height-1 {
		lanesEnd = y + height - 1
	}

	// Draw gap "wick" from previous end to current start (thin line)
	if prevEndPos > 0 && prevEndPos < startPos {
		wickStyle := tcell.StyleDefault.Foreground(theme.FgDim()).Background(theme.Bg())
		for col := prevEndPos; col < startPos && col < width; col++ {
			if col >= 0 {
				screen.SetContent(x+col, selectedRow, '─', nil, wickStyle)
			}
		}
	}

	if startPos >= 0 && startPos < width {
		cursorStyle := tcell.StyleDefault.Foreground(theme.Accent()).Background(theme.Bg())
		tv.drawCursorLine(screen, x, y, width, lanesEnd, selectedRow, startPos, timeRange, cursorStyle, false)

		// Draw start time label in header
		startLabel := formatRelativeDuration(startOffset)
		labelStyle := tcell.StyleDefault.Foreground(theme.Bg()).Background(theme.Accent())
		labelX := x + startPos
		if labelX+len(startLabel) > x+width {
			labelX = x + width - len(startLabel)
		}
		if labelX < x {
			labelX = x
		}
		for i, r := range startLabel {
			if labelX+i >= x && labelX+i < x+width {
				screen.SetContent(labelX+i, y, r, nil, labelStyle)
			}
		}
	}

	// Only draw end marker and duration for completed items (those with an EndTime)
	if lane.EndTime != nil {
		duration := lane.EndTime.Sub(lane.StartTime)
		durationLabel := formatRelativeDuration(duration)
		durationStyle := tcell.StyleDefault.Foreground(theme.Bg()).Background(theme.Success())

		// Draw vertical line at end position (if visible and different from start)
		if endPos > startPos && endPos >= 0 && endPos < width {
			endStyle := tcell.StyleDefault.Foreground(theme.Success()).Background(theme.Bg())
			tv.drawCursorLine(screen, x, y, width, lanesEnd, selectedRow, endPos, timeRange, endStyle, true)
		}

		// Calculate available space inside the candlestick
		candleWidth := endPos - startPos
		startLabelLen := len(formatRelativeDuration(startOffset))

		var durationX int
		if candleWidth > len(durationLabel)+2 {
			// Fits inside - center it between start and end
			midPos := (startPos + endPos) / 2
			durationX = x + midPos - len(durationLabel)/2
			// Make sure it doesn't overlap with start label
			if durationX < x+startPos+startLabelLen+1 {
				durationX = x + startPos + startLabelLen + 1
			}
		} else {
			// Doesn't fit inside - place it to the right of end marker
			durationX = x + endPos + 1
		}

		// Clamp to screen bounds
		if durationX < x {
			durationX = x
		}
		if durationX+len(durationLabel) > x+width {
			durationX = x + width - len(durationLabel)
		}

		// Draw the duration label if it fits on screen
		if durationX >= x && durationX < x+width {
			for i, r := range durationLabel {
				if durationX+i >= x && durationX+i < x+width {
					screen.SetContent(durationX+i, y+1, r, nil, durationStyle)
				}
			}
		}
	}
}

// drawLegend draws the status legend and selected lane stats at the bottom.
func (tv *TimelineView) drawLegend(screen tcell.Screen, x, y, width int) {
	pos := x
	for _, typ := range timelineLegendTypes() {
		label := timelineTypeLabel(typ)
		if typ == temporal.GroupChildWorkflow {
			label = "Child"
		}
		if pos+1+len(label)+1 > x+width/2 {
			break
		}

		style := tcell.StyleDefault.Foreground(timelineTypeColor(typ)).Background(theme.Bg())
		screen.SetContent(pos, y, timelineTypeGlyph(typ), nil, style)
		pos++

		labelStyle := tcell.StyleDefault.Foreground(theme.FgDim()).Background(theme.Bg())
		for _, r := range label {
			screen.SetContent(pos, y, r, nil, labelStyle)
			pos++
		}
		pos += 1
	}

	// Draw selected lane stats on the right side
	if tv.selectedLane >= 0 && tv.selectedLane < len(tv.lanes) {
		lane := tv.lanes[tv.selectedLane]

		// Calculate stats
		startOffset := lane.StartTime.Sub(tv.startTime)

		// Calculate gap from previous lane
		var gap time.Duration
		if tv.selectedLane > 0 {
			prevLane := tv.lanes[tv.selectedLane-1]
			if prevLane.EndTime != nil {
				gap = lane.StartTime.Sub(*prevLane.EndTime)
				if gap < 0 {
					gap = 0
				}
			}
		}

		// Build stats segments with their colors
		type statSegment struct {
			text  string
			color tcell.Color
		}

		segments := []statSegment{}
		labelColor := theme.FgDim()

		// Start segment (accent color)
		segments = append(segments, statSegment{"Start:", labelColor})
		segments = append(segments, statSegment{formatRelativeDuration(startOffset), theme.Accent()})
		segments = append(segments, statSegment{"  ", labelColor})

		// Duration or running segment
		if lane.EndTime != nil {
			duration := lane.EndTime.Sub(lane.StartTime)
			segments = append(segments, statSegment{"Dur:", labelColor})
			segments = append(segments, statSegment{formatRelativeDuration(duration), theme.Success()})
		} else {
			segments = append(segments, statSegment{"(running)", theme.Warning()})
		}

		// Gap segment (dim color)
		if gap > 0 {
			segments = append(segments, statSegment{"  Gap:", labelColor})
			segments = append(segments, statSegment{formatRelativeDuration(gap), theme.FgDim()})
		}

		// Calculate total length
		totalLen := 0
		for _, seg := range segments {
			totalLen += len(seg.text)
		}

		// Draw stats right-aligned
		statsX := x + width - totalLen
		if statsX < pos+2 {
			statsX = pos + 2
		}

		currentX := statsX
		for _, seg := range segments {
			style := tcell.StyleDefault.Foreground(seg.color).Background(theme.Bg())
			for _, r := range seg.text {
				if currentX >= x+width {
					break
				}
				screen.SetContent(currentX, y, r, nil, style)
				currentX++
			}
		}
	}
}

// barStyle returns the bar character and color for a status.
func (tv *TimelineView) barStyle(lane TimelineLane) (rune, tcell.Color) {
	return timelineTypeGlyph(lane.Type), timelineStatusColor(lane.Status)
}

func timelineTypeGlyph(typ temporal.EventGroupType) rune {
	switch typ {
	case temporal.GroupActivity:
		return firstRune(theme.IconActivity)
	case temporal.GroupTimer:
		return firstRune(theme.IconClock)
	case temporal.GroupChildWorkflow:
		return firstRune(theme.IconWorkflow)
	case temporal.GroupSignal:
		return firstRune(theme.IconSignal)
	case temporal.GroupMarker:
		return firstRune(theme.IconTag)
	default:
		return firstRune(theme.IconEvent)
	}
}

func timelineTypeColor(typ temporal.EventGroupType) tcell.Color {
	switch typ {
	case temporal.GroupActivity:
		return theme.AccentDim()
	case temporal.GroupTimer:
		return theme.Warning()
	case temporal.GroupChildWorkflow, temporal.GroupWorkflow, temporal.GroupWorkflowTask:
		return theme.Info()
	case temporal.GroupSignal:
		return theme.Key()
	default:
		return theme.FgDim()
	}
}

func timelineTypeLabel(typ temporal.EventGroupType) string {
	switch typ {
	case temporal.GroupChildWorkflow:
		return "Child Workflow"
	case temporal.GroupWorkflowTask:
		return "Workflow Task"
	default:
		return typ.String()
	}
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return '▪'
}

func timelineStatusColor(status string) tcell.Color {
	switch status {
	case "Fired":
		return theme.Warning()
	case "Received":
		return theme.Key()
	case "Scheduled", "Initiated", "Pending":
		return theme.FgDim()
	default:
		return temporal.GetWorkflowStatus(status).Color()
	}
}

func timelineStatusLabel(status string) string {
	switch status {
	case "Received":
		return "Signaled"
	case "TimedOut":
		return "Timed Out"
	default:
		return status
	}
}

func timelineStatusGlyph(status string) string {
	switch status {
	case "Scheduled", "Initiated", "Pending":
		return "┄┄"
	default:
		return theme.IconDot
	}
}

func timelineBarContents(glyph rune, label string, width int) []rune {
	if width <= 0 {
		return nil
	}
	cells := make([]rune, width)
	for i := range cells {
		cells[i] = timelineBarFill
	}
	if width == 1 {
		return cells
	}
	cells[1] = glyph
	if width < 4 {
		return cells
	}
	name := timelineBarLabelRunes(label, width-3)
	copy(cells[3:], name)
	return cells
}

func timelineBarLabelRunes(label string, width int) []rune {
	name := []rune(fitTimelineName(label, width))
	for i, r := range name {
		if r == ' ' {
			name[i] = timelineBarFill
		}
	}
	return name
}

func timelineBarName(lane TimelineLane) string {
	name := strings.TrimSpace(lane.Name)
	switch {
	case strings.HasPrefix(name, "Activity: "):
		name = strings.TrimPrefix(name, "Activity: ")
	case strings.HasPrefix(name, "Timer: "):
		name = strings.TrimPrefix(name, "Timer: ")
	case strings.HasPrefix(name, "ChildWorkflow: "):
		name = strings.TrimPrefix(name, "ChildWorkflow: ")
	}
	return name
}

func fitTimelineName(name string, width int) string {
	if width <= 0 || name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) <= width {
		return name
	}
	if width == 1 {
		return string(runes[0])
	}
	return string(runes[:width-1]) + "…"
}

// InputHandler handles keyboard input.
func (tv *TimelineView) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return tv.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			tv.moveSelection(-1)
		case tcell.KeyDown:
			tv.moveSelection(1)
		case tcell.KeyLeft:
			tv.scroll(-5)
		case tcell.KeyRight:
			tv.scroll(5)
		case tcell.KeyEnter:
			if tv.onSelect != nil && tv.selectedLane >= 0 && tv.selectedLane < len(tv.lanes) {
				tv.onSelect(&tv.lanes[tv.selectedLane])
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k':
				tv.moveSelection(-1)
			case 'j':
				tv.moveSelection(1)
			case 'h':
				tv.scroll(-5)
			case 'l':
				tv.scroll(5)
			case '+', '=':
				tv.zoom(1.2)
			case '-':
				tv.zoom(0.8)
			case '0':
				tv.resetView()
			}
		}
	})
}

func (tv *TimelineView) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return tv.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event == nil {
			return false, nil
		}
		x, y := event.Position()
		if !tv.InRect(x, y) {
			return false, nil
		}
		if delta := horizontalMouseDelta(action, event); delta != 0 {
			tv.scroll(delta * resolveMouseScrollStep(tv.mouseScrollStep))
			return true, nil
		}
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftClick:
			if setFocus != nil {
				setFocus(tv)
			}
			return true, nil
		}
		return false, nil
	})
}

func timelineLaneScheduledID(lane TimelineLane) int64 {
	if lane.Node == nil {
		return 0
	}
	for _, ev := range lane.Node.Events {
		if ev == nil {
			continue
		}
		if ev.Type == "ActivityTaskScheduled" {
			return ev.ID
		}
	}
	if len(lane.Node.Events) > 0 && lane.Node.Events[0] != nil {
		if lane.Node.Events[0].ScheduledEventID != 0 {
			return lane.Node.Events[0].ScheduledEventID
		}
		return lane.Node.Events[0].ID
	}
	return 0
}

func (tv *TimelineView) ensureLaneVisible() {
	_, _, _, height := tv.GetInnerRect()
	visibleLanes := height - 3
	if visibleLanes < 1 {
		visibleLanes = 1
	}
	if tv.selectedLane < tv.scrollY {
		tv.scrollY = tv.selectedLane
	}
	if tv.selectedLane >= tv.scrollY+visibleLanes {
		tv.scrollY = tv.selectedLane - visibleLanes + 1
	}
	if tv.scrollY < 0 {
		tv.scrollY = 0
	}
}

func (tv *TimelineView) setSelectedLane(index int, notify bool) {
	if index < 0 || index >= len(tv.lanes) {
		return
	}
	changed := tv.selectedLane != index
	tv.selectedLane = index
	tv.ensureLaneVisible()
	if notify && changed && tv.onSelectionChange != nil {
		tv.onSelectionChange(&tv.lanes[tv.selectedLane])
	}
}

// SelectByScheduledID highlights the lane for the activity scheduled event.
func (tv *TimelineView) SelectByScheduledID(id int64) bool {
	if id == 0 {
		return false
	}
	for i, lane := range tv.lanes {
		if timelineLaneScheduledID(lane) == id {
			tv.setSelectedLane(i, false)
			return true
		}
	}
	return false
}

// moveSelection moves the lane selection up or down.
func (tv *TimelineView) moveSelection(delta int) {
	if len(tv.lanes) == 0 {
		return
	}
	next := tv.selectedLane + delta
	if next < 0 {
		next = 0
	}
	if next >= len(tv.lanes) {
		next = len(tv.lanes) - 1
	}
	tv.setSelectedLane(next, true)
}

// selectFirst jumps to the first lane.
func (tv *TimelineView) selectFirst() {
	if len(tv.lanes) == 0 {
		return
	}
	old := tv.selectedLane
	tv.selectedLane = 0
	tv.scrollY = 0
	if old != tv.selectedLane && tv.onSelectionChange != nil {
		tv.onSelectionChange(&tv.lanes[tv.selectedLane])
	}
}

// selectLast jumps to the last lane.
func (tv *TimelineView) selectLast() {
	if len(tv.lanes) == 0 {
		return
	}
	old := tv.selectedLane
	tv.selectedLane = len(tv.lanes) - 1

	// Adjust scroll to keep selection visible
	_, _, _, height := tv.GetInnerRect()
	visibleLanes := height - 3
	if tv.selectedLane >= tv.scrollY+visibleLanes {
		tv.scrollY = tv.selectedLane - visibleLanes + 1
	}

	if old != tv.selectedLane && tv.onSelectionChange != nil {
		tv.onSelectionChange(&tv.lanes[tv.selectedLane])
	}
}

func (tv *TimelineView) laneBarEnd(width int, lane TimelineLane, timeRange time.Duration) int {
	if width <= 0 || timeRange <= 0 {
		return 0
	}
	var barEnd int
	if lane.EndTime != nil {
		endOffset := lane.EndTime.Sub(tv.startTime)
		barEnd = int(float64(width) * float64(endOffset) / float64(timeRange))
	} else {
		barEnd = width
	}
	if barEnd < 1 {
		barEnd = 1
	}
	zoom := tv.zoomLevel
	if zoom < 0.1 {
		zoom = 0.1
	}
	return int(float64(barEnd) * zoom)
}

func (tv *TimelineView) contentWidth(width int) int {
	timeRange := tv.endTime.Sub(tv.startTime)
	if timeRange <= 0 {
		timeRange = time.Minute
	}
	end := 0
	for _, lane := range tv.lanes {
		if barEnd := tv.laneBarEnd(width, lane, timeRange); barEnd > end {
			end = barEnd
		}
	}
	return end
}

func (tv *TimelineView) maxScrollX() int {
	_, _, width, _ := tv.GetInnerRect()
	max := tv.contentWidth(width) - width
	if max < 0 {
		return 0
	}
	return max
}

func (tv *TimelineView) clampScrollX() {
	if tv.scrollX < 0 {
		tv.scrollX = 0
	}
	if max := tv.maxScrollX(); tv.scrollX > max {
		tv.scrollX = max
	}
}

func (tv *TimelineView) scroll(delta int) {
	tv.scrollX += delta
	tv.clampScrollX()
}

func (tv *TimelineView) zoom(factor float64) {
	tv.zoomLevel *= factor
	if tv.zoomLevel < 0.5 {
		tv.zoomLevel = 0.5
	}
	if tv.zoomLevel > 5.0 {
		tv.zoomLevel = 5.0
	}
	tv.clampScrollX()
}

// resetView resets zoom and scroll.
func (tv *TimelineView) resetView() {
	tv.zoomLevel = 1.0
	tv.scrollX = 0
	tv.scrollY = 0
}

// SetOnSelect sets the callback for lane selection (Enter key).
func (tv *TimelineView) SetOnSelect(fn func(lane *TimelineLane)) {
	tv.onSelect = fn
}

// SetOnSelectionChange sets the callback for when selection changes (navigation).
func (tv *TimelineView) SetOnSelectionChange(fn func(lane *TimelineLane)) {
	tv.onSelectionChange = fn
}

// SelectedLane returns the currently selected lane.
func (tv *TimelineView) SelectedLane() *TimelineLane {
	if tv.selectedLane >= 0 && tv.selectedLane < len(tv.lanes) {
		return &tv.lanes[tv.selectedLane]
	}
	return nil
}

// LaneCount returns the number of lanes.
func (tv *TimelineView) LaneCount() int {
	return len(tv.lanes)
}

// Focus implements tview.Primitive.
func (tv *TimelineView) Focus(delegate func(p tview.Primitive)) {
	tv.Box.Focus(delegate)
}

// HasFocus implements tview.Primitive.
func (tv *TimelineView) HasFocus() bool {
	return tv.Box.HasFocus()
}

// roundDuration rounds a duration up to a nice value.
func roundDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}

	// Define rounding thresholds and their round-to values
	type roundRule struct {
		threshold time.Duration
		roundTo   time.Duration
	}

	rules := []roundRule{
		{100 * time.Millisecond, 10 * time.Millisecond}, // < 100ms: round to 10ms
		{time.Second, 50 * time.Millisecond},            // < 1s: round to 50ms
		{10 * time.Second, 500 * time.Millisecond},      // < 10s: round to 500ms
		{time.Minute, time.Second},                      // < 1m: round to 1s
		{10 * time.Minute, 10 * time.Second},            // < 10m: round to 10s
		{time.Hour, time.Minute},                        // < 1h: round to 1m
		{24 * time.Hour, 10 * time.Minute},              // < 24h: round to 10m
	}

	for _, rule := range rules {
		if d < rule.threshold {
			// Round up to nearest roundTo
			return ((d + rule.roundTo - 1) / rule.roundTo) * rule.roundTo
		}
	}

	// For very long durations, round to nearest hour
	return ((d + time.Hour - 1) / time.Hour) * time.Hour
}

// formatRelativeDuration formats a duration as a relative time string.
func formatRelativeDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		secs := d.Seconds()
		if secs == float64(int(secs)) {
			return fmt.Sprintf("%ds", int(secs))
		}
		return fmt.Sprintf("%.1fs", secs)
	}
	if d < time.Hour {
		mins := d.Minutes()
		if mins == float64(int(mins)) {
			return fmt.Sprintf("%dm", int(mins))
		}
		return fmt.Sprintf("%.1fm", mins)
	}
	hours := d.Hours()
	if hours == float64(int(hours)) {
		return fmt.Sprintf("%dh", int(hours))
	}
	return fmt.Sprintf("%.1fh", hours)
}
