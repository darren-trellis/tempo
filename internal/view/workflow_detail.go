package view

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/input"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// WorkflowDetail displays detailed information about a workflow with events.
type WorkflowDetail struct {
	*tview.Flex
	app              *App
	workflowID       string
	runID            string
	workflow         *temporal.Workflow
	allEvents        []temporal.EnhancedHistoryEvent // Full unfiltered list
	events           []temporal.EnhancedHistoryEvent // Filtered list for display
	leftFlex         *tview.Flex
	workflowPanel    *components.Panel
	eventDetailPanel *components.Panel
	eventsPanel      *components.Panel
	workflowView     *tview.TextView
	eventDetailView  *tview.TextView
	eventTable       *components.Table
	eventTableScroll *charScrollView
	focusPane        detailFocusPane
	loading          bool
	searchText       string // Current search filter text
	baseEventsTitle  string // Base title without search suffix
}

// NewWorkflowDetail creates a new workflow detail view.
func NewWorkflowDetail(app *App, workflowID, runID string) *WorkflowDetail {
	wd := &WorkflowDetail{
		Flex:       tview.NewFlex().SetDirection(tview.FlexColumn),
		app:        app,
		workflowID: workflowID,
		runID:      runID,
		eventTable: components.NewTable(),
	}
	wd.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(wd)

	return wd
}

func (wd *WorkflowDetail) setup() {
	wd.SetBackgroundColor(theme.Bg())

	// Combined workflow info view
	wd.workflowView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true)
	wd.workflowView.SetBackgroundColor(theme.Bg())
	attachTextViewScrollbar(wd.workflowView, wd.app)

	// Event detail view
	wd.eventDetailView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetScrollable(true).
		SetWordWrap(true)
	wd.eventDetailView.SetBackgroundColor(theme.Bg())
	attachTextViewScrollbar(wd.eventDetailView, wd.app)

	// Event table
	wd.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")
	wd.eventTable.SetBorder(false)
	wd.eventTable.SetBackgroundColor(theme.Bg())
	wd.eventTable.SetEvaluateAllRows(true)
	wd.eventTableScroll = attachTableCharScroll(wd.eventTable, wd.app)

	// Create panels with icons (blubber pattern)
	wd.workflowPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Workflow", theme.IconWorkflow))
	wd.workflowPanel.SetContent(wd.workflowView)

	wd.eventDetailPanel = components.NewPanel().SetTitle(fmt.Sprintf("%s Event Detail", theme.IconInfo))
	wd.eventDetailPanel.SetContent(wd.eventDetailView)

	wd.baseEventsTitle = fmt.Sprintf("%s Events", theme.IconEvent)
	wd.eventsPanel = components.NewPanel().SetTitle(wd.baseEventsTitle)
	wd.eventsPanel.SetContent(wd.eventTableScroll)

	// Left side: workflow info + event detail stacked
	wd.leftFlex = tview.NewFlex().SetDirection(tview.FlexRow)
	wd.leftFlex.SetBackgroundColor(theme.Bg())
	wd.leftFlex.AddItem(wd.workflowPanel, 0, 1, false)
	wd.leftFlex.AddItem(wd.eventDetailPanel, 0, 1, false)

	// Main layout: left stack + right events
	wd.AddItem(wd.leftFlex, 0, 2, false)
	wd.AddItem(wd.eventsPanel, 0, 3, true)

	// Update event detail when selection changes
	wd.eventTable.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 && row-1 < len(wd.events) {
			wd.updateEventDetail(wd.events[row-1])
		}
	})

	// Show loading state initially
	wd.workflowView.SetText(fmt.Sprintf("\n [%s]Loading...[-]", theme.TagFgDim()))
	wd.setupPaneInput(wd.workflowView)
	wd.setupPaneInput(wd.eventDetailView)
	wd.applyFocusStyles()
}

func (wd *WorkflowDetail) setLoading(loading bool) {
	wd.loading = loading
	wd.app.SetViewLoading("workflow-detail", loading)
}

func (wd *WorkflowDetail) applyFilter(query string) {
	wd.searchText = query
	wd.updateEventsTitle()
	wd.events = filterHistoryEvents(wd.allEvents, query)
	wd.populateEventTable()
}

func (wd *WorkflowDetail) updateEventsTitle() {
	if wd.searchText == "" {
		wd.eventsPanel.SetTitle(wd.baseEventsTitle)
	} else {
		wd.eventsPanel.SetTitle(wd.baseEventsTitle + " (/" + wd.searchText + ")")
	}
}

func (wd *WorkflowDetail) showSearch() {
	wd.app.ShowFilterMode(wd.searchText, FilterModeCallbacks{
		OnChange: func(text string) {
			wd.applyFilter(text)
		},
		OnSubmit: func(text string) {
			wd.applyFilter(text)
		},
		OnCancel: func() {},
	})
}

// RefreshTheme updates all component colors after a theme change.
func (wd *WorkflowDetail) RefreshTheme() {
	bg := theme.Bg()
	fg := theme.Fg()

	// Update main container
	wd.SetBackgroundColor(bg)

	// Update text views
	wd.workflowView.SetBackgroundColor(bg)
	wd.workflowView.SetTextColor(fg)
	wd.eventDetailView.SetBackgroundColor(bg)
	wd.eventDetailView.SetTextColor(fg)

	// Update table
	wd.eventTable.SetBackgroundColor(bg)

	// Update flex containers
	wd.leftFlex.SetBackgroundColor(bg)

	// Re-render content with new theme colors
	wd.render()
	wd.populateEventTable()
	wd.applyFocusStyles()
}

func (wd *WorkflowDetail) loadData() {
	provider := wd.app.Provider()
	if provider == nil {
		wd.loadMockData()
		return
	}

	namespace := wd.app.CurrentNamespace()
	wd.setLoading(true)

	// Load workflow first, then events sequentially to avoid overwhelming the connection
	go func() {
		// Step 1: Load workflow metadata with retry
		var workflow *temporal.Workflow
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				// Exponential backoff: 500ms, 1s, 2s
				time.Sleep(time.Duration(250<<attempt) * time.Millisecond)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			workflow, err = provider.GetWorkflow(ctx, namespace, wd.workflowID, wd.runID)
			cancel()
			if err == nil {
				break
			}
		}

		if err != nil {
			wd.app.JigApp().QueueUpdateDraw(func() {
				wd.setLoading(false)
				wd.showError(err)
			})
			return
		}

		wd.app.JigApp().QueueUpdateDraw(func() {
			wd.workflow = workflow
			wd.render()
			wd.app.JigApp().Menu().SetHints(wd.Hints())
		})

		// Step 2: Load events after workflow succeeds (with retry)
		var events []temporal.EnhancedHistoryEvent
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Duration(250<<attempt) * time.Millisecond)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			events, err = provider.GetEnhancedWorkflowHistory(ctx, namespace, wd.workflowID, wd.runID)
			cancel()
			if err == nil {
				break
			}
		}

		wd.app.JigApp().QueueUpdateDraw(func() {
			wd.setLoading(false)
			if err != nil {
				// Show workflow info even if events fail
				return
			}
			wd.allEvents = events
			wd.events = events
			wd.populateEventTable()

			// Extract input/output from events
			if wd.workflow != nil {
				wd.extractWorkflowIO()
			}
		})
	}()
}

// extractWorkflowIO extracts input/output from the loaded events and updates the workflow.
// This avoids a redundant API call since we already have the full event history.
func (wd *WorkflowDetail) extractWorkflowIO() {
	if wd.workflow == nil {
		return
	}
	wd.workflow.Input, wd.workflow.Output = workflowIOFromEvents(wd.allEvents)
}

func (wd *WorkflowDetail) loadMockData() {
	now := time.Now()
	wd.workflow = &temporal.Workflow{
		ID:        wd.workflowID,
		RunID:     wd.runID,
		Type:      "MockWorkflow",
		Status:    "Running",
		Namespace: wd.app.CurrentNamespace(),
		TaskQueue: "mock-tasks",
		StartTime: now.Add(-5 * time.Minute),
	}
	wd.allEvents = []temporal.EnhancedHistoryEvent{
		{ID: 1, Type: "WorkflowExecutionStarted", Time: now.Add(-5 * time.Minute), Details: "WorkflowType: MockWorkflow, TaskQueue: mock-tasks"},
		{ID: 2, Type: "WorkflowTaskScheduled", Time: now.Add(-5 * time.Minute), Details: "TaskQueue: mock-tasks"},
		{ID: 3, Type: "WorkflowTaskStarted", Time: now.Add(-5 * time.Minute), Details: "Identity: worker-1@host"},
		{ID: 4, Type: "WorkflowTaskCompleted", Time: now.Add(-5 * time.Minute), Details: "ScheduledEventId: 2"},
		{ID: 5, Type: "ActivityTaskScheduled", Time: now.Add(-4 * time.Minute), Details: "ActivityType: MockActivity, TaskQueue: mock-tasks", ActivityType: "MockActivity"},
		{ID: 6, Type: "ActivityTaskStarted", Time: now.Add(-4 * time.Minute), Details: "Identity: worker-1@host, Attempt: 1", ActivityType: "MockActivity", ScheduledEventID: 5},
		{ID: 7, Type: "ActivityTaskCompleted", Time: now.Add(-3 * time.Minute), Details: "ScheduledEventId: 5, Result: {success: true}", ActivityType: "MockActivity", ScheduledEventID: 5},
	}
	wd.events = wd.allEvents
	wd.render()
	wd.populateEventTable()
}

func (wd *WorkflowDetail) showError(err error) {
	wd.workflowView.SetText(fmt.Sprintf("\n [%s]Error: %s[-]", theme.TagError(), err.Error()))
	wd.eventDetailView.SetText("")
}

func (wd *WorkflowDetail) render() {
	if wd.workflow == nil {
		wd.workflowView.SetText(fmt.Sprintf(" [%s]Workflow not found[-]", theme.TagError()))
		return
	}

	wd.workflowView.SetText(formatWorkflowInfo(*wd.workflow))
}

const (
	workflowInfoID        = "ID"
	workflowInfoParent    = "Parent"
	workflowInfoType      = "Type"
	workflowInfoStatus    = "Status"
	workflowInfoStarted   = "Started"
	workflowInfoDuration  = "Duration"
	workflowInfoTaskQueue = "Task Queue"
	workflowInfoRunID     = "Run ID"
	workflowInfoLabelPad  = 13
)

type workflowInfoRow struct {
	Key      string
	Label    string
	Value    string
	Display  string
	Color    tcell.Color
	ColorTag string
}

func workflowInfoRows(now time.Time, w temporal.Workflow) []workflowInfoRow {
	statusHandle := temporal.GetWorkflowStatus(w.Status)
	durationStr := "In progress"
	if w.EndTime != nil {
		durationStr = w.EndTime.Sub(w.StartTime).Round(time.Second).String()
	} else if w.Status == "Running" {
		durationStr = now.Sub(w.StartTime).Round(time.Second).String()
	}

	rows := []workflowInfoRow{
		{Key: workflowInfoID, Label: "ID", Value: w.ID, Color: theme.Fg(), ColorTag: theme.TagFg()},
	}
	if w.ParentID != nil && *w.ParentID != "" {
		rows = append(rows, workflowInfoRow{
			Key:      workflowInfoParent,
			Label:    "Parent",
			Value:    *w.ParentID,
			Color:    theme.Fg(),
			ColorTag: theme.TagFg(),
		})
	}
	rows = append(rows,
		workflowInfoRow{Key: workflowInfoType, Label: "Type", Value: w.Type, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{
			Key:      workflowInfoStatus,
			Label:    "Status",
			Value:    w.Status,
			Display:  statusHandle.Icon() + " " + w.Status,
			Color:    statusHandle.Color(),
			ColorTag: statusHandle.ColorTag(),
		},
		workflowInfoRow{Key: workflowInfoStarted, Label: "Started", Value: formatRelativeTime(now, w.StartTime), Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoDuration, Label: "Duration", Value: durationStr, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoTaskQueue, Label: "Task Queue", Value: w.TaskQueue, Color: theme.Fg(), ColorTag: theme.TagFg()},
		workflowInfoRow{Key: workflowInfoRunID, Label: "Run ID", Value: w.RunID, Color: theme.FgDim(), ColorTag: theme.TagFgDim()},
	)
	return rows
}

func (r workflowInfoRow) displayText() string {
	if r.Display != "" {
		return r.Display
	}
	return r.Value
}

func workflowInfoContentWidth(rows []workflowInfoRow) int {
	labelWidth := 0
	valueWidth := 0
	for _, row := range rows {
		if n := len(row.Label); n > labelWidth {
			labelWidth = n
		}
		if n := len(row.displayText()); n > valueWidth {
			valueWidth = n
		}
	}
	if labelWidth == 0 && valueWidth == 0 {
		return 0
	}
	return labelWidth + 1 + valueWidth
}

func formatWorkflowInfo(w temporal.Workflow) string {
	rows := workflowInfoRows(time.Now(), w)
	var b strings.Builder
	b.WriteByte('\n')
	for i, row := range rows {
		label := row.Label
		if len(label) < workflowInfoLabelPad {
			label += strings.Repeat(" ", workflowInfoLabelPad-len(label))
		}
		fmt.Fprintf(&b, "[%s::b]%s[-:-:-] [%s]%s[-]", theme.TagFgDim(), label, row.ColorTag, row.displayText())
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (wd *WorkflowDetail) updateEventDetail(ev temporal.EnhancedHistoryEvent) {
	wd.eventDetailView.SetText(formatSelectedEventDetail(ev))
}

// formatEventDetails parses event details and formats them with pretty JSON.
func formatEventDetails(details string) string {
	if details == "" {
		return fmt.Sprintf("[%s]No details[-]", theme.TagFgDim())
	}

	// First check if the whole thing is JSON
	trimmed := strings.TrimSpace(details)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		formatted := formatJSONPretty(details)
		return highlightFormattedJSONWorkflow(formatted)
	}

	// Handle key-value format with embedded JSON
	return formatKeyValueDetailsWorkflow(details)
}

// formatKeyValueDetailsWorkflow formats key-value style details with embedded JSON.
func formatKeyValueDetailsWorkflow(details string) string {
	var result strings.Builder

	// Split by commas while preserving JSON objects
	parts := splitPreservingJSONWorkflow(details)

	// First pass: find max key length for alignment
	type kvPair struct {
		key   string
		value string
	}
	var pairs []kvPair
	maxKeyLen := 0

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the key-value split point (first colon not inside JSON)
		colonIdx := findKeyColonIndex(part)
		if colonIdx > 0 {
			key := strings.TrimSpace(part[:colonIdx])
			value := strings.TrimSpace(part[colonIdx+1:])
			pairs = append(pairs, kvPair{key, value})
			if len(key) > maxKeyLen {
				maxKeyLen = len(key)
			}
		} else {
			pairs = append(pairs, kvPair{"", part})
		}
	}

	// Second pass: format with aligned keys
	for i, kv := range pairs {
		if i > 0 {
			result.WriteString("\n")
		}

		if kv.key != "" {
			// Pad key for alignment
			paddedKey := kv.key + strings.Repeat(" ", maxKeyLen-len(kv.key))

			// Check if value is JSON
			value := strings.TrimSpace(kv.value)
			if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
				formatted := formatJSONPretty(value)
				if formatted != value {
					// JSON was successfully formatted - put it on next line at left margin
					result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]\n", theme.TagFgDim(), paddedKey))
					result.WriteString(highlightFormattedJSONWorkflow(formatted))
				} else {
					result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]  ", theme.TagFgDim(), paddedKey))
					result.WriteString(highlightJSONLineWorkflow(value))
				}
			} else {
				result.WriteString(fmt.Sprintf("[%s::b]%s[-:-:-]  ", theme.TagFgDim(), paddedKey))
				result.WriteString(fmt.Sprintf("[%s]%s[-]", theme.TagFg(), highlightValuesWorkflow(value)))
			}
		} else {
			result.WriteString(fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(kv.value)))
		}
	}

	return result.String()
}

// splitPreservingJSONWorkflow splits a string by commas while preserving JSON objects.
func splitPreservingJSONWorkflow(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for _, ch := range s {
		switch ch {
		case '{', '[':
			depth++
			current.WriteRune(ch)
		case '}', ']':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// findKeyColonIndex finds the index of the colon that separates key from value.
// It ignores colons inside JSON objects or strings.
func findKeyColonIndex(s string) int {
	depth := 0
	inString := false
	for i, ch := range s {
		switch ch {
		case '"':
			inString = !inString
		case '{', '[':
			if !inString {
				depth++
			}
		case '}', ']':
			if !inString {
				depth--
			}
		case ':':
			if depth == 0 && !inString {
				return i
			}
		}
	}
	return -1
}

// formatJSONPretty attempts to format a string as pretty JSON.
func formatJSONPretty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return s
	}

	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return s
	}

	return string(pretty)
}

// highlightFormattedJSONWorkflow applies syntax highlighting to formatted JSON.
func highlightFormattedJSONWorkflow(formatted string) string {
	lines := strings.Split(formatted, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, highlightJSONLineWorkflow(line))
	}
	return strings.Join(result, "\n")
}

// highlightJSONLineWorkflow highlights a single line of JSON content.
func highlightJSONLineWorkflow(line string) string {
	// Check for key: value pattern
	if colonIdx := strings.Index(line, ":"); colonIdx > 0 {
		prefix := line[:colonIdx]
		suffix := line[colonIdx+1:]

		trimmed := strings.TrimSpace(prefix)
		if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
			return fmt.Sprintf("[%s]%s[-]:[%s]%s[-]", theme.TagAccent(), escapeForTView(prefix), theme.TagFg(), highlightValuesWorkflow(suffix))
		}
	}

	return highlightValuesWorkflow(line)
}

// escapeForTView removes '[' so tview cannot parse payload bytes as style tags.
// tview.Escape only handles already-closed tags like [red], not raw '['.
func escapeForTView(s string) string {
	return strings.ReplaceAll(s, "[", "［")
}

// highlightValuesWorkflow highlights JSON values (booleans, null).
func highlightValuesWorkflow(s string) string {
	result := escapeForTView(s)
	result = strings.ReplaceAll(result, "true", fmt.Sprintf("[%s]true[-]", temporal.StatusCompleted.ColorTag()))
	result = strings.ReplaceAll(result, "false", fmt.Sprintf("[%s]false[-]", temporal.StatusFailed.ColorTag()))
	result = strings.ReplaceAll(result, "null", fmt.Sprintf("[%s]null[-]", theme.TagFgDim()))
	return result
}

func (wd *WorkflowDetail) populateEventTable() {
	// Preserve current selection
	currentRow := wd.eventTable.SelectedRow()

	wd.eventTable.ClearRows()
	wd.eventTable.SetHeaders("ID", "TIME", "TYPE", "NAME")

	for _, ev := range wd.events {
		icon := eventIcon(ev.Type)
		color := eventColor(ev.Type)
		name := getEventNameDetail(&ev)
		wd.eventTable.AddRowWithColor(color,
			fmt.Sprintf("%d", ev.ID),
			ev.Time.Format("15:04:05"),
			icon+" "+truncateStr(ev.Type, 30),
			name,
		)
	}

	if wd.eventTable.RowCount() > 0 {
		// Restore previous selection if valid, otherwise select first row
		if currentRow >= 0 && currentRow < len(wd.events) {
			wd.eventTable.SelectRow(currentRow)
			wd.updateEventDetail(wd.events[currentRow])
		} else {
			wd.eventTable.SelectRow(0)
			if len(wd.events) > 0 {
				wd.updateEventDetail(wd.events[0])
			}
		}
	}
}

// getEventNameDetail returns the activity type, timer ID, or child workflow type for an event.
func getEventNameDetail(ev *temporal.EnhancedHistoryEvent) string {
	if ev.ActivityType != "" {
		return ev.ActivityType
	}
	if ev.TimerID != "" {
		return "Timer: " + ev.TimerID
	}
	if ev.ChildWorkflowType != "" {
		return ev.ChildWorkflowType
	}
	return ""
}

// CommandContext returns the workflow ID, run ID, and type for command variable expansion.
func (wd *WorkflowDetail) CommandContext() (workflowID, runID, workflowType string) {
	wfType := ""
	if wd.workflow != nil {
		wfType = wd.workflow.Type
	}
	return wd.workflowID, wd.runID, wfType
}

// Name returns the view name.
func (wd *WorkflowDetail) Name() string {
	return "workflow-detail"
}

// Start is called when the view becomes active.
func (wd *WorkflowDetail) Start() {
	bindings := input.NewKeyBindings().
		OnRune('/', func(e *tcell.EventKey) bool {
			wd.showSearch()
			return true
		}).
		OnRune('r', func(e *tcell.EventKey) bool {
			wd.loadData()
			return true
		}).
		OnRune('y', func(e *tcell.EventKey) bool {
			wd.yankEventData()
			return true
		}).
		OnRune('c', func(e *tcell.EventKey) bool {
			wd.showCancelConfirm()
			return true
		}).
		OnRune('X', func(e *tcell.EventKey) bool {
			wd.showTerminateConfirm()
			return true
		}).
		OnRune('s', func(e *tcell.EventKey) bool {
			wd.showSignalInput()
			return true
		}).
		OnRune('D', func(e *tcell.EventKey) bool {
			wd.showDeleteConfirm()
			return true
		}).
		OnRune('R', func(e *tcell.EventKey) bool {
			wd.showResetSelector()
			return true
		}).
		OnRune('Q', func(e *tcell.EventKey) bool {
			wd.showQueryInput()
			return true
		}).
		OnRune('i', func(e *tcell.EventKey) bool {
			wd.showIOModal()
			return true
		}).
		OnRune('g', func(e *tcell.EventKey) bool {
			wd.jumpToChildWorkflow()
			return true
		}).
		OnRune('N', func(e *tcell.EventKey) bool {
			wd.showStartWorkflow()
			return true
		}).
		OnRune('u', func(e *tcell.EventKey) bool {
			if wd.app != nil {
				wd.app.OpenWorkflowInBrowser(wd.workflowID, wd.runID)
			}
			return true
		}).
		On(tcell.KeyTab, func(e *tcell.EventKey) bool {
			wd.cycleFocus(1)
			return true
		}).
		On(tcell.KeyBacktab, func(e *tcell.EventKey) bool {
			wd.cycleFocus(-1)
			return true
		})

	wd.eventTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if handleTableCharScroll(wd.eventTableScroll, wd.eventTable, event) {
			return nil
		}
		if bindings.Handle(event) {
			return nil
		}
		return event
	})
	wd.setupPaneInput(wd.workflowView)
	wd.setupPaneInput(wd.eventDetailView)
	wd.loadData()
}

// Stop is called when the view is deactivated.
func (wd *WorkflowDetail) Stop() {
	wd.eventTable.SetInputCapture(nil)
}

// Hints returns keybinding hints for this view.
func (wd *WorkflowDetail) Hints() []KeyHint {
	switch wd.focusPane {
	case detailFocusEventDetail:
		return []KeyHint{
			{Key: "esc", Description: "Events"},
		}
	case detailFocusWorkflow:
		return []KeyHint{
			{Key: "esc", Description: "Events"},
		}
	}

	hints := []KeyHint{
		{Key: "/", Description: "Search"},
		{Key: "i", Description: "Input/Output"},
		{Key: "g", Description: "Go to Child"},
		{Key: "y", Description: "Yank"},
		{Key: "u", Description: "Web UI"},
		{Key: "r", Description: "Refresh"},
	}

	// Only show mutation hints if workflow is running
	if wd.workflow != nil && wd.workflow.Status == "Running" {
		hints = append(hints,
			KeyHint{Key: "c", Description: "Cancel"},
			KeyHint{Key: "X", Description: "Terminate"},
			KeyHint{Key: "s", Description: "Signal"},
			KeyHint{Key: "Q", Description: "Query"},
		)
	}

	// Reset is available for completed/failed workflows
	if wd.workflow != nil && (wd.workflow.Status == "Completed" || wd.workflow.Status == "Failed" || wd.workflow.Status == "Terminated" || wd.workflow.Status == "Canceled") {
		hints = append(hints, KeyHint{Key: "R", Description: "Reset"})
	}

	hints = append(hints,
		KeyHint{Key: "N", Description: "Start"},
		KeyHint{Key: "D", Description: "Delete"},
		KeyHint{Key: "T", Description: "Theme"},
		KeyHint{Key: "esc", Description: "Back"},
	)

	return hints
}

// Focus sets focus to the active pane.
func (wd *WorkflowDetail) Focus(delegate func(p tview.Primitive)) {
	switch wd.focusPane {
	case detailFocusEventDetail:
		delegate(wd.eventDetailView)
	case detailFocusWorkflow:
		delegate(wd.workflowView)
	default:
		delegate(wd.eventTable)
	}
}

// Draw applies theme colors dynamically and draws the view.
func (wd *WorkflowDetail) Draw(screen tcell.Screen) {
	bg := theme.Bg()
	wd.SetBackgroundColor(bg)
	wd.leftFlex.SetBackgroundColor(bg)
	wd.workflowView.SetBackgroundColor(bg)
	wd.eventDetailView.SetBackgroundColor(bg)
	wd.syncFocusFromPrimitives()
	wd.Flex.Draw(screen)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Mutation methods

func (wd *WorkflowDetail) actionTarget() workflowActionTarget {
	return workflowActionTarget{
		Namespace: wd.app.CurrentNamespace(),
		ID:        wd.workflowID,
		RunID:     wd.runID,
	}
}

func (wd *WorkflowDetail) showCancelConfirm() {
	showCancelWorkflowModal(wd.app, wd.actionTarget(), wd.loadData)
}

func (wd *WorkflowDetail) showTerminateConfirm() {
	showTerminateWorkflowModal(wd.app, wd.actionTarget(), wd.loadData)
}

func (wd *WorkflowDetail) showDeleteConfirm() {
	showDeleteWorkflowModal(wd.app, wd.actionTarget(), func() {
		wd.app.JigApp().Pages().Pop()
	})
}

func (wd *WorkflowDetail) showSignalInput() {
	showSignalWorkflowModal(wd.app, wd.actionTarget(), wd.loadData)
}

// showStartWorkflow displays the start workflow modal pre-filled from the current workflow.
func (wd *WorkflowDetail) showStartWorkflow() {
	var prefill startWorkflowPrefill
	if wd.workflow != nil {
		prefill = startWorkflowPrefill{
			WorkflowID:   wd.workflow.ID,
			WorkflowType: wd.workflow.Type,
			TaskQueue:    wd.workflow.TaskQueue,
			Input:        wd.workflow.Input,
		}
	}

	showStartWorkflowModal(wd.app, prefill)
}

func (wd *WorkflowDetail) showResetSelector() {
	showResetWorkflowModal(wd.app, wd.actionTarget(), func(newRunID string) {
		wd.runID = newRunID
		wd.loadData()
	})
}

func (wd *WorkflowDetail) closeModal() {
	wd.app.JigApp().Pages().DismissModal()
}

func (wd *WorkflowDetail) showQueryInput() {
	showQueryWorkflowModal(wd.app, wd.actionTarget())
}

// getSelectedEventDetails returns the details for the currently selected event.
func (wd *WorkflowDetail) getSelectedEventDetails() (string, string) {
	row := wd.eventTable.SelectedRow()
	if row < 0 || row >= len(wd.events) {
		return "", ""
	}
	ev := wd.events[row]
	return ev.Type, formatWorkflowEventDataRaw(&ev)
}

func formatWorkflowEventDataRaw(ev *temporal.EnhancedHistoryEvent) string {
	if ev == nil {
		return ""
	}

	var parts []string
	if ev.Details != "" {
		parts = append(parts, fmt.Sprintf("Details: %s", prettyPrintJSONDetail(ev.Details)))
	}
	if ev.Result != "" {
		parts = append(parts, fmt.Sprintf("Result: %s", prettyPrintJSONDetail(ev.Result)))
	}
	if ev.Failure != "" {
		parts = append(parts, fmt.Sprintf("Failure: %s", prettyPrintJSONDetail(ev.Failure)))
	}
	if ev.FailureSource != "" {
		parts = append(parts, fmt.Sprintf("Source: %s", ev.FailureSource))
	}
	if ev.FailureStackTrace != "" {
		parts = append(parts, fmt.Sprintf("Stack Trace:\n%s", ev.FailureStackTrace))
	}
	if ev.FailureCause != "" {
		parts = append(parts, fmt.Sprintf("Cause:\n%s", ev.FailureCause))
	}

	return strings.Join(parts, "\n\n")
}

// yankEventData copies the selected event's details to clipboard.
func (wd *WorkflowDetail) yankEventData() {
	eventType, data := wd.getSelectedEventDetails()
	if data == "" {
		return
	}

	if err := copyToClipboard(data); err != nil {
		wd.eventDetailView.SetText(fmt.Sprintf("[%s]%s Failed to copy: %s[-]",
			theme.TagError(), theme.IconError, err.Error()))
		return
	}

	// Show success feedback
	wd.eventDetailView.SetText(fmt.Sprintf(`
[%s::b]Copied to clipboard[-:-:-]

[%s]%s[-]

[%s]%s[-]`,
		theme.TagAccent(),
		theme.TagAccent(), eventType,
		temporal.StatusCompleted.ColorTag(), "Event data copied!"))

	// Restore detail after a brief delay
	go func() {
		time.Sleep(1500 * time.Millisecond)
		wd.app.JigApp().QueueUpdateDraw(func() {
			row := wd.eventTable.SelectedRow()
			if row >= 0 && row < len(wd.events) {
				wd.updateEventDetail(wd.events[row])
			}
		})
	}()
}

// prettyPrintJSONDetail attempts to format JSON in the details string.
func prettyPrintJSONDetail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Try to parse the whole thing as JSON first
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		var parsed interface{}
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			pretty, err := json.MarshalIndent(parsed, "", "  ")
			if err == nil {
				return string(pretty)
			}
		}
	}

	// Otherwise, try to find and format JSON embedded in the string
	// Look for patterns like "Result: {...}" or "Input: {...}"
	var result strings.Builder
	parts := strings.Split(s, ", ")
	for i, part := range parts {
		if i > 0 {
			result.WriteString("\n")
		}

		// Check if this part has embedded JSON
		if colonIdx := strings.Index(part, ": "); colonIdx > 0 {
			key := part[:colonIdx]
			value := part[colonIdx+2:]

			// Try to parse and pretty-print the value as JSON
			if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
				var parsed interface{}
				if err := json.Unmarshal([]byte(value), &parsed); err == nil {
					pretty, err := json.MarshalIndent(parsed, "", "  ")
					if err == nil {
						result.WriteString(fmt.Sprintf("%s:\n%s", key, string(pretty)))
						continue
					}
				}
			}
			result.WriteString(fmt.Sprintf("%s: %s", key, value))
		} else {
			result.WriteString(part)
		}
	}

	return result.String()
}

// formatDetailViewWithHighlighting adds color tags for syntax highlighting.
func formatDetailViewWithHighlighting(data string) string {
	lines := strings.Split(data, "\n")
	var result []string

	for _, line := range lines {
		highlighted := highlightDetailLine(line)
		result = append(result, highlighted)
	}

	return strings.Join(result, "\n")
}

// highlightDetailLine adds tview color tags to a single line.
func highlightDetailLine(line string) string {
	// If line contains a colon that looks like a JSON key
	if idx := strings.Index(line, ":"); idx > 0 {
		prefix := line[:idx]
		suffix := line[idx:]

		trimmed := strings.TrimSpace(prefix)
		if strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "'") {
			return fmt.Sprintf("[%s]%s[-]%s", theme.TagAccent(), prefix, highlightDetailValue(suffix))
		} else if !strings.Contains(trimmed, " ") && len(trimmed) > 0 {
			return fmt.Sprintf("[%s::b]%s[-:-:-]%s", theme.TagAccent(), prefix, highlightDetailValue(suffix))
		}
	}

	return highlightDetailValue(line)
}

// highlightDetailValue highlights JSON values.
func highlightDetailValue(s string) string {
	result := s
	result = strings.ReplaceAll(result, "true", fmt.Sprintf("[%s]true[-]", temporal.StatusCompleted.ColorTag()))
	result = strings.ReplaceAll(result, "false", fmt.Sprintf("[%s]false[-]", temporal.StatusFailed.ColorTag()))
	result = strings.ReplaceAll(result, "null", fmt.Sprintf("[%s]null[-]", theme.TagFgDim()))
	return result
}

func (wd *WorkflowDetail) showIOModal() {
	if wd.workflow == nil {
		return
	}
	showWorkflowIO(wd.app, wd, wd.workflow.Type, wd.workflow.Input, wd.workflow.Output, func() {
		wd.setFocusPane(detailFocusEvents)
	})
}

// jumpToChildWorkflow navigates to the child workflow if the selected event is a child workflow event.
func (wd *WorkflowDetail) jumpToChildWorkflow() {
	row := wd.eventTable.SelectedRow()
	if row < 0 || row >= len(wd.events) {
		return
	}

	ev := wd.events[row]

	// Check if this event has child workflow info
	if ev.ChildWorkflowID == "" || ev.ChildRunID == "" {
		return
	}

	// Navigate to the child workflow
	wd.app.NavigateToWorkflowDetail(ev.ChildWorkflowID, ev.ChildRunID)
}

// hasChildWorkflowInfo returns true if the selected event is a child workflow event with navigation info.
func (wd *WorkflowDetail) hasChildWorkflowInfo() bool {
	row := wd.eventTable.SelectedRow()
	if row < 0 || row >= len(wd.events) {
		return false
	}

	ev := wd.events[row]
	return ev.ChildWorkflowID != "" && ev.ChildRunID != ""
}
