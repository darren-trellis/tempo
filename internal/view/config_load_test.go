package view

import (
	"testing"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
)

// :config load runs on the UI goroutine. tview's QueueUpdateDraw waits for
// the event loop to run the update, so queueing it from there in place
// deadlocks.
func TestConfigLoadDoesNotBlockTheUIThread(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.DefaultConfig().Save(); err != nil {
		t.Fatal(err)
	}
	a := NewAppWithProvider(nil, "default", config.DefaultConfig(), "local")

	done := make(chan struct{})
	go func() {
		a.execConfig([]string{"load"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("config load hung waiting on the UI thread")
	}
	if a.config == nil {
		t.Fatal("the loaded config should be applied")
	}
}
