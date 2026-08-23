package view

import (
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/galaxy-io/tempo/internal/temporal"
)

func previewCacheKey(workflowID, runID string) string {
	return workflowID + "\x00" + runID
}

type previewCache struct {
	limit int
	order []string
	items map[string][]temporal.EnhancedHistoryEvent
}

func newPreviewCache(limit int) *previewCache {
	if limit < 0 {
		limit = 0
	}
	return &previewCache{
		limit: limit,
		items: make(map[string][]temporal.EnhancedHistoryEvent),
	}
}

func previewCacheLimit(app *App) int {
	if app == nil {
		return config.DefaultPreviewCacheSize
	}
	return app.Config().PreviewCacheLimit()
}

func (c *previewCache) get(workflowID, runID string) ([]temporal.EnhancedHistoryEvent, bool) {
	if c == nil || c.limit == 0 {
		return nil, false
	}
	events, ok := c.items[previewCacheKey(workflowID, runID)]
	return events, ok
}

func (c *previewCache) put(workflowID, runID string, events []temporal.EnhancedHistoryEvent) {
	if c == nil || c.limit == 0 {
		return
	}
	key := previewCacheKey(workflowID, runID)
	if _, exists := c.items[key]; exists {
		c.items[key] = events
		return
	}
	for len(c.items) >= c.limit {
		c.evictOldest()
	}
	c.items[key] = events
	c.order = append(c.order, key)
}

func (c *previewCache) evictOldest() {
	if len(c.order) == 0 {
		return
	}
	oldest := c.order[0]
	c.order = c.order[1:]
	delete(c.items, oldest)
}

func (c *previewCache) setLimit(limit int) {
	if c == nil {
		return
	}
	if limit < 0 {
		limit = 0
	}
	c.limit = limit
	if limit == 0 {
		c.order = nil
		c.items = make(map[string][]temporal.EnhancedHistoryEvent)
		return
	}
	for len(c.items) > c.limit {
		c.evictOldest()
	}
}
