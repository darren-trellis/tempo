package temporal

import "testing"

func TestWorkflowsInIDsOrder(t *testing.T) {
	found := []Workflow{
		{ID: "third"},
		{ID: "first"},
		{ID: "second"},
	}
	got := workflowsInIDsOrder([]string{"first", "second", "third", "missing"}, found)
	if len(got) != 3 {
		t.Fatalf("len: %d", len(got))
	}
	if got[0].ID != "first" || got[1].ID != "second" || got[2].ID != "third" {
		t.Fatalf("order: %q %q %q", got[0].ID, got[1].ID, got[2].ID)
	}
}
