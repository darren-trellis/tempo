package view

import (
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// coalesceWait is the longest a navigation event waits for the one before it
// to be drawn. Some never are, such as a wheel tick against the end of a
// pane, so the wait has to end on its own.
const coalesceWait = 100 * time.Millisecond

// coalesceScreen keeps held navigation keys and wheel scrolls from piling
// up. tview redraws the whole screen for every key event and queues up to a
// hundred more behind it, so a key held faster than tempo can draw built a
// backlog that kept moving the cursor long after the key was let go. Each
// navigation event now waits for the one before it to be drawn, and repeats
// that arrive meanwhile collapse into the newest. Letters and Backspace are
// never collapsed, so held keys in a text field still type every character.
// A wheel tick carries distance, so collapsed ticks are counted and replayed
// by replayWheelTicks within the one event that survives.
type coalesceScreen struct {
	tcell.Screen
	ready      chan struct{}
	pending    tcell.Event
	hasPending bool
	wait       time.Duration

	mu     sync.Mutex
	bursts map[*tcell.EventMouse]int
}

func newCoalesceScreen(screen tcell.Screen) *coalesceScreen {
	c := &coalesceScreen{Screen: screen, ready: make(chan struct{}, 1), wait: coalesceWait}
	c.ready <- struct{}{}
	return c
}

// drawn is called after every draw, by which point the event last handed
// over has run.
func (c *coalesceScreen) drawn() {
	select {
	case c.ready <- struct{}{}:
	default:
	}
}

func (c *coalesceScreen) next() tcell.Event {
	if c.hasPending {
		c.hasPending = false
		event := c.pending
		c.pending = nil
		return event
	}
	return c.Screen.PollEvent()
}

func (c *coalesceScreen) PollEvent() tcell.Event {
	event := c.next()
	if !coalescible(event) {
		return event
	}
	select {
	case <-c.ready:
	case <-time.After(c.wait):
	}
	collapsed := 0
	for !c.hasPending && c.Screen.HasPendingEvent() {
		following := c.Screen.PollEvent()
		if sameRepeat(event, following) {
			event = following
			collapsed++
			continue
		}
		c.pending = following
		c.hasPending = true
	}
	if mouse, ok := event.(*tcell.EventMouse); ok && collapsed > 0 {
		c.mu.Lock()
		if c.bursts == nil {
			c.bursts = make(map[*tcell.EventMouse]int)
		}
		c.bursts[mouse] = collapsed
		c.mu.Unlock()
	}
	return event
}

// extraTicks reports, once, how many wheel ticks were collapsed into event.
func (c *coalesceScreen) extraTicks(event *tcell.EventMouse) int {
	if c == nil || event == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.bursts[event]
	delete(c.bursts, event)
	return n
}

// replayWheelTicks wraps the app's mouse capture so a wheel event that stands
// for several collapsed ticks scrolls as far as all of them would have, while
// costing a single redraw. Every tick, the event's own included, takes the
// route tview would give it: through the capture, then to the page under the
// pointer. The event is then reported consumed so tview always redraws: the
// ticks may have carried a pane to its end, where tview would otherwise drop
// the last one without drawing what the others moved.
func replayWheelTicks(app *tview.Application, pages tview.Primitive, extra func(*tcell.EventMouse) int) {
	if app == nil || pages == nil {
		return
	}
	prev := app.GetMouseCapture()
	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		switch action {
		case tview.MouseScrollUp, tview.MouseScrollDown, tview.MouseScrollLeft, tview.MouseScrollRight:
			if n := extra(event); n > 0 {
				for ; n >= 0; n-- {
					ev, act := event, action
					if prev != nil {
						if ev, act = prev(ev, act); ev == nil {
							continue
						}
					}
					if handler := pages.MouseHandler(); handler != nil {
						handler(act, ev, func(p tview.Primitive) { app.SetFocus(p) })
					}
				}
				return nil, tview.MouseConsumed
			}
		}
		if prev != nil {
			return prev(event, action)
		}
		return event, action
	})
}

// coalescible reports whether an event only moves something around, so a
// burst of them can be cut short without losing anything the user meant.
func coalescible(event tcell.Event) bool {
	switch e := event.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
			tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
			return true
		}
	case *tcell.EventMouse:
		return e.Buttons()&wheelButtons != 0
	}
	return false
}

const wheelButtons = tcell.WheelUp | tcell.WheelDown | tcell.WheelLeft | tcell.WheelRight

func sameRepeat(a, b tcell.Event) bool {
	switch x := a.(type) {
	case *tcell.EventKey:
		y, ok := b.(*tcell.EventKey)
		return ok && x.Key() == y.Key() && x.Modifiers() == y.Modifiers()
	case *tcell.EventMouse:
		y, ok := b.(*tcell.EventMouse)
		return ok && x.Buttons()&wheelButtons != 0 && x.Buttons() == y.Buttons() && x.Modifiers() == y.Modifiers()
	}
	return false
}
