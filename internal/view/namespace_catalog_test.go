package view

import (
	"testing"
)

func TestNamespaceCatalogStoresPerNamespace(t *testing.T) {
	var store namespaceCatalogStore
	store.putIfCurrent("default", store.beginFetch("default"), startCatalog{
		types:  []string{"OrderWorkflow"},
		queues: []string{"orders"},
	})
	store.putIfCurrent("prod", store.beginFetch("prod"), startCatalog{
		types:  []string{"PaymentWorkflow"},
		queues: []string{"payments"},
	})

	got, ok := store.get("default")
	if !ok || len(got.types) != 1 || got.types[0] != "OrderWorkflow" || got.queues[0] != "orders" {
		t.Fatalf("default=%+v ok=%v", got, ok)
	}
	got, ok = store.get("prod")
	if !ok || got.types[0] != "PaymentWorkflow" {
		t.Fatalf("prod=%+v ok=%v", got, ok)
	}
}

func TestNamespaceCatalogIgnoresStaleFetch(t *testing.T) {
	var store namespaceCatalogStore
	first := store.beginFetch("default")
	second := store.beginFetch("default")
	store.putIfCurrent("default", first, startCatalog{types: []string{"Stale"}})
	if store.has("default") {
		t.Fatal("a superseded fetch should not write")
	}
	store.putIfCurrent("default", second, startCatalog{types: []string{"Current"}})
	got, ok := store.get("default")
	if !ok || len(got.types) != 1 || got.types[0] != "Current" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestNamespaceCatalogClearDropsItemsAndStaleWrites(t *testing.T) {
	var store namespaceCatalogStore
	gen := store.beginFetch("default")
	store.putIfCurrent("default", gen, startCatalog{types: []string{"OrderWorkflow"}})
	store.clear()
	if store.has("default") {
		t.Fatal("clear should drop the catalog")
	}
	store.putIfCurrent("default", gen, startCatalog{types: []string{"Stale"}})
	if store.has("default") {
		t.Fatal("a fetch started before clear should not write")
	}
}

func TestNamespaceCatalogListenFiresOnPut(t *testing.T) {
	var store namespaceCatalogStore
	var got startCatalog
	store.listen("default", func(c startCatalog) { got = c })
	store.putIfCurrent("default", store.beginFetch("default"), startCatalog{
		types:  []string{"OrderWorkflow"},
		queues: []string{"orders"},
	})
	if len(got.types) != 1 || got.types[0] != "OrderWorkflow" || got.queues[0] != "orders" {
		t.Fatalf("got=%+v", got)
	}
}

func TestStartCatalogAppliesToTypeaheadFields(t *testing.T) {
	typeField := newTypeaheadField("workflowType", "Workflow Type", []string{"OldWorkflow"})
	queueField := newTypeaheadField("taskQueue", "Task Queue", []string{"old-queue"})
	applyStartCatalog(typeField, queueField, startCatalog{
		types:  []string{"OrderWorkflow", "PaymentWorkflow"},
		queues: []string{"orders", "payments"},
	})
	if len(typeField.options) != 2 || typeField.options[0] != "OrderWorkflow" {
		t.Fatalf("types=%v", typeField.options)
	}
	if len(queueField.options) != 2 || queueField.options[0] != "orders" {
		t.Fatalf("queues=%v", queueField.options)
	}
}

func TestNamespaceCatalogTryBeginFetchIsSingleFlight(t *testing.T) {
	var store namespaceCatalogStore
	if _, started := store.tryBeginFetch("default"); !started {
		t.Fatal("first fetch should start")
	}
	if _, started := store.tryBeginFetch("default"); started {
		t.Fatal("a second fetch for the same namespace should wait")
	}
}

func TestNamespaceCatalogClearAllowsANewFetch(t *testing.T) {
	var store namespaceCatalogStore
	old := store.beginFetch("default")
	store.putIfCurrent("default", old, startCatalog{types: []string{"OldWorkflow"}})
	store.clear()
	if store.has("default") {
		t.Fatal("clear should drop the previous profile catalog")
	}
	gen, started := store.tryBeginFetch("default")
	if !started {
		t.Fatal("a profile switch should start a new fetch")
	}
	store.putIfCurrent("default", old, startCatalog{types: []string{"OldWorkflow"}})
	if store.has("default") {
		t.Fatal("a fetch from the previous profile should not write")
	}
	store.putIfCurrent("default", gen, startCatalog{types: []string{"NewWorkflow"}, queues: []string{"new-queue"}})
	got, ok := store.get("default")
	if !ok || len(got.types) != 1 || got.types[0] != "NewWorkflow" || got.queues[0] != "new-queue" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestStartWorkflowSuggestionsUseCatalogOnly(t *testing.T) {
	app := &App{currentNS: "default"}
	app.catalog.putIfCurrent("default", app.catalog.beginFetch("default"), startCatalog{
		types:  []string{"CatalogWorkflow"},
		queues: []string{"catalog-queue"},
	})
	types, queues := startWorkflowSuggestions(app, "default")
	if len(types) != 1 || types[0] != "CatalogWorkflow" {
		t.Fatalf("types=%v", types)
	}
	if len(queues) != 1 || queues[0] != "catalog-queue" {
		t.Fatalf("queues=%v", queues)
	}
}

func TestStartWorkflowSuggestionsIgnorePageUntilCatalogReady(t *testing.T) {
	app := &App{currentNS: "default"}
	types, queues := startWorkflowSuggestions(app, "default")
	if len(types) != 0 || len(queues) != 0 {
		t.Fatalf("empty catalog should not invent page suggestions, types=%v queues=%v", types, queues)
	}
}

func TestStartWorkflowSuggestionsIgnoreOtherNamespace(t *testing.T) {
	app := &App{currentNS: "prod"}
	app.catalog.putIfCurrent("default", app.catalog.beginFetch("default"), startCatalog{
		types: []string{"OldWorkflow"},
	})
	types, queues := startWorkflowSuggestions(app, "prod")
	if len(types) != 0 || len(queues) != 0 {
		t.Fatalf("types=%v queues=%v", types, queues)
	}
}
