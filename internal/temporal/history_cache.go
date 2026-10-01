package temporal

import "sync"

// historyCacheSize is how many runs keep their decoded events. A large
// history holds megabytes of decoded payloads, so only the few runs looked
// at most recently are kept.
const historyCacheSize = 4

// historyCache keeps decoded events of recently loaded runs so a reload only
// has to decode what was appended since.
type historyCache struct {
	mu      sync.Mutex
	order   []string
	entries map[string][]EnhancedHistoryEvent
}

func historyCacheKey(namespace, workflowID, runID string) string {
	return namespace + "\x00" + workflowID + "\x00" + runID
}

func (h *historyCache) get(key string) []EnhancedHistoryEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.entries[key]
}

// put keeps its own copy of events, so changes callers make to theirs do not
// leak into later loads.
func (h *historyCache) put(key string, events []EnhancedHistoryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entries == nil {
		h.entries = make(map[string][]EnhancedHistoryEvent)
	}
	for i, k := range h.order {
		if k == key {
			h.order = append(h.order[:i], h.order[i+1:]...)
			break
		}
	}
	h.order = append(h.order, key)
	h.entries[key] = append([]EnhancedHistoryEvent(nil), events...)
	for len(h.order) > historyCacheSize {
		delete(h.entries, h.order[0])
		h.order = h.order[1:]
	}
}

func (h *historyCache) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.order = nil
	h.entries = nil
}
