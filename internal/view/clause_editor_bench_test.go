package view

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func BenchmarkClauseEditorBackspace(b *testing.B) {
	b.Setenv("XDG_CONFIG_HOME", b.TempDir())
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")
	wl := NewWorkflowList(a, "default")
	wl.keepDataOnStart = true
	now := time.Now()
	for i := 0; i < 400; i++ {
		wl.allWorkflows = append(wl.allWorkflows, temporal.Workflow{
			ID: fmt.Sprintf("order-%04d", i), RunID: fmt.Sprintf("run-%04d", i), Type: "OrderWorkflow",
			Status: "Running", Namespace: "default", TaskQueue: "orders", StartTime: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	wl.applyFilter()
	a.app.Pages().Push(wl)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	screen.SetSize(220, 60)
	tv := a.app.GetApplication()
	tv.SetScreen(screen)
	draws := make(chan struct{}, 1<<16)
	prev := tv.GetAfterDrawFunc()
	tv.SetAfterDrawFunc(func(s tcell.Screen) {
		if prev != nil {
			prev(s)
		}
		draws <- struct{}{}
	})
	done := make(chan error, 1)
	go func() { done <- a.app.Run() }()
	defer func() {
		tv.Stop()
		<-done
	}()
	<-draws

	value := strings.Repeat("x", 64)
	tv.QueueUpdateDraw(func() {
		wl.showClauseEditor(config.FilterClause{Key: "WorkflowId", Op: filterOpEq, Value: value}, nil)
	})
	<-draws
	drain := func() {
		for {
			select {
			case <-draws:
			case <-time.After(50 * time.Millisecond):
				return
			}
		}
	}
	drain()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		screen.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
		<-draws
	}
}
