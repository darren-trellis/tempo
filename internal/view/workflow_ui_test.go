package view

import (
	"testing"

	"github.com/galaxy-io/tempo/internal/config"
)

func TestWorkflowUILinkInfersLocalhost(t *testing.T) {
	a := &App{
		config: &config.Config{
			ActiveProfile: "local",
			Profiles: map[string]config.ConnectionConfig{
				"local": {Address: "localhost:7233", Namespace: "default"},
			},
		},
		activeProfile: "local",
		currentNS:     "orders",
	}
	got, err := a.workflowUILink("order/123", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://localhost:8080/namespaces/orders/workflows/order%2F123/run-1/history"
	if got != want {
		t.Fatalf("link=%q want %q", got, want)
	}
}

func TestWorkflowUILinkUsesProfileURL(t *testing.T) {
	a := &App{
		config: &config.Config{
			ActiveProfile: "staging",
			Profiles: map[string]config.ConnectionConfig{
				"staging": {
					Address: "temporal.staging.example.com:7233",
					UIURL:   "https://temporal.staging.example.com/",
				},
			},
		},
		activeProfile: "staging",
		currentNS:     "staging",
	}
	got, err := a.workflowUILink("wf", "run")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://temporal.staging.example.com/namespaces/staging/workflows/wf/run/history"
	if got != want {
		t.Fatalf("link=%q want %q", got, want)
	}
}

func TestWorkflowUILinkRequiresUIURL(t *testing.T) {
	a := &App{
		config: &config.Config{
			ActiveProfile: "custom",
			Profiles: map[string]config.ConnectionConfig{
				"custom": {Address: "temporal.internal:7233"},
			},
		},
		activeProfile: "custom",
		currentNS:     "default",
	}
	if _, err := a.workflowUILink("wf", "run"); err == nil {
		t.Fatal("unknown address should require ui_url")
	}
}

func TestWorkflowListHintsWebUI(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	if desc := hintDescription(wl.Hints(), "u"); desc != "Web UI" {
		t.Fatalf("u hint: %q", desc)
	}
}

func TestWorkflowDetailHintsWebUI(t *testing.T) {
	wd := NewWorkflowDetail(&App{}, "wf", "run")
	if desc := hintDescription(wd.Hints(), "u"); desc != "Web UI" {
		t.Fatalf("u hint: %q", desc)
	}
}
