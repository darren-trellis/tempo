package view

import (
	"context"
	"fmt"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// WorkflowGraphView displays workflow relationships in a dual-pane layout.
// Top pane: GraphTree showing hierarchical tree of workflows
// Bottom pane: NodeGraph showing 2D visualization of relationships
type WorkflowGraphView struct {
	*components.Split
	app       *App
	namespace string
	workflow  *temporal.Workflow

	tree      *components.GraphTree
	graph     *components.NodeGraph
	treeData  *components.GraphTreeData
	graphData *components.NodeGraphData

	relationships *temporal.WorkflowRelationships
	depthLimit    int
	loading       bool
	loadGen       uint64
	embedded      bool
}

// NewWorkflowGraphView creates a new workflow graph view.
func NewWorkflowGraphView(app *App, namespace string, workflow *temporal.Workflow) *WorkflowGraphView {
	wg := &WorkflowGraphView{
		app:        app,
		namespace:  namespace,
		workflow:   workflow,
		depthLimit: 1, // Default depth (keep low for performance)
	}
	wg.setup()

	// Register for automatic theme refresh
	theme.RegisterRefreshable(wg)

	return wg
}

func (wg *WorkflowGraphView) setup() {
	// Create the tree component
	wg.tree = components.NewGraphTree()
	wg.tree.SetShowEdgeLabels(true)
	wg.tree.SetOnChange(wg.onTreeSelectionChange)
	wg.tree.SetOnSelect(wg.onTreeSelect)
	wg.tree.SetOnLoadChildren(wg.loadChildren)

	// Create the graph component
	wg.graph = components.NewNodeGraph()
	wg.graph.SetShowEdgeLabels(true)
	wg.graph.SetNodeWidth(18)
	wg.graph.SetOnSelect(wg.onGraphSelect)
	bindGraphHorizontalScroll(wg.graph)

	// Wrap in panels with titles
	treePanel := components.NewPanel().
		SetTitle(fmt.Sprintf("%s Tree", theme.IconNamespace)).
		SetContent(wg.tree)

	graphPanel := components.NewPanel().
		SetTitle(fmt.Sprintf("%s Graph", theme.IconGrid)).
		SetContent(wg.graph)

	wg.Split = components.NewSplit().
		SetDirection(components.SplitVertical).
		SetRatio(0.5).
		SetTop(treePanel).
		SetBottom(graphPanel).
		SetResizable(true).
		SetShowDivider(true)
}

func (wg *WorkflowGraphView) SetEmbedded(embedded bool) {
	if wg == nil {
		return
	}
	wg.embedded = embedded
}

func (wg *WorkflowGraphView) ShowWorkflow(namespace string, workflow temporal.Workflow) {
	if wg == nil || workflow.ID == "" {
		return
	}
	wg.namespace = namespace
	same := wg.workflow != nil && wg.workflow.ID == workflow.ID && wg.workflow.RunID == workflow.RunID
	if same && (wg.relationships != nil || wg.loading) {
		return
	}
	wf := workflow
	wg.workflow = &wf
	if wg.loading {
		wg.loadGen++
		wg.setLoading(false)
		wg.relationships = nil
	}
	wg.loadData()
}

// RefreshTheme updates colors after a theme change.
func (wg *WorkflowGraphView) RefreshTheme() {
	// Components auto-refresh via theme.Register
}

// Name returns the view name.
func (wg *WorkflowGraphView) Name() string {
	return "workflow-graph"
}

// Start is called when the view becomes active.
func (wg *WorkflowGraphView) Start() {
	wg.Split.SetInputCapture(wg.handleInput)
	wg.loadData()
}

// Stop is called when the view is deactivated.
func (wg *WorkflowGraphView) Stop() {
	wg.Split.SetInputCapture(nil)
}

// Hints returns keybinding hints for this view.
func (wg *WorkflowGraphView) Hints() []KeyHint {
	return []KeyHint{
		{Key: "space", Description: "Collapse/Expand"},
		{Key: "c", Description: "Center Graph"},
		{Key: "+/-", Description: "Depth"},
		{Key: "Enter", Description: "Open Detail"},
		{Key: "r", Description: "Refresh"},
		{Key: "?", Description: "Help"},
		{Key: "esc", Description: "Back"},
	}
}

// Focus sets focus to the tree pane.
func (wg *WorkflowGraphView) Focus(delegate func(p tview.Primitive)) {
	// Ensure Split focuses the first pane (tree)
	wg.Split.FocusFirst()
	wg.Split.Focus(delegate)
}

func (wg *WorkflowGraphView) handleGraphKeys(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}
	switch event.Rune() {
	case '+':
		wg.adjustDepth(1)
		return true
	case '-':
		wg.adjustDepth(-1)
		return true
	case 'r':
		wg.refresh()
		return true
	case 'c':
		if node := wg.tree.GetSelected(); node != nil {
			wg.graph.SetFocus(node.ID)
		}
		return true
	}
	return false
}

func (wg *WorkflowGraphView) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if wg.embedded {
		return event
	}
	switch event.Key() {
	case tcell.KeyEscape:
		wg.app.JigApp().Pages().Pop()
		return nil
	}
	if wg.handleGraphKeys(event) {
		return nil
	}
	if event.Rune() == 'q' {
		wg.app.JigApp().Pages().Pop()
		return nil
	}
	return event
}

func (wg *WorkflowGraphView) adjustDepth(delta int) {
	newDepth := wg.depthLimit + delta
	if newDepth < 1 {
		newDepth = 1
	}
	if newDepth > 10 {
		newDepth = 10
	}
	if newDepth != wg.depthLimit {
		wg.depthLimit = newDepth
		wg.app.ToastSuccess(fmt.Sprintf("Depth limit: %d", wg.depthLimit))
		wg.loadData()
	}
}

// Invalidate drops the relationship graph held for the current workflow so the
// next show refetches it. A load already in flight is abandoned, since its
// answer predates the refresh.
func (wg *WorkflowGraphView) Invalidate() {
	if wg == nil {
		return
	}
	wg.relationships = nil
	if wg.loading {
		wg.loadGen++
		wg.setLoading(false)
	}
}

// refresh refetches the relationship graph from the server.
func (wg *WorkflowGraphView) refresh() {
	wg.Invalidate()
	wg.loadData()
}

func (wg *WorkflowGraphView) setLoading(loading bool) {
	wg.loading = loading
	wg.app.SetViewLoading("workflow-graph", loading)
}

func (wg *WorkflowGraphView) loadData() {
	if wg.loading || wg.workflow == nil {
		return
	}
	wg.setLoading(true)
	wg.loadGen++
	gen := wg.loadGen
	id, runID := wg.workflow.ID, wg.workflow.RunID

	wg.showInitialState()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		finish := func(fn func()) {
			if wg.app != nil && wg.app.JigApp() != nil {
				wg.app.JigApp().QueueUpdateDraw(func() {
					if wg.loadGen != gen {
						return
					}
					fn()
				})
				return
			}
			if wg.loadGen == gen {
				fn()
			}
		}

		if wg.app == nil {
			finish(func() { wg.setLoading(false) })
			return
		}
		provider := wg.app.Provider()
		if provider == nil {
			finish(func() {
				if wg.app != nil {
					wg.app.ToastError("No provider available")
				}
				wg.setLoading(false)
			})
			return
		}

		relationships, err := provider.GetWorkflowRelationships(
			ctx,
			wg.namespace,
			id,
			runID,
			wg.depthLimit,
		)
		if err != nil {
			finish(func() {
				if wg.app != nil {
					wg.app.ToastError(fmt.Sprintf("Failed to load relationships: %v", err))
				}
				wg.setLoading(false)
			})
			return
		}

		finish(func() {
			wg.relationships = relationships
			wg.buildTreeData()
			wg.buildGraphData()
			wg.setLoading(false)
		})
	}()
}

// showInitialState displays the current workflow immediately while loading relationships.
func graphExecID(id, runID string) string {
	if runID == "" {
		return id
	}
	return id + "\x1e" + runID
}

func (wg *WorkflowGraphView) showInitialState() {
	if wg.workflow == nil {
		return
	}
	id := graphExecID(wg.workflow.ID, wg.workflow.RunID)
	wg.treeData = components.NewGraphTreeData()
	currentNode := &components.GraphTreeNode{
		ID:        id,
		Label:     truncateLabel(wg.workflow.ID, 30),
		Sublabel:  wg.workflow.Type,
		Status:    wg.workflow.Status,
		NodeType:  components.GraphNodePrimary,
		CanExpand: false,
		Loading:   true,
		Data:      wg.workflow,
	}
	wg.treeData.AddNode(currentNode)
	wg.treeData.RootID = id
	wg.tree.SetData(wg.treeData)

	wg.graphData = components.NewNodeGraphData()
	wg.graphData.AddNode(&components.GraphNode{
		ID:      id,
		Label:   truncateLabel(wg.workflow.ID, 14),
		Status:  wg.workflow.Status,
		Focused: true,
		Data:    wg.workflow,
	})
	wg.graphData.FocusID = id
	wg.graph.SetData(wg.graphData)
}

func (wg *WorkflowGraphView) buildTreeData() {
	wg.treeData = components.NewGraphTreeData()

	if wg.relationships == nil || wg.relationships.Current == nil {
		return
	}

	seen := make(map[string]bool)
	current := wg.relationships.Current
	currentID := graphExecID(current.ID, current.RunID)

	if parent := wg.relationships.Parent; parent != nil {
		parentID := graphExecID(parent.ID, parent.RunID)
		if parentID != "" && parentID != currentID {
			parentNode := &components.GraphTreeNode{
				ID:        parentID,
				Label:     parent.ID,
				Sublabel:  parent.Type,
				Status:    parent.Status,
				NodeType:  components.GraphNodeSecondary,
				CanExpand: false,
				Expanded:  true,
				Data:      parent,
			}
			wg.treeData.AddNode(parentNode)
			wg.treeData.RootID = parentID
			parentNode.Children = append(parentNode.Children, currentID)
			wg.treeData.AddEdge(&components.GraphTreeEdge{
				From: parentID,
				To:   currentID,
				Type: components.GraphEdgeSolid,
			})
			seen[parentID] = true
		}
	}

	currentNode := &components.GraphTreeNode{
		ID:        currentID,
		Label:     current.ID,
		Sublabel:  current.Type,
		Status:    current.Status,
		NodeType:  components.GraphNodePrimary,
		CanExpand: len(wg.relationships.Children) > 0,
		Expanded:  true,
		Data:      current,
	}
	if wg.treeData.RootID == "" {
		wg.treeData.RootID = currentID
	}
	wg.treeData.AddNode(currentNode)
	seen[currentID] = true

	wg.addChildrenToTree(currentNode, wg.relationships.Children, 1, seen)

	for _, signal := range wg.relationships.OutgoingSignals {
		signalID := fmt.Sprintf("signal-%s-%s", signal.FromWorkflowID, signal.ToWorkflowID)
		if seen[signalID] {
			continue
		}
		seen[signalID] = true
		signalNode := &components.GraphTreeNode{
			ID:       signalID,
			Label:    fmt.Sprintf("Signal: %s", signal.SignalName),
			Sublabel: fmt.Sprintf("-> %s", truncateLabel(signal.ToWorkflowID, 20)),
			NodeType: components.GraphNodeLink,
			Data:     signal,
		}
		wg.treeData.AddNode(signalNode)
		currentNode.Children = append(currentNode.Children, signalID)
		wg.treeData.AddEdge(&components.GraphTreeEdge{
			From:  currentID,
			To:    signalID,
			Type:  components.GraphEdgeDashed,
			Label: signal.SignalName,
		})
	}

	wg.tree.SetData(wg.treeData)
}

func (wg *WorkflowGraphView) addChildrenToTree(parentNode *components.GraphTreeNode, children []*temporal.WorkflowNode, depth int, seen map[string]bool) {
	for _, child := range children {
		id := graphExecID(child.ID, child.RunID)
		if id == "" || id == parentNode.ID || seen[id] {
			continue
		}
		seen[id] = true
		childNode := &components.GraphTreeNode{
			ID:        id,
			Label:     child.ID,
			Sublabel:  child.Type,
			Status:    child.Status,
			NodeType:  components.GraphNodeSecondary,
			Depth:     depth,
			CanExpand: len(child.Children) > 0,
			Expanded:  depth < 2,
			Data:      &child.Workflow,
		}

		wg.treeData.AddNode(childNode)
		parentNode.Children = append(parentNode.Children, id)

		edgeType := components.GraphEdgeSolid
		if child.EdgeType == "signal" {
			edgeType = components.GraphEdgeDashed
		} else if child.EdgeType == "continue" {
			edgeType = components.GraphEdgeDotted
		}

		wg.treeData.AddEdge(&components.GraphTreeEdge{
			From: parentNode.ID,
			To:   id,
			Type: edgeType,
		})

		if len(child.Children) > 0 {
			wg.addChildrenToTree(childNode, child.Children, depth+1, seen)
		}
	}
}

func (wg *WorkflowGraphView) buildGraphData() {
	wg.graphData = components.NewNodeGraphData()

	if wg.relationships == nil || wg.relationships.Current == nil {
		return
	}

	seen := make(map[string]bool)
	byWorkflow := make(map[string]string)
	current := wg.relationships.Current
	currentID := graphExecID(current.ID, current.RunID)

	if parent := wg.relationships.Parent; parent != nil {
		parentID := graphExecID(parent.ID, parent.RunID)
		if parentID != "" && parentID != currentID {
			wg.graphData.AddNode(&components.GraphNode{
				ID:     parentID,
				Label:  truncateLabel(parent.ID, 14),
				Status: parent.Status,
				Data:   parent,
			})
			wg.graphData.AddEdge(&components.GraphEdge{
				From: parentID,
				To:   currentID,
				Type: components.GraphEdgeSolid,
			})
			seen[parentID] = true
			byWorkflow[parent.ID] = parentID
		}
	}

	wg.graphData.AddNode(&components.GraphNode{
		ID:      currentID,
		Label:   truncateLabel(current.ID, 14),
		Status:  current.Status,
		Focused: true,
		Data:    current,
	})
	wg.graphData.FocusID = currentID
	seen[currentID] = true
	byWorkflow[current.ID] = currentID

	wg.addChildrenToGraph(currentID, wg.relationships.Children, seen, byWorkflow)

	for _, signal := range wg.relationships.OutgoingSignals {
		from := byWorkflow[signal.FromWorkflowID]
		to := byWorkflow[signal.ToWorkflowID]
		if from == "" || to == "" || from == to {
			continue
		}
		wg.graphData.AddEdge(&components.GraphEdge{
			From:  from,
			To:    to,
			Type:  components.GraphEdgeDashed,
			Label: signal.SignalName,
		})
	}

	wg.graph.SetData(wg.graphData)
}

func (wg *WorkflowGraphView) addChildrenToGraph(parentID string, children []*temporal.WorkflowNode, seen map[string]bool, byWorkflow map[string]string) {
	for _, child := range children {
		id := graphExecID(child.ID, child.RunID)
		if id == "" || id == parentID || seen[id] {
			continue
		}
		seen[id] = true
		if byWorkflow != nil {
			byWorkflow[child.ID] = id
		}
		wg.graphData.AddNode(&components.GraphNode{
			ID:     id,
			Label:  truncateLabel(child.ID, 14),
			Status: child.Status,
			Data:   &child.Workflow,
		})

		edgeType := components.GraphEdgeSolid
		if child.EdgeType == "signal" {
			edgeType = components.GraphEdgeDashed
		} else if child.EdgeType == "continue" {
			edgeType = components.GraphEdgeDotted
		}

		wg.graphData.AddEdge(&components.GraphEdge{
			From: parentID,
			To:   id,
			Type: edgeType,
		})

		if len(child.Children) > 0 {
			wg.addChildrenToGraph(id, child.Children, seen, byWorkflow)
		}
	}
}

func (wg *WorkflowGraphView) onTreeSelectionChange(node *components.GraphTreeNode) {
	if node == nil {
		return
	}
	// Update graph focus to match tree selection
	wg.graph.SetFocus(node.ID)
}

func (wg *WorkflowGraphView) onTreeSelect(node *components.GraphTreeNode) {
	if node == nil || node.Data == nil {
		return
	}

	// Navigate to workflow detail
	if wf, ok := node.Data.(*temporal.Workflow); ok {
		wg.app.NavigateToWorkflowDetail(wf.ID, wf.RunID)
	}
}

func (wg *WorkflowGraphView) onGraphSelect(node *components.GraphNode) {
	if node == nil || node.Data == nil {
		return
	}

	// Navigate to workflow detail
	if wf, ok := node.Data.(*temporal.Workflow); ok {
		wg.app.NavigateToWorkflowDetail(wf.ID, wf.RunID)
	}
}

func (wg *WorkflowGraphView) loadChildren(nodeID string) ([]*components.GraphTreeNode, []*components.GraphTreeEdge) {
	if wg.loading || wg.app == nil {
		return nil, nil
	}
	provider := wg.app.Provider()
	if provider == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Get the workflow for this node
	treeNode := wg.treeData.GetNode(nodeID)
	if treeNode == nil || treeNode.Data == nil {
		return nil, nil
	}

	wf, ok := treeNode.Data.(*temporal.Workflow)
	if !ok {
		return nil, nil
	}

	children, err := provider.GetChildWorkflows(ctx, wg.namespace, wf.ID, wf.RunID)
	if err != nil {
		return nil, nil
	}

	var nodes []*components.GraphTreeNode
	var edges []*components.GraphTreeEdge

	for _, child := range children {
		id := graphExecID(child.ID, child.RunID)
		if id == "" || id == nodeID {
			continue
		}
		if wg.treeData != nil && wg.treeData.GetNode(id) != nil {
			continue
		}
		childNode := &components.GraphTreeNode{
			ID:        id,
			Label:     truncateLabel(child.ID, 30),
			Sublabel:  child.Type,
			Status:    child.Status,
			NodeType:  components.GraphNodeSecondary,
			CanExpand: true,
			Data:      &child,
		}
		nodes = append(nodes, childNode)

		edges = append(edges, &components.GraphTreeEdge{
			From: nodeID,
			To:   id,
			Type: components.GraphEdgeSolid,
		})

		if wg.graphData != nil && wg.graphData.GetNode(id) == nil {
			wg.graphData.AddNode(&components.GraphNode{
				ID:     id,
				Label:  truncateLabel(child.ID, 14),
				Status: child.Status,
				Data:   &child,
			})
			wg.graphData.AddEdge(&components.GraphEdge{
				From: nodeID,
				To:   id,
				Type: components.GraphEdgeSolid,
			})
		}
	}

	return nodes, edges
}

func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
