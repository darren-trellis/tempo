package view

import (
	"sort"
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

const workflowTreeIndent = 3

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

func workflowIdentityKey(w temporal.Workflow) string {
	return w.ID + "\x00" + w.RunID
}

func workflowTreeFoldAllKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	if event.Key() == tcell.KeyCtrlSpace {
		return true
	}
	if event.Key() != tcell.KeyRune || event.Rune() != ' ' {
		return false
	}
	mods := event.Modifiers()
	return mods&tcell.ModCtrl != 0 || mods&tcell.ModMeta != 0
}

func workflowTreeFoldAllHint(folded bool) string {
	if folded {
		return "Unfold All"
	}
	return "Fold All"
}

func (wl *WorkflowList) applyWorkflowOrder(workflows []temporal.Workflow) {
	if wl.workflowTreeMode {
		collapsed := wl.workflowCollapsed
		if wl.filterText != "" {
			collapsed = nil
		}
		wl.workflows, wl.workflowDepths, wl.workflowTreePrefixes, wl.workflowHasChildren = nestWorkflows(workflows, collapsed)
		return
	}
	wl.workflows = workflows
	wl.workflowDepths = nil
	wl.workflowTreePrefixes = nil
	wl.workflowHasChildren = nil
}

func (wl *WorkflowList) workflowDepth(index int) int {
	if index < 0 || index >= len(wl.workflowDepths) {
		return 0
	}
	return wl.workflowDepths[index]
}

func (wl *WorkflowList) workflowTreePrefixAt(index int) string {
	if index < 0 || index >= len(wl.workflowTreePrefixes) {
		return ""
	}
	return wl.workflowTreePrefixes[index]
}

func (wl *WorkflowList) workflowHasChildAt(index int) bool {
	return index >= 0 && index < len(wl.workflowHasChildren) && wl.workflowHasChildren[index]
}

func (wl *WorkflowList) workflowRowPrefix(index int) string {
	if wl == nil {
		return ""
	}
	prefix := wl.workflowTreePrefixAt(index)
	if !wl.workflowTreeMode || !wl.workflowHasChildAt(index) || index >= len(wl.workflows) {
		return prefix
	}
	mark := theme.IconTreeExpanded
	if wl.workflowCollapsed[workflowIdentityKey(wl.workflows[index])] {
		mark = theme.IconTreeCollapsed
	}
	return prefix + mark + " "
}

func (wl *WorkflowList) anyWorkflowFolded() bool {
	if wl == nil || len(wl.workflowCollapsed) == 0 {
		return false
	}
	for i, w := range wl.workflows {
		if wl.workflowHasChildAt(i) && wl.workflowCollapsed[workflowIdentityKey(w)] {
			return true
		}
	}
	return false
}

func (wl *WorkflowList) refreshWorkflowTreeHints() {
	if wl.app != nil && wl.app.JigApp() != nil && wl.app.JigApp().Menu() != nil {
		wl.app.JigApp().Menu().SetHints(wl.Hints())
	}
}

func (wl *WorkflowList) toggleWorkflowTreeFold() bool {
	if wl == nil || !wl.workflowTreeMode || wl.selectionMode {
		return false
	}
	row := wl.table.SelectedRow()
	if !wl.workflowHasChildAt(row) {
		return false
	}
	key := workflowIdentityKey(wl.workflows[row])
	if wl.workflowCollapsed == nil {
		wl.workflowCollapsed = make(map[string]bool)
	}
	if wl.workflowCollapsed[key] {
		delete(wl.workflowCollapsed, key)
	} else {
		wl.workflowCollapsed[key] = true
	}
	wl.applyFilter()
	wl.refreshWorkflowTreeHints()
	return true
}

func (wl *WorkflowList) toggleWorkflowTreeFoldAll() bool {
	if wl == nil || !wl.workflowTreeMode || wl.selectionMode {
		return false
	}
	if wl.anyWorkflowFolded() {
		wl.workflowCollapsed = nil
	} else {
		wl.foldAllWorkflowParents()
	}
	wl.applyFilter()
	wl.refreshWorkflowTreeHints()
	return true
}

func (wl *WorkflowList) foldAllWorkflowParents() {
	full, _, _, hasKids := nestWorkflows(wl.allWorkflows, nil)
	collapsed := make(map[string]bool)
	for i, w := range full {
		if i < len(hasKids) && hasKids[i] {
			collapsed[workflowIdentityKey(w)] = true
		}
	}
	if len(collapsed) == 0 {
		wl.workflowCollapsed = nil
		return
	}
	wl.workflowCollapsed = collapsed
}

func colorizeWorkflowTreePrefix(text, prefix string) string {
	if prefix == "" {
		return text
	}
	dim := "[" + theme.TagFgDim() + "]"
	if strings.HasPrefix(text, prefix) && len(text) >= len(prefix) {
		return dim + prefix + "[-]" + text[len(prefix):]
	}
	return dim + text + "[-]"
}

func workflowTreePrefix(isLastAtLevel []bool) string {
	if len(isLastAtLevel) == 0 {
		return ""
	}
	var b strings.Builder
	for i, last := range isLastAtLevel {
		if i == len(isLastAtLevel)-1 {
			if last {
				b.WriteString(" " + theme.IconTreeLast + theme.IconTreeHoriz)
			} else {
				b.WriteString(" " + theme.IconTreeBranch + theme.IconTreeHoriz)
			}
			continue
		}
		if last {
			b.WriteString(strings.Repeat(" ", workflowTreeIndent))
		} else {
			b.WriteString(" " + theme.IconTreeVert + " ")
		}
	}
	return b.String()
}

func workflowChildLinks(workflows []temporal.Workflow) (children [][]int, isChild []bool) {
	children = make([][]int, len(workflows))
	isChild = make([]bool, len(workflows))
	if len(workflows) == 0 {
		return children, isChild
	}

	byID := make(map[string][]int, len(workflows))
	for i, w := range workflows {
		byID[w.ID] = append(byID[w.ID], i)
	}

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
	return children, isChild
}

func nestWorkflows(workflows []temporal.Workflow, collapsed map[string]bool) ([]temporal.Workflow, []int, []string, []bool) {
	if len(workflows) == 0 {
		return workflows, nil, nil, nil
	}

	children, isChild := workflowChildLinks(workflows)

	for p := range children {
		sort.SliceStable(children[p], func(a, b int) bool {
			return workflows[children[p][a]].StartTime.Before(workflows[children[p][b]].StartTime)
		})
	}

	out := make([]temporal.Workflow, 0, len(workflows))
	depths := make([]int, 0, len(workflows))
	prefixes := make([]string, 0, len(workflows))
	hasChildren := make([]bool, 0, len(workflows))
	visited := make([]bool, len(workflows))

	var hide func(int)
	hide = func(i int) {
		if i < 0 || i >= len(visited) || visited[i] {
			return
		}
		visited[i] = true
		for _, c := range children[i] {
			hide(c)
		}
	}

	var walk func(int, int, []bool)
	walk = func(i, depth int, path []bool) {
		if i < 0 || i >= len(workflows) || visited[i] {
			return
		}
		visited[i] = true
		out = append(out, workflows[i])
		depths = append(depths, depth)
		prefixes = append(prefixes, workflowTreePrefix(path))
		kids := children[i]
		hasChildren = append(hasChildren, len(kids) > 0)
		if len(kids) > 0 && collapsed[workflowIdentityKey(workflows[i])] {
			for _, c := range kids {
				hide(c)
			}
			return
		}
		for j, c := range kids {
			childPath := make([]bool, len(path)+1)
			copy(childPath, path)
			childPath[len(path)] = j == len(kids)-1
			walk(c, depth+1, childPath)
		}
	}

	for i := range workflows {
		if !isChild[i] {
			walk(i, 0, nil)
		}
	}
	for i := range workflows {
		if !visited[i] {
			walk(i, 0, nil)
		}
	}
	return out, depths, prefixes, hasChildren
}
