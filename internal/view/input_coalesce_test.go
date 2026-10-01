package view

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func simulatedCoalesceScreen(t *testing.T) (*coalesceScreen, tcell.SimulationScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sim.Fini)
	c := newCoalesceScreen(sim)
	c.wait = 20 * time.Millisecond
	return c, sim
}

func describeEvent(event tcell.Event) string {
	switch e := event.(type) {
	case *tcell.EventKey:
		if e.Key() == tcell.KeyRune {
			return string(e.Rune())
		}
		return tcell.KeyNames[e.Key()]
	case *tcell.EventMouse:
		switch {
		case e.Buttons()&tcell.WheelDown != 0:
			return "wheel-down"
		case e.Buttons()&tcell.WheelUp != 0:
			return "wheel-up"
		}
	}
	return "?"
}

func pollAll(c *coalesceScreen, n int) []string {
	var got []string
	for i := 0; i < n; i++ {
		got = append(got, describeEvent(c.PollEvent()))
		c.drawn()
	}
	return got
}

func waitQueued(t *testing.T, sim tcell.SimulationScreen) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !sim.HasPendingEvent() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
}

func TestHeldArrowCollapsesKeepingOrder(t *testing.T) {
	c, sim := simulatedCoalesceScreen(t)
	for i := 0; i < 5; i++ {
		sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	}
	sim.InjectKey(tcell.KeyRune, 'a', tcell.ModNone)
	for i := 0; i < 3; i++ {
		sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	}
	waitQueued(t, sim)

	got := pollAll(c, 3)
	want := []string{"Down", "a", "Down"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if sim.HasPendingEvent() || c.hasPending {
		t.Fatal("the queued repeats should all have collapsed")
	}
}

func TestHeldLettersAreNeverDropped(t *testing.T) {
	c, sim := simulatedCoalesceScreen(t)
	for i := 0; i < 3; i++ {
		sim.InjectKey(tcell.KeyRune, 'j', tcell.ModNone)
	}
	sim.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	sim.InjectKey(tcell.KeyBackspace2, 0, tcell.ModNone)
	waitQueued(t, sim)

	got := pollAll(c, 5)
	for i, want := range []string{"j", "j", "j", "Backspace", "Backspace"} {
		if got[i] != want {
			t.Fatalf("got %v", got)
		}
	}
}

func TestWheelBurstCollapsesByDirection(t *testing.T) {
	c, sim := simulatedCoalesceScreen(t)
	for i := 0; i < 4; i++ {
		sim.InjectMouse(1, 1, tcell.WheelDown, tcell.ModNone)
	}
	sim.InjectMouse(1, 1, tcell.WheelUp, tcell.ModNone)
	waitQueued(t, sim)

	got := pollAll(c, 2)
	if got[0] != "wheel-down" || got[1] != "wheel-up" {
		t.Fatalf("got %v", got)
	}
	if sim.HasPendingEvent() || c.hasPending {
		t.Fatal("the downward ticks should have collapsed into one")
	}
}

// A navigation event waits for the last one to be drawn, but not forever:
// some are never drawn.
func TestNavigationWaitsForTheLastDraw(t *testing.T) {
	c, sim := simulatedCoalesceScreen(t)
	sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitQueued(t, sim)
	c.PollEvent()

	sim.InjectKey(tcell.KeyUp, 0, tcell.ModNone)
	waitQueued(t, sim)
	start := time.Now()
	if got := describeEvent(c.PollEvent()); got != "Up" {
		t.Fatalf("got %s", got)
	}
	if waited := time.Since(start); waited < c.wait {
		t.Fatalf("an undrawn event should hold the next back, waited %s", waited)
	}

	c.drawn()
	sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
	waitQueued(t, sim)
	start = time.Now()
	c.PollEvent()
	if waited := time.Since(start); waited >= c.wait {
		t.Fatalf("a drawn event should let the next through at once, waited %s", waited)
	}
}
