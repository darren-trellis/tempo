package view

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/galaxy-io/tempo/internal/temporal"
)

const customColumnErrorText = "ERR"

var singleLine = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")

type customColumnEnv struct {
	WorkflowId        string
	RunId             string
	WorkflowType      string
	ExecutionStatus   string
	TaskQueue         string
	Namespace         string
	ParentWorkflowId  string
	StartTime         time.Time
	CloseTime         time.Time
	ExecutionDuration time.Duration
	SearchAttributes  map[string]any
	Memo              map[string]string
}

func newCustomColumnEnv(now time.Time, w temporal.Workflow) customColumnEnv {
	env := customColumnEnv{
		WorkflowId:       w.ID,
		RunId:            w.RunID,
		WorkflowType:     w.Type,
		ExecutionStatus:  w.Status,
		TaskQueue:        w.TaskQueue,
		Namespace:        w.Namespace,
		StartTime:        w.StartTime,
		SearchAttributes: make(map[string]any, len(w.SearchAttributes)),
		Memo:             w.Memo,
	}
	if w.ParentID != nil {
		env.ParentWorkflowId = *w.ParentID
	}
	switch {
	case w.EndTime != nil:
		env.CloseTime = *w.EndTime
		env.ExecutionDuration = w.EndTime.Sub(w.StartTime)
	case w.Status == "Running" && !w.StartTime.IsZero():
		env.ExecutionDuration = now.Sub(w.StartTime)
	}
	for name, value := range w.SearchAttributes {
		env.SearchAttributes[name] = value
	}
	for name, value := range w.SearchAttributeValues {
		env.SearchAttributes[name] = value
	}
	return env
}

type customColumnProgram struct {
	program  *vm.Program
	err      error
	reported atomic.Bool
}

var customColumnPrograms sync.Map

func compileCustomColumn(source string) *customColumnProgram {
	if cached, ok := customColumnPrograms.Load(source); ok {
		return cached.(*customColumnProgram)
	}
	program, err := expr.Compile(source, expr.Env(customColumnEnv{}))
	compiled, _ := customColumnPrograms.LoadOrStore(source, &customColumnProgram{program: program, err: err})
	return compiled.(*customColumnProgram)
}

// value leaves the cell empty when evaluation fails at runtime, since that is
// usually a workflow that lacks a search attribute the expression reads.
func (p *customColumnProgram) value(now time.Time, w temporal.Workflow, timeFmt string) string {
	if p.err != nil {
		return customColumnErrorText
	}
	out, err := expr.Run(p.program, newCustomColumnEnv(now, w))
	if err != nil {
		return ""
	}
	return formatCustomColumnValue(out, now, timeFmt)
}

func formatCustomColumnValue(value any, now time.Time, timeFmt string) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return singleLine.Replace(v)
	case time.Time:
		return formatDisplayTime(now, v, timeFmt)
	case time.Duration:
		if v >= time.Second || v <= -time.Second {
			v = v.Round(time.Second)
		}
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = formatCustomColumnValue(item, now, timeFmt)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}
