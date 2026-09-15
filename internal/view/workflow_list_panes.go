package view

import (
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	defaultPrimaryWeight   = 11
	defaultSecondaryWeight = 9
	defaultSecondaryStack  = 3
	defaultTertiaryStack   = 2
	minPrimaryWidth        = 24
	minSecondaryWidth      = 24
	minPrimaryHeight       = 8
	minSecondaryHeight     = 6
	minTertiaryHeight      = 5
	minTimelineHeight      = 5
)

type listPane int

const (
	panePrimary listPane = iota
	paneSecondary
	paneTertiary
)

func (wl *WorkflowList) focusedListPane() listPane {
	switch wl.focusPane {
	case focusEvents, focusPollers, focusScheduleDetail, focusWorkerDetail:
		return paneSecondary
	case focusEventDetail, focusScheduleRuns:
		return paneTertiary
	default:
		return panePrimary
	}
}

func paneResizeArrow(event *tcell.EventKey) (tcell.Key, bool) {
	if event == nil {
		return 0, false
	}
	mods := event.Modifiers()
	if mods&tcell.ModShift == 0 {
		return 0, false
	}
	if mods&tcell.ModAlt == 0 && mods&tcell.ModMeta == 0 {
		return 0, false
	}
	switch event.Key() {
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
		return event.Key(), true
	}
	return 0, false
}

func (wl *WorkflowList) handlePaneResizeKey(event *tcell.EventKey) bool {
	key, ok := paneResizeArrow(event)
	if !ok {
		return false
	}
	switch key {
	case tcell.KeyLeft:
		wl.resizePrimaryWidth(-1)
	case tcell.KeyRight:
		wl.resizePrimaryWidth(1)
	case tcell.KeyUp, tcell.KeyDown:
		delta := 1
		if key == tcell.KeyDown {
			delta = -1
		}
		switch wl.focusedListPane() {
		case paneSecondary, paneTertiary:
			wl.resizeTertiaryHeight(delta)
		default:
			wl.resizeTimelineHeight(delta)
		}
	}
	return true
}

func (wl *WorkflowList) secondaryColumnVisible() bool {
	if wl.taskQueuesActive() {
		return wl.pollersVisible
	}
	if wl.schedulesActive() {
		return wl.scheduleDetailVisible
	}
	if wl.workersActive() {
		return wl.workerDetailVisible
	}
	return wl.workflowsActive() && wl.previewModeEnabled()
}

func (wl *WorkflowList) primaryColumnItem() tview.Primitive {
	if wl.mainFlex == nil || wl.mainFlex.GetItemCount() == 0 {
		return nil
	}
	return wl.mainFlex.GetItem(0)
}

func (wl *WorkflowList) tertiaryStack() (*tview.Flex, tview.Primitive) {
	if wl.schedulesActive() && wl.scheduleDetailVisible && wl.schedules != nil {
		return wl.schedules.detailFlex, wl.schedules.runsPanel
	}
	if !wl.workflowsActive() || !wl.previewModeEnabled() || wl.rightFlex == nil {
		return nil, nil
	}
	if wl.previewShowsSidePane() {
		return wl.rightFlex, wl.eventDetailPanel
	}
	if wl.previewKind == previewHierarchy {
		return wl.rightFlex, wl.hierarchyGraphPanel
	}
	return nil, nil
}

func (wl *WorkflowList) currentPrimaryWidth() int {
	if wl.primaryWidth > 0 {
		return wl.primaryWidth
	}
	if item := wl.primaryColumnItem(); item != nil {
		if _, _, w, _ := item.GetRect(); w > 0 {
			return w
		}
	}
	if wl.mainFlex != nil {
		if _, _, w, _ := wl.mainFlex.GetInnerRect(); w > 0 {
			return w * defaultPrimaryWeight / (defaultPrimaryWeight + defaultSecondaryWeight)
		}
	}
	return 0
}

func (wl *WorkflowList) currentTertiaryHeight() int {
	if wl.tertiaryHeight > 0 {
		return wl.tertiaryHeight
	}
	if _, tertiary := wl.tertiaryStack(); tertiary != nil {
		if _, _, _, h := tertiary.GetRect(); h > 0 {
			return h
		}
	}
	if flex, _ := wl.tertiaryStack(); flex != nil {
		if _, _, _, h := flex.GetInnerRect(); h > 0 {
			return h * defaultTertiaryStack / (defaultSecondaryStack + defaultTertiaryStack)
		}
	}
	return 0
}

func (wl *WorkflowList) resizePrimaryWidth(delta int) bool {
	if !wl.secondaryColumnVisible() {
		return false
	}
	cur := wl.currentPrimaryWidth()
	if cur == 0 {
		return false
	}
	parent := 0
	if wl.mainFlex != nil {
		_, _, parent, _ = wl.mainFlex.GetInnerRect()
	}
	next := clampSplit(cur+delta, minPrimaryWidth, parent, minSecondaryWidth)
	if next == 0 || next == cur {
		return false
	}
	wl.primaryWidth = next
	wl.applyPrimaryWidthNow()
	return true
}

func (wl *WorkflowList) resizeTertiaryHeight(delta int) bool {
	flex, tertiary := wl.tertiaryStack()
	if flex == nil || tertiary == nil {
		return false
	}
	cur := wl.currentTertiaryHeight()
	if cur == 0 {
		return false
	}
	_, _, _, parent := flex.GetInnerRect()
	next := clampSplit(cur+delta, minTertiaryHeight, parent, minSecondaryHeight)
	if next == 0 || next == cur {
		return false
	}
	wl.tertiaryHeight = next
	wl.applyTertiaryHeightNow()
	return true
}

func (wl *WorkflowList) resizeTimelineHeight(delta int) bool {
	if !wl.timelineVisible || wl.timelinePanel == nil {
		return false
	}
	cur := wl.timelineSize()
	if wl.timelineHeight == 0 {
		if _, _, _, h := wl.timelinePanel.GetRect(); h > 0 {
			cur = h
		}
	}
	parent := 0
	if wl.timelineDocked() && wl.primaryStack != nil {
		_, _, _, parent = wl.primaryStack.GetInnerRect()
	} else {
		_, _, _, parent = wl.GetInnerRect()
	}
	next := clampSplit(cur+delta, minTimelineHeight, parent, minPrimaryHeight)
	if next == 0 || next == cur {
		return false
	}
	wl.timelineHeight = next
	wl.applyTimelineHeightNow()
	return true
}

func clampSplit(next, minSelf, parent, minOther int) int {
	if next < minSelf {
		next = minSelf
	}
	if parent > 0 && parent-next < minOther {
		next = parent - minOther
	}
	if next < minSelf {
		return 0
	}
	return next
}

func (wl *WorkflowList) timelineSize() int {
	if wl.timelineHeight > 0 {
		return wl.timelineHeight
	}
	return timelinePanelHeight
}

func (wl *WorkflowList) addPrimarySecondary(primary, secondary tview.Primitive, primaryFocus bool) {
	if wl.mainFlex == nil || primary == nil {
		return
	}
	if secondary == nil {
		wl.mainFlex.AddItem(primary, 0, 1, primaryFocus)
		return
	}
	if w := wl.primaryWidth; w > 0 {
		wl.mainFlex.AddItem(primary, w, 0, primaryFocus)
		wl.mainFlex.AddItem(secondary, 0, 1, false)
		return
	}
	wl.mainFlex.AddItem(primary, 0, defaultPrimaryWeight, primaryFocus)
	wl.mainFlex.AddItem(secondary, 0, defaultSecondaryWeight, false)
}

func (wl *WorkflowList) addSecondaryTertiary(flex *tview.Flex, secondary, tertiary tview.Primitive) {
	if flex == nil || secondary == nil {
		return
	}
	if tertiary == nil {
		flex.AddItem(secondary, 0, 1, false)
		return
	}
	if h := wl.tertiaryHeight; h > 0 {
		flex.AddItem(secondary, 0, 1, false)
		flex.AddItem(tertiary, h, 0, false)
		return
	}
	flex.AddItem(secondary, 0, defaultSecondaryStack, false)
	flex.AddItem(tertiary, 0, defaultTertiaryStack, false)
}

func (wl *WorkflowList) applyPrimaryWidthNow() {
	if wl.mainFlex == nil || wl.mainFlex.GetItemCount() < 2 || wl.primaryWidth <= 0 {
		return
	}
	wl.mainFlex.ResizeItem(wl.mainFlex.GetItem(0), wl.primaryWidth, 0)
	wl.mainFlex.ResizeItem(wl.mainFlex.GetItem(1), 0, 1)
}

func (wl *WorkflowList) applyTertiaryHeightNow() {
	flex, tertiary := wl.tertiaryStack()
	if flex == nil || tertiary == nil || flex.GetItemCount() < 2 || wl.tertiaryHeight <= 0 {
		return
	}
	flex.ResizeItem(flex.GetItem(0), 0, 1)
	flex.ResizeItem(tertiary, wl.tertiaryHeight, 0)
}

func (wl *WorkflowList) applyTimelineHeightNow() {
	if wl.timelinePanel == nil || wl.timelineSize() <= 0 {
		return
	}
	h := wl.timelineSize()
	if wl.timelineDocked() && wl.primaryStack != nil {
		wl.primaryStack.ResizeItem(wl.timelinePanel, h, 0)
		return
	}
	if wl.timelineVisible && !wl.timelineDocked() {
		wl.ResizeItem(wl.timelinePanel, h, 0)
	}
}

func (wl *WorkflowList) applyStoredPaneSizes() {
	if wl.primaryWidth > 0 {
		wl.applyPrimaryWidthNow()
	}
	if wl.tertiaryHeight > 0 {
		wl.applyTertiaryHeightNow()
	}
	if wl.timelineVisible {
		wl.applyTimelineHeightNow()
	}
}

func (wl *WorkflowList) ensurePrimaryStack() *tview.Flex {
	if wl.primaryStack == nil {
		wl.primaryStack = tview.NewFlex().SetDirection(tview.FlexRow)
		wl.primaryStack.SetBackgroundColor(theme.Bg())
	}
	return wl.primaryStack
}
