package temporal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/galaxy-io/tempo/internal/config"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
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

// Ensure Client implements Provider
var _ Provider = (*Client)(nil)
