package view

import (
	"sort"
	"strings"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func treeModeHint(tree bool) string {
	if tree {
		return "List"
	}
	return "Tree"
}

func (wl *WorkflowList) toggleWorkflowTree() {
	wl.workflowTreeMode = !wl.workflowTreeMode
	id, runID, _ := wl.CommandContext()
	wl.applyFilter()
	wl.updatePanelTitle()
	if id != "" {
		for i, w := range wl.workflows {
			if w.ID == id && w.RunID == runID {
				wl.table.SelectRow(i)
				break
			}
		}
	}
	if wl.app != nil && wl.app.JigApp() != nil {
		wl.app.JigApp().Menu().SetHints(wl.Hints())
	}
}

func (wl *WorkflowList) applyWorkflowOrder(workflows []temporal.Workflow) {
	if wl.workflowTreeMode {
		wl.workflows, wl.workflowDepths = nestWorkflows(workflows)
		return
	}
	wl.workflows = workflows
	wl.workflowDepths = nil
}

func (wl *WorkflowList) workflowDepth(index int) int {
	if index < 0 || index >= len(wl.workflowDepths) {
		return 0
	}
	return wl.workflowDepths[index]
}

func workflowTreePrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("  ", depth) + "└ "
}

func nestWorkflows(workflows []temporal.Workflow) ([]temporal.Workflow, []int) {
	if len(workflows) == 0 {
		return workflows, nil
	}

	byID := make(map[string][]int, len(workflows))
	for i, w := range workflows {
		byID[w.ID] = append(byID[w.ID], i)
	}

	children := make([][]int, len(workflows))
	isChild := make([]bool, len(workflows))
	for i, w := range workflows {
		if w.ParentID == nil || *w.ParentID == "" || *w.ParentID == w.ID {
			continue
		}
		parents, ok := byID[*w.ParentID]
		if !ok {
			continue
		}
		parent := parents[0]
		for _, p := range parents {
			if p != i {
				parent = p
				break
			}
		}
		if parent == i {
			continue
		}
		children[parent] = append(children[parent], i)
		isChild[i] = true
	}

	for p := range children {
		sort.SliceStable(children[p], func(a, b int) bool {
			return workflows[children[p][a]].StartTime.Before(workflows[children[p][b]].StartTime)
		})
	}

	out := make([]temporal.Workflow, 0, len(workflows))
	depths := make([]int, 0, len(workflows))
	visited := make([]bool, len(workflows))

	var walk func(int, int)
	walk = func(i, depth int) {
		if i < 0 || i >= len(workflows) || visited[i] {
			return
		}
		visited[i] = true
		out = append(out, workflows[i])
		depths = append(depths, depth)
		for _, c := range children[i] {
			walk(c, depth+1)
		}
	}

	for i := range workflows {
		if !isChild[i] {
			walk(i, 0)
		}
	}
	for i := range workflows {
		if !visited[i] {
			walk(i, 0)
		}
	}
	return out, depths
}
