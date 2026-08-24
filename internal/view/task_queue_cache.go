package view

import "github.com/galaxy-io/tempo/internal/temporal"

type taskQueueCacheEntry struct {
	pollers     []temporal.Poller
	pollerCount int
	backlog     int
}

type taskQueueCache struct {
	limit int
	order []string
	items map[string]taskQueueCacheEntry
}

func taskQueueCacheKey(namespace, queue string) string {
	return namespace + "\x00" + queue
}

func newTaskQueueCache(limit int) *taskQueueCache {
	if limit < 0 {
		limit = 0
	}
	return &taskQueueCache{
		limit: limit,
		items: make(map[string]taskQueueCacheEntry),
	}
}

func (c *taskQueueCache) get(namespace, queue string) (taskQueueCacheEntry, bool) {
	if c == nil || c.limit == 0 {
		return taskQueueCacheEntry{}, false
	}
	entry, ok := c.items[taskQueueCacheKey(namespace, queue)]
	if !ok {
		return taskQueueCacheEntry{}, false
	}
	entry.pollers = copyPollers(entry.pollers)
	return entry, true
}

func (c *taskQueueCache) put(namespace, queue string, entry taskQueueCacheEntry) {
	if c == nil || c.limit == 0 || queue == "" {
		return
	}
	entry.pollers = copyPollers(entry.pollers)
	key := taskQueueCacheKey(namespace, queue)
	if _, exists := c.items[key]; exists {
		c.items[key] = entry
		return
	}
	for len(c.items) >= c.limit {
		c.evictOldest()
	}
	c.items[key] = entry
	c.order = append(c.order, key)
}

// clear drops every cached queue. A refresh must not be answered from a
// snapshot taken before it.
func (c *taskQueueCache) clear() {
	if c == nil {
		return
	}
	c.order = nil
	c.items = make(map[string]taskQueueCacheEntry)
}

// remove drops one queue's cached pollers.
func (c *taskQueueCache) remove(namespace, queue string) {
	if c == nil {
		return
	}
	key := taskQueueCacheKey(namespace, queue)
	if _, ok := c.items[key]; !ok {
		return
	}
	delete(c.items, key)
	for i, existing := range c.order {
		if existing == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (c *taskQueueCache) evictOldest() {
	if len(c.order) == 0 {
		return
	}
	oldest := c.order[0]
	c.order = c.order[1:]
	delete(c.items, oldest)
}

func (c *taskQueueCache) setLimit(limit int) {
	if c == nil {
		return
	}
	if limit < 0 {
		limit = 0
	}
	c.limit = limit
	if limit == 0 {
		c.order = nil
		c.items = make(map[string]taskQueueCacheEntry)
		return
	}
	for len(c.items) > c.limit {
		c.evictOldest()
	}
}

func copyPollers(in []temporal.Poller) []temporal.Poller {
	if in == nil {
		return nil
	}
	out := make([]temporal.Poller, len(in))
	copy(out, in)
	return out
}
