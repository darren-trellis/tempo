package view

import (
	"context"
	"sync"
	"time"

	"github.com/galaxy-io/tempo/internal/temporal"
)

type startCatalog struct {
	types  []string
	queues []string
}

type catalogFetch struct {
	gen   uint64
	epoch uint64
}

type namespaceCatalogStore struct {
	mu        sync.RWMutex
	epoch     uint64
	items     map[string]startCatalog
	attrs     map[string][]temporal.SearchAttribute
	fetch     map[string]uint64
	pending   map[string]catalogFetch
	listeners map[string][]func(startCatalog)
}

func (s *namespaceCatalogStore) get(ns string) (startCatalog, bool) {
	if s == nil || ns == "" {
		return startCatalog{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.items[ns]
	if !ok {
		return startCatalog{}, false
	}
	return copyStartCatalog(c), true
}

func (s *namespaceCatalogStore) has(ns string) bool {
	if s == nil || ns == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.items[ns]
	return ok
}

func (s *namespaceCatalogStore) beginFetch(ns string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startFetchLocked(ns)
}

func (s *namespaceCatalogStore) tryBeginFetch(ns string) (uint64, bool) {
	if s == nil || ns == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pending, ok := s.pending[ns]; ok && pending.epoch == s.epoch {
		return 0, false
	}
	return s.startFetchLocked(ns), true
}

func (s *namespaceCatalogStore) startFetchLocked(ns string) uint64 {
	if s.fetch == nil {
		s.fetch = map[string]uint64{}
	}
	if s.pending == nil {
		s.pending = map[string]catalogFetch{}
	}
	s.fetch[ns]++
	gen := s.fetch[ns]
	s.pending[ns] = catalogFetch{gen: gen, epoch: s.epoch}
	return gen
}

func (s *namespaceCatalogStore) putIfCurrent(ns string, gen uint64, c startCatalog) {
	var listeners []func(startCatalog)
	s.mu.Lock()
	pending, ok := s.pending[ns]
	if !ok || pending.gen != gen || pending.epoch != s.epoch {
		s.mu.Unlock()
		return
	}
	delete(s.pending, ns)
	if s.items == nil {
		s.items = map[string]startCatalog{}
	}
	copied := copyStartCatalog(c)
	s.items[ns] = copied
	listeners = append(listeners, s.listeners[ns]...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(copyStartCatalog(copied))
	}
}

func (s *namespaceCatalogStore) abandonFetch(ns string, gen uint64) {
	if s == nil || ns == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[ns]
	if !ok || pending.gen != gen || pending.epoch != s.epoch {
		return
	}
	delete(s.pending, ns)
}

func (s *namespaceCatalogStore) listen(ns string, fn func(startCatalog)) {
	if s == nil || ns == "" || fn == nil {
		return
	}
	s.mu.Lock()
	if s.listeners == nil {
		s.listeners = map[string][]func(startCatalog){}
	}
	s.listeners[ns] = []func(startCatalog){fn}
	s.mu.Unlock()
}

func (s *namespaceCatalogStore) unlisten(ns string) {
	if s == nil || ns == "" {
		return
	}
	s.mu.Lock()
	delete(s.listeners, ns)
	s.mu.Unlock()
}

func (s *namespaceCatalogStore) getAttrs(ns string) []temporal.SearchAttribute {
	if s == nil || ns == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]temporal.SearchAttribute(nil), s.attrs[ns]...)
}

func (s *namespaceCatalogStore) putAttrs(ns string, attrs []temporal.SearchAttribute) {
	if s == nil || ns == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attrs == nil {
		s.attrs = map[string][]temporal.SearchAttribute{}
	}
	s.attrs[ns] = append([]temporal.SearchAttribute(nil), attrs...)
}

func (s *namespaceCatalogStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	s.items = nil
	s.attrs = nil
	s.pending = nil
	s.listeners = nil
}

func copyStartCatalog(c startCatalog) startCatalog {
	return startCatalog{
		types:  append([]string(nil), c.types...),
		queues: append([]string(nil), c.queues...),
	}
}

func (a *App) refreshNamespaceCatalog() {
	if a == nil {
		return
	}
	a.refreshNamespaceCatalogFor(a.catalogNamespace())
}

func (a *App) resetNamespaceCatalog() {
	if a == nil {
		return
	}
	a.catalog.clear()
	a.refreshNamespaceCatalog()
}

func (a *App) catalogNamespace() string {
	if a == nil {
		return ""
	}
	if ns := a.CurrentNamespace(); ns != "" {
		return ns
	}
	if provider := a.Provider(); provider != nil {
		return provider.Config().Namespace
	}
	return ""
}

func (a *App) refreshNamespaceCatalogFor(ns string) {
	if a == nil || ns == "" {
		return
	}
	provider := a.Provider()
	if provider == nil {
		return
	}
	gen, started := a.catalog.tryBeginFetch(ns)
	if !started {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		types, queues, err := listStartCatalog(ctx, provider, ns)
		if err != nil && len(types) == 0 && len(queues) == 0 {
			a.catalog.abandonFetch(ns, gen)
			return
		}
		a.catalog.putIfCurrent(ns, gen, startCatalog{types: types, queues: queues})
		if attrs, attrErr := listCustomSearchAttributes(ctx, provider, ns); attrErr == nil {
			a.catalog.putAttrs(ns, attrs)
		}
	}()
}

func listStartCatalog(ctx context.Context, provider temporal.Provider, namespace string) (types, queues []string, err error) {
	if provider == nil {
		return nil, nil, nil
	}
	return provider.ListStartCatalog(ctx, namespace)
}

func listCustomSearchAttributes(ctx context.Context, provider temporal.Provider, namespace string) ([]temporal.SearchAttribute, error) {
	if provider == nil {
		return nil, nil
	}
	return provider.ListCustomSearchAttributes(ctx, namespace)
}

func (a *App) catalogSuggestions(namespace string) (types, queues []string, ok bool) {
	if a == nil {
		return nil, nil, false
	}
	if namespace == "" {
		namespace = a.catalogNamespace()
	}
	c, ok := a.catalog.get(namespace)
	return c.types, c.queues, ok
}

func (a *App) watchStartCatalog(namespace string, typeField, queueField *dropdownField) {
	if a == nil || namespace == "" {
		return
	}
	a.catalog.listen(namespace, func(c startCatalog) {
		apply := func() {
			applyStartCatalog(typeField, queueField, c)
		}
		if jig := a.JigApp(); jig != nil {
			jig.QueueUpdateDraw(apply)
			return
		}
		apply()
	})
	if c, ok := a.catalog.get(namespace); ok {
		applyStartCatalog(typeField, queueField, c)
	}
}

func applyStartCatalog(typeField, queueField *dropdownField, c startCatalog) {
	if typeField != nil {
		typeField.SetOptions(c.types)
	}
	if queueField != nil {
		queueField.SetOptions(c.queues)
	}
}
