package temporal

import (
	"encoding/json"
	"strings"

	"github.com/atterpac/jig/theme"
	commonpb "go.temporal.io/api/common/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/sdk/converter"
)

const (
	temporalReportedProblemsAttr = "TemporalReportedProblems"
	reportedProblemTaskFailed    = "category=WorkflowTaskFailed"
	reportedProblemTaskTimedOut  = "category=WorkflowTaskTimedOut"
)

func workflowFromExecutionInfo(info *workflowpb.WorkflowExecutionInfo, namespace string) Workflow {
	wf := Workflow{
		Namespace:   namespace,
		TaskFailure: executionHasTaskFailure(info),
	}
	if info == nil {
		return wf
	}
	if exec := info.GetExecution(); exec != nil {
		wf.ID = exec.GetWorkflowId()
		wf.RunID = exec.GetRunId()
	}
	if typ := info.GetType(); typ != nil {
		wf.Type = typ.GetName()
	}
	wf.Status = MapWorkflowStatus(info.GetStatus())
	wf.TaskQueue = info.GetTaskQueue()
	if start := info.GetStartTime(); start != nil {
		wf.StartTime = start.AsTime()
	}
	if close := info.GetCloseTime(); close != nil && !close.AsTime().IsZero() {
		t := close.AsTime()
		wf.EndTime = &t
	}
	if parent := info.GetParentExecution(); parent != nil && parent.GetWorkflowId() != "" {
		parentID := parent.GetWorkflowId()
		wf.ParentID = &parentID
	}
	if memo := info.GetMemo(); memo != nil {
		wf.Memo = memoFields(memo)
	}
	if wf.Status != "Running" {
		wf.TaskFailure = false
	}
	return wf
}

func memoFields(memo *commonpb.Memo) map[string]string {
	fields := memo.GetFields()
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if v == nil || v.GetData() == nil {
			continue
		}
		var strVal string
		if err := json.Unmarshal(v.GetData(), &strVal); err == nil {
			out[k] = strVal
			continue
		}
		out[k] = string(v.GetData())
	}
	return out
}

func executionHasTaskFailure(info *workflowpb.WorkflowExecutionInfo) bool {
	if info == nil || MapWorkflowStatus(info.GetStatus()) != "Running" {
		return false
	}
	payload, ok := info.GetSearchAttributes().GetIndexedFields()[temporalReportedProblemsAttr]
	if !ok {
		return false
	}
	for _, problem := range decodeReportedProblems(payload) {
		if isReportedTaskFailure(problem) {
			return true
		}
	}
	return false
}

func decodeReportedProblems(payload *commonpb.Payload) []string {
	if payload == nil {
		return nil
	}
	dc := converter.GetDefaultDataConverter()
	var list []string
	if err := dc.FromPayload(payload, &list); err == nil && len(list) > 0 {
		return list
	}
	var text string
	if err := dc.FromPayload(payload, &text); err == nil && text != "" {
		return []string{text}
	}
	if err := json.Unmarshal(payload.GetData(), &list); err == nil && len(list) > 0 {
		return list
	}
	raw := strings.TrimSpace(string(payload.GetData()))
	if raw == "" {
		return nil
	}
	return []string{strings.Trim(raw, `"`)}
}

func isReportedTaskFailure(problem string) bool {
	p := strings.TrimSpace(problem)
	switch p {
	case reportedProblemTaskFailed, reportedProblemTaskTimedOut:
		return true
	}
	return strings.Contains(p, "WorkflowTaskFailed") ||
		strings.Contains(p, "WorkflowTaskTimedOut") ||
		strings.Contains(p, "UnhandledFailure")
}

func (w Workflow) HasTaskFailure() bool {
	return w.TaskFailure && (w.Status == "Running" || w.Status == "")
}

func WorkflowDisplayStatus(w Workflow) (string, *theme.Status) {
	if w.HasTaskFailure() {
		return "Unhandled Failure", StatusUnhandledFailure
	}
	return w.Status, GetWorkflowStatus(w.Status)
}

func HistoryHasTaskFailure(events []EnhancedHistoryEvent) bool {
	last := ""
	for _, ev := range events {
		switch ev.Type {
		case "WorkflowTaskCompleted":
			last = "ok"
		case "WorkflowTaskFailed", "WorkflowTaskTimedOut":
			last = "fail"
		}
	}
	return last == "fail"
}
