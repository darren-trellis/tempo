package temporal

import (
	"context"
	"strings"
	"testing"
)

// TestClientCallsWithoutConnectionReturnErrors guards against the crash class a
// profile switch used to cause: the connection is swapped out from under an
// in-flight call, and every entry point must report that rather than dereference
// a nil client.
func TestClientCallsWithoutConnectionReturnErrors(t *testing.T) {
	c := &Client{} // never connected: what an in-flight call saw mid-switch
	ctx := context.Background()

	if _, err := c.conn(); err == nil {
		t.Fatal("conn should refuse to hand out a nil connection")
	}

	calls := map[string]func() error{
		"ListNamespaces":             func() error { _, err := c.ListNamespaces(ctx); return err },
		"CreateNamespace":            func() error { return c.CreateNamespace(ctx, NamespaceCreateRequest{Name: "n", RetentionDays: 1}) },
		"DescribeNamespace":          func() error { _, err := c.DescribeNamespace(ctx, "n"); return err },
		"UpdateNamespace":            func() error { return c.UpdateNamespace(ctx, NamespaceUpdateRequest{Name: "n"}) },
		"DeprecateNamespace":         func() error { return c.DeprecateNamespace(ctx, "n") },
		"DeleteNamespace":            func() error { return c.DeleteNamespace(ctx, "n") },
		"ListWorkflows":              func() error { _, _, err := c.ListWorkflows(ctx, "n", ListOptions{}); return err },
		"CountWorkflows":             func() error { _, err := c.CountWorkflows(ctx, "n", ""); return err },
		"GetWorkflow":                func() error { _, err := c.GetWorkflow(ctx, "n", "w", "r"); return err },
		"GetWorkflowHistory":         func() error { _, err := c.GetWorkflowHistory(ctx, "n", "w", "r"); return err },
		"GetEnhancedWorkflowHistory": func() error { _, err := c.GetEnhancedWorkflowHistory(ctx, "n", "w", "r"); return err },
		"DescribeTaskQueue":          func() error { _, _, err := c.DescribeTaskQueue(ctx, "n", "q"); return err },
		"ListWorkers":                func() error { _, err := c.ListWorkers(ctx, "n"); return err },
		"ListTaskQueueNames":         func() error { _, err := c.ListTaskQueueNames(ctx, "n"); return err },
		"ListWorkflowTypes":          func() error { _, err := c.ListWorkflowTypes(ctx, "n"); return err },
		"ListStartCatalog":           func() error { _, _, err := c.ListStartCatalog(ctx, "n"); return err },
		"ListSchedules":              func() error { _, _, err := c.ListSchedules(ctx, "n", ListOptions{}); return err },
		"GetSchedule":                func() error { _, err := c.GetSchedule(ctx, "n", "s"); return err },
		"PauseSchedule":              func() error { return c.PauseSchedule(ctx, "n", "s", "why") },
		"UnpauseSchedule":            func() error { return c.UnpauseSchedule(ctx, "n", "s", "why") },
		"TriggerSchedule":            func() error { return c.TriggerSchedule(ctx, "n", "s") },
		"DeleteSchedule":             func() error { return c.DeleteSchedule(ctx, "n", "s") },
		"CancelWorkflow":             func() error { return c.CancelWorkflow(ctx, "n", "w", "r", "why") },
		"TerminateWorkflow":          func() error { return c.TerminateWorkflow(ctx, "n", "w", "r", "why") },
		"SignalWorkflow":             func() error { return c.SignalWorkflow(ctx, "n", "w", "r", "sig", []byte("{}")) },
		"DeleteWorkflow":             func() error { return c.DeleteWorkflow(ctx, "n", "w", "r") },
		"ResetWorkflow":              func() error { _, err := c.ResetWorkflow(ctx, "n", "w", "r", 1, "why"); return err },
		"QueryWorkflow":              func() error { _, err := c.QueryWorkflow(ctx, "n", "w", "r", "q", nil); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("expected an error while disconnected")
			}
			if !strings.Contains(err.Error(), "not connected") {
				t.Logf("%s reported: %v", name, err)
			}
		})
	}
}

// TestReconnectKeepsWorkingConnectionOnFailure covers the other half: a profile
// that fails to dial must not take the current connection down with it.
func TestReconnectKeepsWorkingConnectionOnFailure(t *testing.T) {
	c := &Client{connected: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // fail the dial immediately

	err := c.ReconnectWithConfig(ctx, ConnectionConfig{Address: "127.0.0.1:1", Namespace: "other"})
	if err == nil {
		t.Fatal("expected the dial to fail")
	}
	if !c.IsConnected() {
		t.Fatal("a failed switch should leave the existing connection reported as up")
	}
	if got := c.Config().Namespace; got == "other" {
		t.Fatal("a failed switch should not adopt the new profile's config")
	}
}

func TestNewClientRequiresCloudAPIKey(t *testing.T) {
	_, err := NewClient(context.Background(), ConnectionConfig{
		Address:   "us-west-2.aws.api.temporal.io:7233",
		Namespace: "prod-beta.cvhrv",
	})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("got %v", err)
	}
}
