package temporal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/galaxy-io/tempo/internal/config"
	"github.com/google/uuid"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	namespacepb "go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	logFile   *os.File
	sdkLogger *fileLogger
)

// fileLogger writes logs to a file.
type fileLogger struct {
	logger *log.Logger
}

func (l *fileLogger) Debug(msg string, keyvals ...interface{}) {
	l.logger.Printf("DEBUG: %s %v", msg, keyvals)
}

func (l *fileLogger) Info(msg string, keyvals ...interface{}) {
	l.logger.Printf("INFO: %s %v", msg, keyvals)
}

func (l *fileLogger) Warn(msg string, keyvals ...interface{}) {
	l.logger.Printf("WARN: %s %v", msg, keyvals)
}

func (l *fileLogger) Error(msg string, keyvals ...interface{}) {
	l.logger.Printf("ERROR: %s %v", msg, keyvals)
}

// initLogFile sets up logging to a file in the config directory.
func initLogFile() {
	if logFile != nil {
		return
	}

	logPath := filepath.Join(config.ConfigDir(), "tempo.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fall back to discarding logs if we can't open the file
		sdkLogger = &fileLogger{logger: log.New(os.Stderr, "", 0)}
		return
	}
	logFile = f
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	sdkLogger = &fileLogger{logger: log.New(f, "", log.Ldate|log.Ltime)}
}

// staticHeadersProvider implements the Temporal SDK HeadersProvider interface,
// returning a fixed set of gRPC metadata headers on every outgoing request.
type staticHeadersProvider struct {
	headers map[string]string
}

func (p *staticHeadersProvider) GetHeaders(_ context.Context) (map[string]string, error) {
	return p.headers, nil
}

// Client implements the Provider interface using the Temporal SDK.
type Client struct {
	client    client.Client
	config    ConnectionConfig
	connected bool
	mu        sync.RWMutex
}

// NewClient creates a new Temporal SDK client with the given configuration.
func NewClient(ctx context.Context, connConfig ConnectionConfig) (*Client, error) {
	// Redirect logs to file instead of stdout
	initLogFile()

	if config.IsTemporalCloudAddress(connConfig.Address) && connConfig.APIKey == "" {
		return nil, fmt.Errorf("Temporal Cloud requires an API key")
	}

	opts := client.Options{
		HostPort:  connConfig.Address,
		Namespace: connConfig.Namespace,
		Logger:    sdkLogger,
	}

	// Configure authentication
	if connConfig.APIKey != "" {
		// API Key authentication (Temporal Cloud)
		opts.Credentials = client.NewAPIKeyStaticCredentials(connConfig.APIKey)
		// API key auth requires TLS but doesn't need client certificates
		opts.ConnectionOptions.TLS = &tls.Config{}
	} else if connConfig.TLSCertPath != "" || connConfig.TLSCAPath != "" || connConfig.TLSSkipVerify {
		// mTLS authentication
		tlsConfig, err := buildTLSConfig(connConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		opts.ConnectionOptions.TLS = tlsConfig
	}

	// Attach custom gRPC metadata headers if configured
	if len(connConfig.GRPCMeta) > 0 {
		opts.HeadersProvider = &staticHeadersProvider{headers: connConfig.GRPCMeta}
	}
	if dc := dataConverterFor(connConfig); dc != nil {
		opts.DataConverter = dc
	}

	c, err := client.DialContext(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Temporal server: %w", err)
	}

	return &Client{
		client:    c,
		config:    connConfig,
		connected: true,
	}, nil
}

// buildTLSConfig creates a TLS configuration from the connection config.
func buildTLSConfig(config ConnectionConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.TLSSkipVerify,
	}

	if config.TLSServerName != "" {
		tlsConfig.ServerName = config.TLSServerName
	}

	// Load client certificate if provided
	if config.TLSCertPath != "" && config.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(config.TLSCertPath, config.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if config.TLSCAPath != "" {
		caCert, err := os.ReadFile(config.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// Close releases the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	if c.client != nil {
		c.client.Close()
	}
	return nil
}

// conn returns the active connection. Callers must go through this rather than
// reading c.client directly: a profile switch swaps the connection under them.
func (c *Client) conn() (client.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	return c.client, nil
}

// IsConnected returns true if the client has an active connection.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// CheckConnection verifies the connection is still alive by making a lightweight API call.
func (c *Client) CheckConnection(ctx context.Context) error {
	c.mu.RLock()
	cl := c.client
	c.mu.RUnlock()

	if cl == nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return fmt.Errorf("client is nil")
	}

	// Make a lightweight API call to check connection
	// ListNamespaces with PageSize 1 is a good health check
	_, err := cl.WorkflowService().ListNamespaces(ctx, &workflowservice.ListNamespacesRequest{
		PageSize: 1,
	})
	if err != nil {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		return fmt.Errorf("connection check failed: %w", err)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	return nil
}

// Reconnect attempts to re-establish a connection to the Temporal server.
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.RLock()
	reconnConfig := c.config
	c.mu.RUnlock()
	return c.reconnectWithConfig(ctx, reconnConfig)
}

// ReconnectWithConfig reconnects using a new configuration.
// This enables hot-swapping to a different Temporal server/namespace.
func (c *Client) ReconnectWithConfig(ctx context.Context, newConfig ConnectionConfig) error {
	return c.reconnectWithConfig(ctx, newConfig)
}

// reconnectWithConfig is the internal implementation for reconnection.
func (c *Client) reconnectWithConfig(ctx context.Context, connConfig ConnectionConfig) error {
	// The old connection stays in place until the new one is dialed. Clearing it
	// first left every in-flight call dereferencing a nil client for the length of
	// the dial, and a failed switch dropped a working connection.

	if config.IsTemporalCloudAddress(connConfig.Address) && connConfig.APIKey == "" {
		return fmt.Errorf("Temporal Cloud requires an API key")
	}

	opts := client.Options{
		HostPort:  connConfig.Address,
		Namespace: connConfig.Namespace,
		Logger:    sdkLogger,
	}

	// Configure authentication
	if connConfig.APIKey != "" {
		// API Key authentication (Temporal Cloud)
		opts.Credentials = client.NewAPIKeyStaticCredentials(connConfig.APIKey)
		// API key auth requires TLS but doesn't need client certificates
		opts.ConnectionOptions.TLS = &tls.Config{}
	} else if connConfig.TLSCertPath != "" || connConfig.TLSCAPath != "" || connConfig.TLSSkipVerify {
		// mTLS authentication
		tlsConfig, err := buildTLSConfig(connConfig)
		if err != nil {
			return fmt.Errorf("failed to configure TLS: %w", err)
		}
		opts.ConnectionOptions.TLS = tlsConfig
	}

	// Attach custom gRPC metadata headers if configured
	if len(connConfig.GRPCMeta) > 0 {
		opts.HeadersProvider = &staticHeadersProvider{headers: connConfig.GRPCMeta}
	}
	if dc := dataConverterFor(connConfig); dc != nil {
		opts.DataConverter = dc
	}

	newClient, err := client.DialContext(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	c.mu.Lock()
	previous := c.client
	c.client = newClient
	c.config = connConfig // Update stored config
	c.connected = true
	c.mu.Unlock()

	// Calls still running on the old connection now fail with an error rather
	// than crashing on a nil client.
	if previous != nil {
		previous.Close()
	}

	return nil
}

// Config returns the connection configuration used by this client.
func (c *Client) Config() ConnectionConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

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

// ListWorkflows returns workflows for a namespace with optional filtering.
func (c *Client) ListWorkflows(ctx context.Context, namespace string, opts ListOptions) ([]Workflow, string, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, "", err
	}

	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	req := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     namespace,
		PageSize:      int32(pageSize),
		NextPageToken: []byte(opts.PageToken),
	}

	if opts.Query != "" {
		req.Query = opts.Query
	}

	resp, err := cl.WorkflowService().ListWorkflowExecutions(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list workflows: %w", err)
	}

	decodePayloadsInMessages(c.payloadCodec(namespace), asProtoMessages(resp.GetExecutions())...)

	var workflows []Workflow
	for _, exec := range resp.GetExecutions() {
		workflows = append(workflows, workflowFromExecutionInfo(exec, namespace))
	}

	return workflows, string(resp.GetNextPageToken()), nil
}

// GetWorkflow returns details for a specific workflow execution.
func (c *Client) GetWorkflow(ctx context.Context, namespace, workflowID, runID string) (*Workflow, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}

	resp, err := cl.WorkflowService().DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: namespace,
		Execution: &commonpb.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      runID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe workflow: %w", err)
	}

	wf := workflowFromExecutionInfo(resp.GetWorkflowExecutionInfo(), namespace)
	if !wf.TaskFailure && wf.Status == "Running" {
		if task := resp.GetPendingWorkflowTask(); task != nil && task.GetAttempt() > 1 {
			wf.TaskFailure = true
		}
	}

	// Note: Input/Output are populated separately from event history
	// to avoid redundant API calls. See workflow_detail.go loadData().

	return &wf, nil
}

// DescribeTaskQueue returns task queue info and active pollers.
func (c *Client) DescribeTaskQueue(ctx context.Context, namespace, taskQueue string) (*TaskQueueInfo, []Poller, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, nil, err
	}
	type descResult struct {
		resp *workflowservice.DescribeTaskQueueResponse
		err  error
	}
	describe := func(tqType enums.TaskQueueType) descResult {
		resp, err := cl.WorkflowService().DescribeTaskQueue(ctx, &workflowservice.DescribeTaskQueueRequest{
			Namespace: namespace,
			TaskQueue: &taskqueue.TaskQueue{
				Name: taskQueue,
				Kind: enums.TASK_QUEUE_KIND_NORMAL,
			},
			TaskQueueType: tqType,
		})
		return descResult{resp: resp, err: err}
	}

	var wf, act descResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		wf = describe(enums.TASK_QUEUE_TYPE_WORKFLOW)
	}()
	go func() {
		defer wg.Done()
		act = describe(enums.TASK_QUEUE_TYPE_ACTIVITY)
	}()
	wg.Wait()
	if wf.err != nil && act.err != nil {
		return nil, nil, fmt.Errorf("failed to describe workflow task queue: %w", wf.err)
	}

	var pollers []Poller
	if wf.resp != nil {
		for _, p := range wf.resp.GetPollers() {
			pollers = append(pollers, Poller{
				Identity:       p.GetIdentity(),
				LastAccessTime: p.GetLastAccessTime().AsTime(),
				TaskQueueType:  TaskQueueTypeWorkflow,
				RatePerSecond:  p.GetRatePerSecond(),
			})
		}
	}
	if act.resp != nil {
		for _, p := range act.resp.GetPollers() {
			pollers = append(pollers, Poller{
				Identity:       p.GetIdentity(),
				LastAccessTime: p.GetLastAccessTime().AsTime(),
				TaskQueueType:  TaskQueueTypeActivity,
				RatePerSecond:  p.GetRatePerSecond(),
			})
		}
	}

	info := &TaskQueueInfo{
		Name:        taskQueue,
		Type:        "Combined",
		PollerCount: len(pollers),
		Backlog:     0,
	}

	return info, pollers, nil
}

func (c *Client) ListWorkers(ctx context.Context, namespace string) ([]Worker, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	var workers []Worker
	var token []byte
	for {
		resp, err := cl.WorkflowService().ListWorkers(ctx, &workflowservice.ListWorkersRequest{
			Namespace:     namespace,
			PageSize:      100,
			NextPageToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list workers: %w", err)
		}
		for _, info := range resp.GetWorkersInfo() {
			if w, ok := WorkerFromHeartbeat(info.GetWorkerHeartbeat()); ok {
				workers = append(workers, w)
			}
		}
		token = resp.GetNextPageToken()
		if len(token) == 0 {
			break
		}
	}
	return workers, nil
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

// CancelWorkflow requests graceful cancellation of a workflow execution.
func (c *Client) CancelWorkflow(ctx context.Context, namespace, workflowID, runID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.CancelWorkflow(ctx, workflowID, runID)
}

// TerminateWorkflow forcefully terminates a workflow execution immediately.
func (c *Client) TerminateWorkflow(ctx context.Context, namespace, workflowID, runID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.TerminateWorkflow(ctx, workflowID, runID, reason)
}

// SignalWorkflow sends a signal to a running workflow execution.
func (c *Client) SignalWorkflow(ctx context.Context, namespace, workflowID, runID, signalName string, input []byte) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	return cl.SignalWorkflow(ctx, workflowID, runID, signalName, json.RawMessage(input))
}

// StartWorkflow starts a new workflow execution.
func (c *Client) StartWorkflow(ctx context.Context, namespace string, req StartWorkflowRequest) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	opts := client.StartWorkflowOptions{
		ID:        req.WorkflowID,
		TaskQueue: req.TaskQueue,
	}

	args := []interface{}{}
	if len(req.Input) > 0 {
		args = append(args, json.RawMessage(req.Input))
	}

	run, err := cl.ExecuteWorkflow(ctx, opts, req.WorkflowType, args...)
	if err != nil {
		return "", fmt.Errorf("failed to start workflow: %w", err)
	}
	return run.GetRunID(), nil
}

// SignalWithStartWorkflow starts a workflow if it doesn't exist and sends a signal to it.
func (c *Client) SignalWithStartWorkflow(ctx context.Context, namespace string, req SignalWithStartRequest) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	opts := client.StartWorkflowOptions{
		ID:        req.WorkflowID,
		TaskQueue: req.TaskQueue,
	}

	run, err := cl.SignalWithStartWorkflow(
		ctx,
		req.WorkflowID,
		req.SignalName,
		json.RawMessage(req.SignalInput),
		opts,
		req.WorkflowType,
		json.RawMessage(req.WorkflowInput),
	)
	if err != nil {
		return "", fmt.Errorf("failed to signal with start workflow: %w", err)
	}
	return run.GetRunID(), nil
}

// DeleteWorkflow permanently deletes a workflow execution and its history.
func (c *Client) DeleteWorkflow(ctx context.Context, namespace, workflowID, runID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	_, err = cl.WorkflowService().DeleteWorkflowExecution(ctx,
		&workflowservice.DeleteWorkflowExecutionRequest{
			Namespace: namespace,
			WorkflowExecution: &commonpb.WorkflowExecution{
				WorkflowId: workflowID,
				RunId:      runID,
			},
		})
	return err
}

// ResetWorkflow resets a workflow to a previous state, creating a new run.
func (c *Client) ResetWorkflow(ctx context.Context, namespace, workflowID, runID string, eventID int64, reason string) (string, error) {
	cl, err := c.conn()
	if err != nil {
		return "", err
	}
	resp, err := cl.WorkflowService().ResetWorkflowExecution(ctx, resetWorkflowRequest(namespace, workflowID, runID, eventID, reason))
	if err != nil {
		return "", err
	}
	return resp.GetRunId(), nil
}

func resetWorkflowRequest(namespace, workflowID, runID string, eventID int64, reason string) *workflowservice.ResetWorkflowExecutionRequest {
	return &workflowservice.ResetWorkflowExecutionRequest{
		Namespace: namespace,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      runID,
		},
		Reason:                    reason,
		WorkflowTaskFinishEventId: eventID,
		RequestId:                 uuid.NewString(),
	}
}

// ListSchedules returns all schedules in a namespace.
func (c *Client) ListSchedules(ctx context.Context, namespace string, opts ListOptions) ([]Schedule, string, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, "", err
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	resp, err := cl.ScheduleClient().List(ctx, client.ScheduleListOptions{
		PageSize: pageSize,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list schedules: %w", err)
	}

	var schedules []Schedule
	for resp.HasNext() {
		entry, err := resp.Next()
		if err != nil {
			return nil, "", fmt.Errorf("failed to iterate schedules: %w", err)
		}

		schedule := Schedule{
			ID:           entry.ID,
			Paused:       entry.Paused,
			Notes:        entry.Note,
			WorkflowType: entry.WorkflowType.Name,
			RecentRuns:   convertScheduleRuns(entry.RecentActions),
		}

		// Extract spec info
		if entry.Spec != nil {
			schedule.Spec = formatScheduleSpec(entry.Spec)
		}

		// Recent and future actions
		if len(entry.RecentActions) > 0 {
			lastAction := entry.RecentActions[len(entry.RecentActions)-1]
			t := lastAction.ActualTime
			schedule.LastRunTime = &t
		}
		if len(entry.NextActionTimes) > 0 {
			t := entry.NextActionTimes[0]
			schedule.NextRunTime = &t
		}

		schedules = append(schedules, schedule)
	}

	return schedules, "", nil
}

// GetSchedule returns details for a specific schedule.
func (c *Client) GetSchedule(ctx context.Context, namespace, scheduleID string) (*Schedule, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	desc, err := handle.Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to describe schedule: %w", err)
	}

	schedule := &Schedule{
		ID:     scheduleID,
		Paused: desc.Schedule.State.Paused,
		Notes:  desc.Schedule.State.Note,
	}

	// Extract workflow info from action
	if desc.Schedule.Action != nil {
		if startAction, ok := desc.Schedule.Action.(*client.ScheduleWorkflowAction); ok {
			// Workflow is an interface{} representing the workflow type
			if wfType, ok := startAction.Workflow.(string); ok {
				schedule.WorkflowType = wfType
			}
			schedule.WorkflowID = startAction.ID
			schedule.TaskQueue = startAction.TaskQueue
		}
	}

	// Extract spec info
	if desc.Schedule.Spec != nil {
		schedule.Spec = formatScheduleSpec(desc.Schedule.Spec)
	}

	// Info from description
	schedule.TotalActions = int64(desc.Info.NumActions)
	schedule.RecentRuns = convertScheduleRuns(desc.Info.RecentActions)
	if len(desc.Info.RecentActions) > 0 {
		lastAction := desc.Info.RecentActions[len(desc.Info.RecentActions)-1]
		t := lastAction.ActualTime
		schedule.LastRunTime = &t
	}
	if len(desc.Info.NextActionTimes) > 0 {
		t := desc.Info.NextActionTimes[0]
		schedule.NextRunTime = &t
	}

	return schedule, nil
}

// PauseSchedule pauses a schedule.
func (c *Client) PauseSchedule(ctx context.Context, namespace, scheduleID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Pause(ctx, client.SchedulePauseOptions{
		Note: reason,
	})
}

// UnpauseSchedule unpauses a schedule.
func (c *Client) UnpauseSchedule(ctx context.Context, namespace, scheduleID, reason string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Unpause(ctx, client.ScheduleUnpauseOptions{
		Note: reason,
	})
}

// TriggerSchedule immediately triggers a scheduled workflow execution.
func (c *Client) TriggerSchedule(ctx context.Context, namespace, scheduleID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Trigger(ctx, client.ScheduleTriggerOptions{})
}

// DeleteSchedule permanently deletes a schedule.
func (c *Client) DeleteSchedule(ctx context.Context, namespace, scheduleID string) error {
	cl, err := c.conn()
	if err != nil {
		return err
	}
	handle := cl.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Delete(ctx)
}

func convertScheduleRuns(actions []client.ScheduleActionResult) []ScheduleRun {
	if len(actions) == 0 {
		return nil
	}

	runs := make([]ScheduleRun, 0, len(actions))
	for _, action := range actions {
		run := ScheduleRun{
			ScheduleTime: action.ScheduleTime,
			ActualTime:   action.ActualTime,
		}
		if action.StartWorkflowResult != nil {
			run.WorkflowID = action.StartWorkflowResult.WorkflowID
			run.RunID = action.StartWorkflowResult.FirstExecutionRunID
		}
		runs = append(runs, run)
	}

	return runs
}

// formatScheduleSpec creates a human-readable schedule specification.
func formatScheduleSpec(spec *client.ScheduleSpec) string {
	if spec == nil {
		return ""
	}

	var parts []string

	// Check for cron expressions
	if len(spec.CronExpressions) > 0 {
		parts = append(parts, spec.CronExpressions[0])
	}

	// Check for intervals
	if len(spec.Intervals) > 0 {
		interval := spec.Intervals[0]
		parts = append(parts, fmt.Sprintf("every %s", interval.Every))
	}

	// Check for calendars
	if len(spec.Calendars) > 0 {
		parts = append(parts, "calendar-based")
	}

	if len(parts) == 0 {
		return "custom"
	}

	return strings.Join(parts, ", ")
}

// QueryWorkflow executes a query against a running workflow and returns the result.
func (c *Client) QueryWorkflow(ctx context.Context, namespace, workflowID, runID, queryType string, args []byte) (*QueryResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	// Build query input if args provided
	var queryArgs interface{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &queryArgs); err != nil {
			// If not valid JSON, pass as raw string
			queryArgs = string(args)
		}
	}

	// Execute the query
	response, err := cl.QueryWorkflow(ctx, workflowID, runID, queryType, queryArgs)
	if err != nil {
		return &QueryResult{
			QueryType: queryType,
			Error:     err.Error(),
		}, nil
	}

	// Decode the result
	var result interface{}
	if err := response.Get(&result); err != nil {
		return &QueryResult{
			QueryType: queryType,
			Error:     fmt.Sprintf("failed to decode query result: %v", err),
		}, nil
	}

	// Format result as JSON for display
	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &QueryResult{
			QueryType: queryType,
			Result:    fmt.Sprintf("%v", result),
		}, nil
	}

	return &QueryResult{
		QueryType: queryType,
		Result:    string(resultJSON),
	}, nil
}

// CancelWorkflows cancels multiple workflows and returns results for each.
func (c *Client) CancelWorkflows(ctx context.Context, namespace string, workflows []WorkflowIdentifier) ([]BatchResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	results := make([]BatchResult, len(workflows))

	for i, wf := range workflows {
		err := cl.CancelWorkflow(ctx, wf.WorkflowID, wf.RunID)
		results[i] = BatchResult{
			WorkflowID: wf.WorkflowID,
			RunID:      wf.RunID,
			Success:    err == nil,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}

	return results, nil
}

// TerminateWorkflows terminates multiple workflows and returns results for each.
func (c *Client) TerminateWorkflows(ctx context.Context, namespace string, workflows []WorkflowIdentifier, reason string) ([]BatchResult, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	results := make([]BatchResult, len(workflows))

	for i, wf := range workflows {
		err := cl.TerminateWorkflow(ctx, wf.WorkflowID, wf.RunID, reason)
		results[i] = BatchResult{
			WorkflowID: wf.WorkflowID,
			RunID:      wf.RunID,
			Success:    err == nil,
		}
		if err != nil {
			results[i].Error = err.Error()
		}
	}

	return results, nil
}

// GetResetPoints returns valid reset points for a workflow execution.
func (c *Client) GetResetPoints(ctx context.Context, namespace, workflowID, runID string) ([]ResetPoint, error) {
	// Get workflow history to find reset points
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, err
	}

	var resetPoints []ResetPoint

	// Track activity/timer state for building descriptions
	activityInfo := make(map[int64]string) // scheduledEventID -> activity type
	timerInfo := make(map[int64]string)    // startedEventID -> timer ID

	for _, event := range events {
		// Track activity scheduled events
		if strings.Contains(event.Type, "ActivityTaskScheduled") {
			activityInfo[event.ID] = event.ActivityType
		}

		// Track timer started events
		if strings.Contains(event.Type, "TimerStarted") {
			timerInfo[event.ID] = event.TimerID
		}

		// WorkflowTaskCompleted events are valid reset points
		if strings.Contains(event.Type, "WorkflowTaskCompleted") {
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ID,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Workflow task completed at event %d", event.ID),
				Reason:      "Reset to this workflow task",
			})
		}

		// ActivityTaskFailed - reset to before the failure
		if strings.Contains(event.Type, "ActivityTaskFailed") {
			actType := activityInfo[event.ScheduledEventID]
			if actType == "" {
				actType = "Unknown"
			}
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1, // Reset to workflow task before activity was scheduled
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Activity '%s' failed: %s", actType, truncateString(event.Failure, 50)),
				Reason:      "Reset to retry failed activity",
			})
		}

		// ActivityTaskTimedOut - reset to before the timeout
		if strings.Contains(event.Type, "ActivityTaskTimedOut") {
			actType := activityInfo[event.ScheduledEventID]
			if actType == "" {
				actType = "Unknown"
			}
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Activity '%s' timed out", actType),
				Reason:      "Reset to retry timed out activity",
			})
		}

		// WorkflowTaskFailed - this is a good reset point
		if strings.Contains(event.Type, "WorkflowTaskFailed") {
			resetPoints = append(resetPoints, ResetPoint{
				EventID:     event.ScheduledEventID - 1,
				EventType:   event.Type,
				Timestamp:   event.Time,
				Description: fmt.Sprintf("Workflow task failed: %s", truncateString(event.Failure, 50)),
				Reason:      "Reset to retry failed workflow task",
			})
		}
	}

	return resetPoints, nil
}

// truncateString truncates a string to maxLen and adds ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetChildWorkflows returns immediate child workflows spawned by a workflow.
// This parses the workflow history for ChildWorkflowExecutionStarted events.
func (c *Client) GetChildWorkflows(ctx context.Context, namespace, workflowID, runID string) ([]Workflow, error) {
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow history: %w", err)
	}
	return c.getChildWorkflowsFromEvents(ctx, namespace, events)
}

// getChildWorkflowsFromEvents extracts child workflows from pre-fetched events.
// This avoids duplicate history fetches when called from GetWorkflowRelationships.
func (c *Client) getChildWorkflowsFromEvents(ctx context.Context, namespace string, events []EnhancedHistoryEvent) ([]Workflow, error) {
	// Track child workflow IDs
	var childIDs []string
	seen := make(map[string]bool)

	for _, event := range events {
		// ChildWorkflowExecutionStarted contains the child's run ID
		if strings.Contains(event.Type, "ChildWorkflowExecutionStarted") {
			if event.ChildWorkflowID != "" && !seen[event.ChildWorkflowID] {
				childIDs = append(childIDs, event.ChildWorkflowID)
				seen[event.ChildWorkflowID] = true
			}
		}
	}

	if len(childIDs) == 0 {
		return nil, nil
	}

	// Fetch child workflows in parallel (limit concurrency to avoid overwhelming the server)
	type result struct {
		workflow Workflow
		err      error
	}

	results := make(chan result, len(childIDs))
	sem := make(chan struct{}, 5) // Max 5 concurrent requests

	for _, childID := range childIDs {
		go func(id string) {
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			query := fmt.Sprintf("WorkflowId = '%s'", id)
			workflows, _, err := c.ListWorkflows(ctx, namespace, ListOptions{
				PageSize: 1,
				Query:    query,
			})
			if err != nil {
				results <- result{err: err}
				return
			}
			if len(workflows) > 0 {
				results <- result{workflow: workflows[0]}
			} else {
				results <- result{}
			}
		}(childID)
	}

	found := make([]Workflow, 0, len(childIDs))
	for range childIDs {
		r := <-results
		if r.err == nil && r.workflow.ID != "" {
			found = append(found, r.workflow)
		}
	}

	return workflowsInIDsOrder(childIDs, found), nil
}

func workflowsInIDsOrder(ids []string, found []Workflow) []Workflow {
	byID := make(map[string]Workflow, len(found))
	for _, w := range found {
		if w.ID == "" {
			continue
		}
		byID[w.ID] = w
	}
	children := make([]Workflow, 0, len(ids))
	for _, id := range ids {
		if w, ok := byID[id]; ok {
			children = append(children, w)
		}
	}
	return children
}

// GetWorkflowRelationships returns the complete relationship graph for a workflow.
// depth controls how many levels of children to fetch (1 = immediate children only).
func (c *Client) GetWorkflowRelationships(ctx context.Context, namespace, workflowID, runID string, depth int) (*WorkflowRelationships, error) {
	// Get the current workflow details
	current, err := c.GetWorkflow(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	result := &WorkflowRelationships{
		Current: current,
	}

	// Get parent workflow if exists (in parallel with history fetch)
	type parentResult struct {
		parent *Workflow
	}
	parentChan := make(chan parentResult, 1)

	go func() {
		if current.ParentID != nil && *current.ParentID != "" {
			query := fmt.Sprintf("WorkflowId = '%s'", *current.ParentID)
			parents, _, err := c.ListWorkflows(ctx, namespace, ListOptions{
				PageSize: 1,
				Query:    query,
			})
			if err == nil && len(parents) > 0 {
				parentChan <- parentResult{parent: &parents[0]}
				return
			}
		}
		parentChan <- parentResult{}
	}()

	// Get enhanced history for relationship extraction
	events, err := c.GetEnhancedWorkflowHistory(ctx, namespace, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow history: %w", err)
	}

	// Wait for parent lookup
	parentRes := <-parentChan
	result.Parent = parentRes.parent

	// Extract signals from events (single pass)
	for _, event := range events {
		if strings.Contains(event.Type, "SignalExternalWorkflowExecutionInitiated") {
			signal := WorkflowSignal{
				FromWorkflowID: workflowID,
				ToWorkflowID:   event.ChildWorkflowID,
				Time:           event.Time,
			}
			if strings.Contains(event.Details, "SignalName:") {
				parts := strings.Split(event.Details, "SignalName: ")
				if len(parts) > 1 {
					signalPart := strings.Split(parts[1], ",")[0]
					signal.SignalName = strings.TrimSpace(signalPart)
				}
			}
			result.OutgoingSignals = append(result.OutgoingSignals, signal)
		} else if strings.Contains(event.Type, "WorkflowExecutionSignaled") {
			signal := WorkflowSignal{
				ToWorkflowID: workflowID,
				Time:         event.Time,
			}
			if strings.Contains(event.Details, "SignalName:") {
				parts := strings.Split(event.Details, "SignalName: ")
				if len(parts) > 1 {
					signalPart := strings.Split(parts[1], ",")[0]
					signal.SignalName = strings.TrimSpace(signalPart)
				}
			}
			result.IncomingSignals = append(result.IncomingSignals, signal)
		}
	}

	// Get child workflows with depth control (reuse events, don't fetch again)
	if depth > 0 {
		children, err := c.getChildWorkflowsFromEvents(ctx, namespace, events)
		if err == nil {
			for _, child := range children {
				node := &WorkflowNode{
					Workflow: child,
					EdgeType: "child",
					Depth:    1,
				}

				// Recursively fetch children if depth allows
				if depth > 1 {
					childRel, err := c.GetWorkflowRelationships(ctx, namespace, child.ID, child.RunID, depth-1)
					if err == nil && childRel != nil {
						node.Children = childRel.Children
					}
				}

				result.Children = append(result.Children, node)
			}
		}
	}

	return result, nil
}

// Ensure Client implements Provider
var _ Provider = (*Client)(nil)
