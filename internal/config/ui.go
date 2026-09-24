package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ResolveUIURL returns the Temporal Web UI base URL for this profile.
// An explicit ui_url wins; otherwise localhost and Temporal Cloud addresses
// are inferred from the frontend address.
func (c ConnectionConfig) ResolveUIURL() string {
	expanded := c.ExpandEnv()
	if u := strings.TrimRight(strings.TrimSpace(expanded.UIURL), "/"); u != "" {
		return u
	}
	return inferUIURL(expanded.Address)
}

func inferUIURL(address string) string {
	host := strings.TrimSpace(address)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch {
	case host == "localhost", host == "127.0.0.1", host == "::1":
		return "http://localhost:8080"
	case strings.HasSuffix(host, ".tmprl.cloud"),
		strings.HasSuffix(host, ".temporal.io"),
		host == "temporal.io":
		return "https://cloud.temporal.io"
	default:
		return ""
	}
}

// WorkflowUIURL builds the Temporal Web UI timeline page for a workflow run.
func WorkflowUIURL(base, namespace, workflowID, runID string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || namespace == "" || workflowID == "" {
		return ""
	}
	path := fmt.Sprintf("%s/namespaces/%s/workflows/%s",
		base, url.PathEscape(namespace), url.PathEscape(workflowID))
	if runID != "" {
		path += "/" + url.PathEscape(runID) + "/timeline"
	}
	return path
}
