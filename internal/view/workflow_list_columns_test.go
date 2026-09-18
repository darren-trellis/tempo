package view

import (
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestWorkflowColumnParentID(t *testing.T) {
	parent := "order-processing-abc123"
	got, _ := workflowColumnValue(config.WorkflowColumnParentID, time.Now(), temporal.Workflow{
		ID:       "payment-xyz789",
		ParentID: &parent,
	}, config.TimeFormatRelative)
	if got != parent {
		t.Fatalf("got %q", got)
	}

	empty, _ := workflowColumnValue(config.WorkflowColumnParentID, time.Now(), temporal.Workflow{ID: "solo"}, config.TimeFormatRelative)
	if empty != "" {
		t.Fatalf("expected empty parent, got %q", empty)
	}

	header, ok := workflowColumnHeader(config.WorkflowColumnParentID)
	if !ok || header != "PARENT ID" {
		t.Fatalf("header=%q ok=%v", header, ok)
	}
}

func TestDefaultColumnLayoutIncludesParent(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflowTreeMode = false
	found := false
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default workflow columns should include parent id")
	}
}

func TestWorkflowColumnMarksUnhandledFailure(t *testing.T) {
	text, status := workflowColumnValue(config.WorkflowColumnStatus, time.Now(), temporal.Workflow{
		Status:      "Running",
		TaskFailure: true,
	}, config.TimeFormatRelative)
	if status != temporal.StatusUnhandledFailure {
		t.Fatalf("status=%v", status)
	}
	if !strings.Contains(text, temporal.StatusUnhandledFailure.Icon()) || !strings.Contains(text, "Unhandled") {
		t.Fatalf("text=%q", text)
	}

	running, runningStatus := workflowColumnValue(config.WorkflowColumnStatus, time.Now(), temporal.Workflow{
		Status: "Running",
	}, config.TimeFormatRelative)
	if runningStatus != temporal.StatusRunning {
		t.Fatalf("running status=%v", runningStatus)
	}
	if running == text {
		t.Fatalf("running and unhandled should differ: %q", running)
	}
}

func TestStatusColumnWidthTwoShowsFullIcon(t *testing.T) {
	w := temporal.Workflow{Status: "Running"}
	icon := temporal.StatusRunning.Icon()
	got, _ := workflowStatusText(w, 2)
	if got != icon {
		t.Fatalf("width 2 should be the full icon, got %q", got)
	}
	if strings.Contains(got, ".") {
		t.Fatal("width 2 must not use ellipsis truncation")
	}
	wide, _ := workflowStatusText(w, 12)
	if !strings.Contains(wide, icon) || !strings.Contains(wide, "Running") {
		t.Fatalf("wide status=%q", wide)
	}
	if got := config.ClampWorkflowColumnWidth(config.WorkflowColumnStatus, 2); got != 2 {
		t.Fatalf("status width 2 should be kept, got %d", got)
	}
}

func TestStatusColumnWidthOneShowsCompactIcon(t *testing.T) {
	w := temporal.Workflow{Status: "Running"}
	got, _ := workflowStatusText(w, 1)
	want := compactStatusIcon(temporal.StatusRunning)
	if got != want {
		t.Fatalf("width 1 should be the compact icon %q, got %q", want, got)
	}
	if got == temporal.StatusRunning.Icon() {
		t.Fatal("compact running icon should differ from the full icon")
	}
	if got := config.ClampWorkflowColumnWidth(config.WorkflowColumnStatus, 1); got != 1 {
		t.Fatalf("status width 1 should be kept, got %d", got)
	}
}

func TestWorkflowRowColorFollowsConfig(t *testing.T) {
	w := temporal.Workflow{ID: "wf-1", RunID: "r", Status: "Failed"}
	wl := NewWorkflowList(&App{}, "default")
	cells := wl.styledWorkflowCells(time.Now(), w, 0)
	idIdx, statusIdx := workflowColumnIndexes(wl)
	if idIdx < 0 || statusIdx < 0 {
		t.Fatal("missing workflow id or status column")
	}
	if cells[idIdx].Status != nil {
		t.Fatal("workflow rows should not color the whole row by default")
	}
	if cells[statusIdx].Status != temporal.StatusFailed {
		t.Fatalf("status column should stay color coded, status=%v", cells[statusIdx].Status)
	}

	on := true
	wl.app = &App{config: &config.Config{ColorCodeWorkflows: &on}}
	cells = wl.styledWorkflowCells(time.Now(), w, 0)
	if cells[idIdx].Status != temporal.StatusFailed {
		t.Fatalf("enabled row color coding: id status=%v", cells[idIdx].Status)
	}
	if cells[statusIdx].Status != temporal.StatusFailed {
		t.Fatalf("enabled row color coding: status=%v", cells[statusIdx].Status)
	}
}

func workflowColumnIndexes(wl *WorkflowList) (idIdx, statusIdx int) {
	idIdx, statusIdx = -1, -1
	if wl == nil {
		return idIdx, statusIdx
	}
	for i, col := range wl.columnLayout() {
		switch col.id {
		case config.WorkflowColumnWorkflowID:
			idIdx = i
		case config.WorkflowColumnStatus:
			statusIdx = i
		}
	}
	return idIdx, statusIdx
}

func TestMergeWorkflowRefreshesUnhandledStatus(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflows = []temporal.Workflow{{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running"}}
	wl.allWorkflows = wl.workflows
	wl.populateTable()

	wl.mergeWorkflow(temporal.Workflow{ID: "wf-1", RunID: "run-1", Type: "T", Status: "Running", TaskFailure: true})
	cells := wl.table.GetRowCells(0)
	statusIdx := -1
	for i, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnStatus {
			statusIdx = i
			break
		}
	}
	if statusIdx < 0 || statusIdx >= len(cells) {
		t.Fatal("missing status column")
	}
	if cells[statusIdx].Status != temporal.StatusUnhandledFailure && !strings.Contains(cells[statusIdx].Text, "Unhan") {
		t.Fatalf("status cell=%q status=%v", cells[statusIdx].Text, cells[statusIdx].Status)
	}
}

func TestTreeModeKeepsParentIDColumn(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.workflowTreeMode = true
	found := false
	for _, col := range wl.columnLayout() {
		if col.id == config.WorkflowColumnParentID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("tree view should keep parent id so hoisted matches stay attributable")
	}
}

func TestFormatDisplayTime(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-5 * time.Minute)
	if got := formatDisplayTime(now, start, config.TimeFormatRelative); got != "5m ago" {
		t.Fatalf("relative=%q", got)
	}
	if got := formatDisplayTime(now, start, config.TimeFormatAbsolute); got != "11:55:00" {
		t.Fatalf("absolute=%q", got)
	}
	if got := formatDisplayTime(now, time.Time{}, config.TimeFormatRelative); got != "-" {
		t.Fatalf("zero=%q", got)
	}
}

func TestWorkflowTimesFollowConfig(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-5 * time.Minute)
	end := now.Add(-time.Minute)
	w := temporal.Workflow{ID: "wf-1", Status: "Completed", StartTime: start, EndTime: &end}

	got, _ := workflowColumnValue(config.WorkflowColumnStarted, now, w, config.TimeFormatRelative)
	if got != "5m ago" {
		t.Fatalf("relative started=%q", got)
	}
	got, _ = workflowColumnValue(config.WorkflowColumnEnded, now, w, config.TimeFormatRelative)
	if got != "1m ago" {
		t.Fatalf("relative ended=%q", got)
	}
	got, _ = workflowColumnValue(config.WorkflowColumnStarted, now, w, config.TimeFormatAbsolute)
	if got != "11:55:00" {
		t.Fatalf("absolute started=%q", got)
	}
	got, _ = workflowColumnValue(config.WorkflowColumnEnded, now, w, config.TimeFormatAbsolute)
	if got != "11:59:00" {
		t.Fatalf("absolute ended=%q", got)
	}
}

func TestDefaultActivityColumnLayoutIncludesEnded(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	found := false
	for _, col := range wl.activityColumnLayout() {
		if col.id == config.ActivityColumnEnded {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default activity columns should include ended")
	}
}

func TestActivityTimesFollowConfig(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	start := now.Add(-5 * time.Minute)
	end := now.Add(-time.Minute)
	a := previewActivity{Type: "Charge", Status: "Completed", StartTime: start, EndTime: &end}

	wl := NewWorkflowList(&App{}, "default")
	cells := wl.styledActivityCells(now, a)
	startedIdx, endedIdx := activityTimeColumnIndexes(wl)
	if startedIdx < 0 || endedIdx < 0 {
		t.Fatal("missing started or ended column")
	}
	if !strings.Contains(cells[startedIdx].Text, "5m ago") {
		t.Fatalf("activity started should default to relative, got %q", cells[startedIdx].Text)
	}
	if !strings.Contains(cells[endedIdx].Text, "1m ago") {
		t.Fatalf("activity ended should default to relative, got %q", cells[endedIdx].Text)
	}

	wl.app = &App{config: &config.Config{ActivityTimeFormat: config.TimeFormatAbsolute}}
	cells = wl.styledActivityCells(now, a)
	if !strings.Contains(cells[startedIdx].Text, "11:55:00") {
		t.Fatalf("absolute started=%q", cells[startedIdx].Text)
	}
	if !strings.Contains(cells[endedIdx].Text, "11:59:00") {
		t.Fatalf("absolute ended=%q", cells[endedIdx].Text)
	}
}

func TestActivityRowColorFollowsConfig(t *testing.T) {
	a := previewActivity{Type: "Charge", Status: "Failed", StartTime: time.Now()}
	wl := NewWorkflowList(&App{}, "default")
	cells := wl.styledActivityCells(time.Now(), a)
	nameIdx, statusIdx := activityNameStatusIndexes(wl)
	if nameIdx < 0 || statusIdx < 0 {
		t.Fatal("missing name or status column")
	}
	if cells[nameIdx].Status != nil {
		t.Fatal("activity rows should not color the whole row by default")
	}
	if cells[statusIdx].Status != temporal.StatusFailed {
		t.Fatalf("status column should stay color coded, status=%v", cells[statusIdx].Status)
	}

	on := true
	wl.app = &App{config: &config.Config{ColorCodeActivities: &on}}
	cells = wl.styledActivityCells(time.Now(), a)
	if cells[nameIdx].Status != temporal.StatusFailed {
		t.Fatalf("enabled row color coding: name status=%v", cells[nameIdx].Status)
	}
}

func activityTimeColumnIndexes(wl *WorkflowList) (startedIdx, endedIdx int) {
	startedIdx, endedIdx = -1, -1
	if wl == nil {
		return startedIdx, endedIdx
	}
	for i, col := range wl.activityColumnLayout() {
		switch col.id {
		case config.ActivityColumnStarted:
			startedIdx = i
		case config.ActivityColumnEnded:
			endedIdx = i
		}
	}
	return startedIdx, endedIdx
}

func activityNameStatusIndexes(wl *WorkflowList) (nameIdx, statusIdx int) {
	nameIdx, statusIdx = -1, -1
	if wl == nil {
		return nameIdx, statusIdx
	}
	for i, col := range wl.activityColumnLayout() {
		switch col.id {
		case config.ActivityColumnName:
			nameIdx = i
		case config.ActivityColumnStatus:
			statusIdx = i
		}
	}
	return nameIdx, statusIdx
}
