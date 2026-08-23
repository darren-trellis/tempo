package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/temporal"
)

func TestPreviewCacheFIFO(t *testing.T) {
	c := newPreviewCache(2)
	c.put("a", "1", []temporal.EnhancedHistoryEvent{{ID: 1}})
	c.put("b", "2", []temporal.EnhancedHistoryEvent{{ID: 2}})
	c.put("a", "1", []temporal.EnhancedHistoryEvent{{ID: 11}})
	c.put("c", "3", []temporal.EnhancedHistoryEvent{{ID: 3}})

	if _, ok := c.get("a", "1"); ok {
		t.Fatal("FIFO should evict the first inserted workflow")
	}
	if events, ok := c.get("b", "2"); !ok || events[0].ID != 2 {
		t.Fatal("second insert should still be present")
	}
	if events, ok := c.get("c", "3"); !ok || events[0].ID != 3 {
		t.Fatal("newest insert should be present")
	}
}

func TestPreviewCacheDisabled(t *testing.T) {
	c := newPreviewCache(0)
	c.put("a", "1", []temporal.EnhancedHistoryEvent{{ID: 1}})
	if _, ok := c.get("a", "1"); ok {
		t.Fatal("size 0 should not cache")
	}
}

func TestPreviewCacheSetLimit(t *testing.T) {
	c := newPreviewCache(3)
	c.put("a", "1", []temporal.EnhancedHistoryEvent{{ID: 1}})
	c.put("b", "2", []temporal.EnhancedHistoryEvent{{ID: 2}})
	c.put("c", "3", []temporal.EnhancedHistoryEvent{{ID: 3}})
	c.setLimit(1)
	if _, ok := c.get("a", "1"); ok {
		t.Fatal("shrinking should evict the oldest entries")
	}
	if _, ok := c.get("c", "3"); !ok {
		t.Fatal("newest entry should survive a shrink")
	}
}
