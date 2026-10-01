package view

import (
	"time"

	"github.com/gdamore/tcell/v2"
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
type coalesceScreen struct {
	tcell.Screen
	ready      chan struct{}
	pending    tcell.Event
	hasPending bool
	wait       time.Duration
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
	for !c.hasPending && c.Screen.HasPendingEvent() {
		following := c.Screen.PollEvent()
		if sameRepeat(event, following) {
			event = following
			continue
		}
		c.pending = following
		c.hasPending = true
	}
	return event
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
