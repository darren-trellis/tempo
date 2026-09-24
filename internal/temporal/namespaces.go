package temporal

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"
	namespacepb "go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ListNamespaces returns all namespaces visible to the client.
func (c *Client) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}

	var namespaces []Namespace
	var nextPageToken []byte

	for {
		resp, err := cl.WorkflowService().ListNamespaces(ctx, &workflowservice.ListNamespacesRequest{
			PageSize:      100,
			NextPageToken: nextPageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list namespaces: %w", err)
		}

		for _, ns := range resp.GetNamespaces() {
			info := ns.GetNamespaceInfo()
			config := ns.GetConfig()

			retention := "N/A"
			if config.GetWorkflowExecutionRetentionTtl() != nil {
				retention = formatDuration(config.GetWorkflowExecutionRetentionTtl())
			}

			namespaces = append(namespaces, Namespace{
				Name:            info.GetName(),
				State:           MapNamespaceState(info.GetState()),
				RetentionPeriod: retention,
				Description:     info.GetDescription(),
				OwnerEmail:      info.GetOwnerEmail(),
			})
		}

		nextPageToken = resp.GetNextPageToken()
		if len(nextPageToken) == 0 {
			break
		}
	}

	return namespaces, nil
}

// CreateNamespace registers a new namespace with the Temporal server.
func (c *Client) CreateNamespace(ctx context.Context, req NamespaceCreateRequest) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	if req.RetentionDays < 1 {
		return fmt.Errorf("retention period must be at least 1 day")
	}

	retention := durationpb.New(time.Duration(req.RetentionDays) * 24 * time.Hour)

	_, err = cl.WorkflowService().RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        req.Name,
		Description:                      req.Description,
		OwnerEmail:                       req.OwnerEmail,
		WorkflowExecutionRetentionPeriod: retention,
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	return nil
}

// DescribeNamespace returns detailed information about a namespace.
func (c *Client) DescribeNamespace(ctx context.Context, name string) (*NamespaceDetail, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	resp, err := cl.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe namespace: %w", err)
	}

	info := resp.GetNamespaceInfo()
	config := resp.GetConfig()
	replication := resp.GetReplicationConfig()

	retention := "N/A"
	if config.GetWorkflowExecutionRetentionTtl() != nil {
		retention = formatDuration(config.GetWorkflowExecutionRetentionTtl())
	}

	// Format archival info
	historyArchival := formatArchivalState(config.GetHistoryArchivalState(), config.GetHistoryArchivalUri())
	visibilityArchival := formatArchivalState(config.GetVisibilityArchivalState(), config.GetVisibilityArchivalUri())

	// Extract cluster names
	var clusters []string
	for _, cluster := range replication.GetClusters() {
		clusters = append(clusters, cluster.GetClusterName())
	}

	detail := &NamespaceDetail{
		Namespace: Namespace{
			Name:            info.GetName(),
			State:           MapNamespaceState(info.GetState()),
			RetentionPeriod: retention,
			Description:     info.GetDescription(),
			OwnerEmail:      info.GetOwnerEmail(),
		},
		ID:                 info.GetId(),
		IsGlobalNamespace:  resp.GetIsGlobalNamespace(),
		FailoverVersion:    resp.GetFailoverVersion(),
		HistoryArchival:    historyArchival,
		VisibilityArchival: visibilityArchival,
		Clusters:           clusters,
	}

	// Parse timestamps if available
	if info.GetData() != nil {
		// Note: CreatedAt and UpdatedAt are not directly exposed in the API response
		// They would need to be extracted from namespace info data if stored there
	}

	return detail, nil
}

// UpdateNamespace modifies an existing namespace's configuration.
func (c *Client) UpdateNamespace(ctx context.Context, req NamespaceUpdateRequest) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	// First describe to get current state
	current, err := cl.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: req.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to get current namespace config: %w", err)
	}

	// Build update request preserving existing values where not specified
	updateReq := &workflowservice.UpdateNamespaceRequest{
		Namespace: req.Name,
	}

	// Update info fields
	description := req.Description
	ownerEmail := req.OwnerEmail
	if description == "" {
		description = current.GetNamespaceInfo().GetDescription()
	}
	if ownerEmail == "" {
		ownerEmail = current.GetNamespaceInfo().GetOwnerEmail()
	}
	updateReq.UpdateInfo = &namespacepb.UpdateNamespaceInfo{
		Description: description,
		OwnerEmail:  ownerEmail,
	}

	// Update config if retention specified
	if req.RetentionDays > 0 {
		updateReq.Config = &namespacepb.NamespaceConfig{
			WorkflowExecutionRetentionTtl: durationpb.New(time.Duration(req.RetentionDays) * 24 * time.Hour),
		}
	}

	_, err = cl.WorkflowService().UpdateNamespace(ctx, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update namespace: %w", err)
	}
	return nil
}

// DeprecateNamespace marks a namespace as deprecated (soft delete).
func (c *Client) DeprecateNamespace(ctx context.Context, name string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	_, err = cl.WorkflowService().DeprecateNamespace(ctx, &workflowservice.DeprecateNamespaceRequest{
		Namespace: name,
	})
	if err != nil {
		return fmt.Errorf("failed to deprecate namespace: %w", err)
	}
	return nil
}

// DeleteNamespace permanently deletes a namespace.
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	_, err = cl.OperatorService().DeleteNamespace(ctx, &operatorservice.DeleteNamespaceRequest{
		Namespace: name,
	})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}
	return nil
}

// formatArchivalState formats archival state and URI for display.
func formatArchivalState(state enums.ArchivalState, uri string) string {
	stateStr := "Disabled"
	switch state {
	case enums.ARCHIVAL_STATE_ENABLED:
		stateStr = "Enabled"
	case enums.ARCHIVAL_STATE_DISABLED:
		stateStr = "Disabled"
	}

	if uri != "" {
		return fmt.Sprintf("%s (%s)", stateStr, uri)
	}
	return stateStr
}

// formatDuration formats a protobuf duration as a human-readable string.
func formatDuration(d *durationpb.Duration) string {
	if d == nil {
		return "N/A"
	}

	dur := d.AsDuration()

	if dur < time.Hour {
		return fmt.Sprintf("%d minutes", int(dur.Minutes()))
	}
	if dur < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(dur.Hours()))
	}

	days := int(dur.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
