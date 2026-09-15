package view

import (
	"fmt"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const eventTreeDurationGap = 1

// EventTreeView displays workflow history events in a collapsible tree structure.
type EventTreeView struct {
	*tview.TreeView
	app          *App
	root         *tview.TreeNode
	nodes        []*temporal.EventTreeNode
	onSelect     func(node *temporal.EventTreeNode)
	onSelChange  func(node *temporal.EventTreeNode)
	selectedNode *temporal.EventTreeNode
}

// NewEventTreeView creates a new tree view for displaying workflow events.
func NewEventTreeView() *EventTreeView {
	root := tview.NewTreeNode("Events")
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	tree.SetBackgroundColor(tcell.ColorDefault)
	tree.SetGraphics(true)

	etv := &EventTreeView{
		TreeView: tree,
		root:     root,
	}

	// Handle selection changes
	tree.SetChangedFunc(func(node *tview.TreeNode) {
		if node != nil {
			ref := node.GetReference()
			if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
				etv.selectedNode = eventNode
				if etv.onSelChange != nil {
					etv.onSelChange(eventNode)
				}
			}
		}
	})

	// Handle enter key (toggle expand/collapse or select)
	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		if node == nil {
			return
		}

		ref := node.GetReference()
		if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
			// Toggle expand/collapse if has children
			if eventNode.HasChildren() {
				eventNode.Collapsed = !eventNode.Collapsed
				node.SetExpanded(!eventNode.Collapsed)
			}

			// Call select handler
			if etv.onSelect != nil {
				etv.onSelect(eventNode)
			}
		}
	})

	return etv
}

// Destroy is a no-op kept for backward compatibility.
func (etv *EventTreeView) Destroy() {}

// Draw applies theme colors dynamically before drawing.
func (etv *EventTreeView) Draw(screen tcell.Screen) {
	etv.SetBackgroundColor(theme.Bg())
	etv.SetGraphicsColor(theme.FgDim())
	etv.root.SetColor(theme.Accent())
	etv.refreshColors()
	colW := eventTreeDurationWidth(etv.nodes)
	if colW > 0 {
		colW += eventTreeDurationGap
	}
	etv.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		innerW := width
		if appShowsScrollbars(etv.app) && treeScrollMetrics(etv.TreeView, height).overflow() {
			innerW--
		}
		if colW > 0 && innerW > colW+2 {
			innerW -= colW
		}
		if innerW < 1 {
			innerW = 1
		}
		return x, y, innerW, height
	})
	etv.TreeView.Draw(screen)
	etv.drawDurationColumn(screen, colW)
	drawTreeScrollbar(etv.TreeView, screen, etv.app)
}

// SetNodes populates the tree with event nodes.
func (etv *EventTreeView) SetNodes(nodes []*temporal.EventTreeNode) {
	etv.nodes = nodes
	etv.root.ClearChildren()

	for _, node := range nodes {
		treeNode := etv.createTreeNode(node, 0)
		etv.root.AddChild(treeNode)
	}

	// Expand root by default
	etv.root.SetExpanded(true)

	// Select first node if available
	if len(nodes) > 0 {
		if children := etv.root.GetChildren(); len(children) > 0 {
			etv.SetCurrentNode(children[0])
		}
	}
}

// createTreeNode recursively creates tview tree nodes from EventTreeNodes.
func (etv *EventTreeView) createTreeNode(node *temporal.EventTreeNode, depth int) *tview.TreeNode {
	// Build display text
	text := etv.formatNodeText(node)

	treeNode := tview.NewTreeNode(text).
		SetReference(node).
		SetSelectable(true).
		SetExpanded(!node.Collapsed)

	// Set color based on status
	treeNode.SetColor(etv.statusColor(node.Status))

	// Add children (attempts for activities with retries)
	for _, child := range node.Children {
		childTreeNode := etv.createTreeNode(child, depth+1)
		treeNode.AddChild(childTreeNode)
	}

	return treeNode
}

// formatNodeText creates the display text for a tree node.
func (etv *EventTreeView) formatNodeText(node *temporal.EventTreeNode) string {
	icon := etv.statusIcon(node.Status)
	name := node.Name

	var suffix string
	if node.Attempts > 1 {
		suffix = fmt.Sprintf(" %d attempts", node.Attempts)
	}

	statusTag := fmt.Sprintf("[%s]", node.Status)
	return fmt.Sprintf("%s %s %s%s", icon, name, statusTag, suffix)
}

func eventTreeDurationText(node *temporal.EventTreeNode) string {
	if node == nil || node.Duration <= 0 {
		return ""
	}
	return temporal.FormatDuration(node.Duration)
}

func eventTreeDurationWidth(nodes []*temporal.EventTreeNode) int {
	width := 0
	var walk func([]*temporal.EventTreeNode)
	walk = func(list []*temporal.EventTreeNode) {
		for _, node := range list {
			if n := len(eventTreeDurationText(node)); n > width {
				width = n
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return width
}

func flattenVisibleTreeNodes(root *tview.TreeNode) []*tview.TreeNode {
	if root == nil {
		return nil
	}
	var out []*tview.TreeNode
	var walk func(*tview.TreeNode)
	walk = func(node *tview.TreeNode) {
		out = append(out, node)
		if node.IsExpanded() {
			for _, child := range node.GetChildren() {
				walk(child)
			}
		}
	}
	walk(root)
	return out
}

func rightAlignIn(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) > width {
		return string(runes[len(runes)-width:])
	}
	pad := width - len(runes)
	out := make([]rune, width)
	for i := 0; i < pad; i++ {
		out[i] = ' '
	}
	copy(out[pad:], runes)
	return string(out)
}

func (etv *EventTreeView) drawDurationColumn(screen tcell.Screen, colW int) {
	if etv == nil || screen == nil || colW <= 0 {
		return
	}
	innerX, y, innerW, height := etv.GetInnerRect()
	if height < 1 {
		return
	}
	colX := innerX + innerW
	_, _, width, _ := etv.GetRect()
	if colX+colW > innerX+width {
		colX = innerX + innerW - colW
		if colX < innerX {
			return
		}
	}
	visible := flattenVisibleTreeNodes(etv.root)
	offset := etv.GetScrollOffset()
	style := tcell.StyleDefault.Background(theme.Bg()).Foreground(theme.FgDim())
	for row := 0; row < height; row++ {
		idx := offset + row
		if idx < 0 || idx >= len(visible) {
			break
		}
		text := ""
		if eventNode, ok := visible[idx].GetReference().(*temporal.EventTreeNode); ok {
			text = eventTreeDurationText(eventNode)
		}
		aligned := rightAlignIn(text, colW)
		for i, r := range aligned {
			screen.SetContent(colX+i, y+row, r, nil, style)
		}
	}
}

// statusIcon returns the icon for a node status.
func (etv *EventTreeView) statusIcon(status string) string {
	switch status {
	case "Running":
		return theme.IconRunning
	case "Completed":
		return theme.IconCompleted
	case "Failed":
		return theme.IconFailed
	case "Canceled":
		return theme.IconCanceled
	case "Terminated":
		return theme.IconTerminated
	case "TimedOut":
		return theme.IconTimedOut
	case "Fired":
		return theme.IconCompleted
	case "Scheduled", "Initiated", "Pending":
		return theme.IconPending
	default:
		return theme.IconEvent
	}
}

// statusColor returns the color for a node status.
func (etv *EventTreeView) statusColor(status string) tcell.Color {
	return temporal.GetWorkflowStatus(status).Color()
}

// refreshColors updates all node colors after theme change.
func (etv *EventTreeView) refreshColors() {
	etv.walkNodes(etv.root, func(node *tview.TreeNode) {
		ref := node.GetReference()
		if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
			node.SetColor(etv.statusColor(eventNode.Status))
		}
	})
}

// walkNodes traverses all nodes in the tree.
func (etv *EventTreeView) walkNodes(node *tview.TreeNode, fn func(*tview.TreeNode)) {
	fn(node)
	for _, child := range node.GetChildren() {
		etv.walkNodes(child, fn)
	}
}

// SetOnSelect sets the callback for when a node is activated (Enter pressed).
func (etv *EventTreeView) SetOnSelect(fn func(node *temporal.EventTreeNode)) {
	etv.onSelect = fn
}

// SetOnSelectionChanged sets the callback for when selection changes.
func (etv *EventTreeView) SetOnSelectionChanged(fn func(node *temporal.EventTreeNode)) {
	etv.onSelChange = fn
}

// SelectedNode returns the currently selected event node.
func (etv *EventTreeView) SelectedNode() *temporal.EventTreeNode {
	return etv.selectedNode
}

// ToggleSelected collapses or expands the selected node.
func (etv *EventTreeView) ToggleSelected() bool {
	node := etv.GetCurrentNode()
	if node == nil {
		return false
	}
	eventNode, ok := node.GetReference().(*temporal.EventTreeNode)
	if !ok || !eventNode.HasChildren() {
		return false
	}
	eventNode.Collapsed = !eventNode.Collapsed
	node.SetExpanded(!eventNode.Collapsed)
	return true
}

// ExpandAll expands all nodes in the tree.
func (etv *EventTreeView) ExpandAll() {
	etv.walkNodes(etv.root, func(node *tview.TreeNode) {
		node.SetExpanded(true)
		ref := node.GetReference()
		if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
			eventNode.Collapsed = false
		}
	})
}

// CollapseAll collapses all nodes in the tree (except root).
func (etv *EventTreeView) CollapseAll() {
	for _, child := range etv.root.GetChildren() {
		etv.walkNodes(child, func(node *tview.TreeNode) {
			node.SetExpanded(false)
			ref := node.GetReference()
			if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
				eventNode.Collapsed = true
			}
		})
	}
}

// JumpToFailed finds and selects the first failed node.
func (etv *EventTreeView) JumpToFailed() bool {
	var failedNode *tview.TreeNode
	etv.walkNodes(etv.root, func(node *tview.TreeNode) {
		if failedNode != nil {
			return
		}
		ref := node.GetReference()
		if eventNode, ok := ref.(*temporal.EventTreeNode); ok {
			if eventNode.Status == "Failed" || eventNode.Status == "TimedOut" {
				failedNode = node
			}
		}
	})

	if failedNode != nil {
		// Expand parent nodes to make it visible
		etv.expandParentsOf(failedNode)
		etv.SetCurrentNode(failedNode)
		return true
	}
	return false
}

// expandParentsOf expands all parent nodes of the given node.
func (etv *EventTreeView) expandParentsOf(target *tview.TreeNode) {
	// Walk from root and expand nodes on the path to target
	etv.expandPath(etv.root, target)
}

// expandPath recursively expands nodes on the path to target.
func (etv *EventTreeView) expandPath(current, target *tview.TreeNode) bool {
	if current == target {
		return true
	}

	for _, child := range current.GetChildren() {
		if etv.expandPath(child, target) {
			current.SetExpanded(true)
			if ref, ok := current.GetReference().(*temporal.EventTreeNode); ok {
				ref.Collapsed = false
			}
			return true
		}
	}

	return false
}

// NodeCount returns the total number of nodes.
func (etv *EventTreeView) NodeCount() int {
	count := 0
	etv.walkNodes(etv.root, func(_ *tview.TreeNode) {
		count++
	})
	return count - 1 // Exclude root
}

// JumpToFirst selects the first event node in the tree.
func (etv *EventTreeView) JumpToFirst() {
	children := etv.root.GetChildren()
	if len(children) > 0 {
		etv.SetCurrentNode(children[0])
	}
}

// JumpToLast selects the last visible event node in the tree.
func (etv *EventTreeView) JumpToLast() {
	node := etv.findLastVisible(etv.root)
	if node != nil && node != etv.root {
		etv.SetCurrentNode(node)
	}
}

// findLastVisible returns the last visible (expanded) node in the subtree.
func (etv *EventTreeView) findLastVisible(node *tview.TreeNode) *tview.TreeNode {
	children := node.GetChildren()
	if len(children) == 0 || !node.IsExpanded() {
		return node
	}
	return etv.findLastVisible(children[len(children)-1])
}
