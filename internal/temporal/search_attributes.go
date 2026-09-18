package temporal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

type SearchAttributeType int

const (
	SearchAttributeKeyword SearchAttributeType = iota
	SearchAttributeText
	SearchAttributeInt
	SearchAttributeDouble
	SearchAttributeBool
	SearchAttributeDatetime
	SearchAttributeKeywordList
)

type SearchAttribute struct {
	Name string
	Type SearchAttributeType
}

func (c *Client) ListCustomSearchAttributes(ctx context.Context, namespace string) ([]SearchAttribute, error) {
	cl, err := c.conn()
	if err != nil {
		return nil, err
	}
	var custom, system map[string]enums.IndexedValueType
	opResp, opErr := cl.OperatorService().ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
		Namespace: namespace,
	})
	if opErr == nil {
		custom = opResp.GetCustomAttributes()
		system = opResp.GetSystemAttributes()
	}
	var aliases map[string]string
	nsResp, nsErr := cl.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: namespace,
	})
	if nsErr == nil {
		aliases = nsResp.GetConfig().GetCustomSearchAttributeAliases()
	}
	var extra map[string]enums.IndexedValueType
	saResp, saErr := cl.WorkflowService().GetSearchAttributes(ctx, &workflowservice.GetSearchAttributesRequest{})
	if saErr == nil {
		extra = saResp.GetKeys()
	}
	attrs := customSearchAttributesFromMaps(custom, system, aliases, extra)
	if len(attrs) > 0 {
		return attrs, nil
	}

	// A namespace-scoped Temporal Cloud API key cannot read the registry: both
	// ListSearchAttributes and GetSearchAttributes answer "Request unauthorized"
	// and the alias map comes back empty. The attributes still ride along on
	// visibility records, so recover the names from the executions themselves.
	observed, obsErr := observedSearchAttributes(ctx, cl, namespace)
	if len(observed) > 0 {
		return observed, nil
	}
	if err := firstError(opErr, saErr, nsErr, obsErr); err != nil {
		return nil, fmt.Errorf("failed to list search attributes: %w", err)
	}
	return nil, nil
}

const observedSearchAttributePageSize = 50

// observedSearchAttributes collects custom search attribute names from a page of
// visibility records.
func observedSearchAttributes(ctx context.Context, cl client.Client, namespace string) ([]SearchAttribute, error) {
	resp, err := cl.WorkflowService().ListWorkflowExecutions(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: namespace,
		PageSize:  observedSearchAttributePageSize,
	})
	if err != nil {
		return nil, err
	}
	fields := make([]map[string]*commonpb.Payload, 0, len(resp.GetExecutions()))
	for _, exec := range resp.GetExecutions() {
		fields = append(fields, exec.GetSearchAttributes().GetIndexedFields())
	}
	return searchAttributesFromIndexedFields(fields...), nil
}

func searchAttributesFromIndexedFields(fields ...map[string]*commonpb.Payload) []SearchAttribute {
	byName := map[string]SearchAttribute{}
	for _, indexed := range fields {
		for name, payload := range indexed {
			name = strings.TrimSpace(name)
			if name == "" || isReservedSearchAttribute(name) {
				continue
			}
			if _, ok := byName[name]; ok {
				continue
			}
			byName[name] = SearchAttribute{
				Name: name,
				Type: searchAttributeTypeFromMetadata(string(payload.GetMetadata()["type"])),
			}
		}
	}
	return sortedSearchAttributes(byName)
}

func searchAttributeTypeFromMetadata(name string) SearchAttributeType {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "keywordlist":
		return SearchAttributeKeywordList
	case "text":
		return SearchAttributeText
	case "int":
		return SearchAttributeInt
	case "double":
		return SearchAttributeDouble
	case "bool":
		return SearchAttributeBool
	case "datetime":
		return SearchAttributeDatetime
	default:
		return SearchAttributeKeyword
	}
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func customSearchAttributesFromMaps(
	custom map[string]enums.IndexedValueType,
	system map[string]enums.IndexedValueType,
	aliases map[string]string,
	extra map[string]enums.IndexedValueType,
) []SearchAttribute {
	byName := map[string]SearchAttribute{}
	add := func(name string, typ SearchAttributeType) {
		name = strings.TrimSpace(name)
		if name == "" || isReservedSearchAttribute(name) {
			return
		}
		byName[name] = SearchAttribute{Name: name, Type: typ}
	}
	for name, typ := range custom {
		add(name, searchAttributeType(typ))
	}
	for left, right := range aliases {
		field, name := aliasFieldAndName(left, right)
		typ := searchAttributeTypeFromFieldName(field)
		if t, ok := system[field]; ok {
			typ = searchAttributeType(t)
		} else if t, ok := custom[field]; ok {
			typ = searchAttributeType(t)
		} else if t, ok := custom[name]; ok {
			typ = searchAttributeType(t)
		}
		delete(byName, field)
		add(name, typ)
	}
	for name, typ := range extra {
		if isReservedSearchAttribute(name) {
			continue
		}
		if _, ok := byName[name]; ok {
			continue
		}
		add(name, searchAttributeType(typ))
	}
	return sortedSearchAttributes(byName)
}

func sortedSearchAttributes(byName map[string]SearchAttribute) []SearchAttribute {
	out := make([]SearchAttribute, 0, len(byName))
	for _, attr := range byName {
		out = append(out, attr)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func aliasFieldAndName(a, b string) (field, name string) {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	aCloud, bCloud := isCloudPlaceholderName(a), isCloudPlaceholderName(b)
	switch {
	case aCloud && !bCloud:
		return a, b
	case bCloud && !aCloud:
		return b, a
	default:
		return a, b
	}
}

func isCloudPlaceholderName(name string) bool {
	f := strings.ToLower(strings.TrimSpace(name))
	prefixes := []string{"keywordlist", "keyword", "datetime", "double", "text", "bool", "int"}
	for _, p := range prefixes {
		if !strings.HasPrefix(f, p) {
			continue
		}
		rest := f[len(p):]
		if rest == "" {
			return false
		}
		for _, r := range rest {
			if !unicode.IsDigit(r) {
				return false
			}
		}
		return true
	}
	return false
}

func isReservedSearchAttribute(name string) bool {
	if isCloudPlaceholderName(name) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	// The server owns the whole Temporal prefix, so new system attributes stay
	// out of the custom list without needing to be enumerated here.
	if strings.HasPrefix(lower, "temporal") {
		return true
	}
	switch lower {
	case "workflowid", "runid", "workflowtype", "taskqueue", "executionstatus",
		"starttime", "closetime", "executiontime", "executionduration",
		"historylength", "historysizebytes", "statetransitioncount",
		"binarychecksums", "batchernamespace", "batcheruser",
		"buildids", "parentworkflowid", "parentrunid",
		"rootworkflowid", "rootrunid":
		return true
	default:
		return false
	}
}

func searchAttributeTypeFromFieldName(field string) SearchAttributeType {
	f := strings.ToLower(strings.TrimSpace(field))
	switch {
	case strings.HasPrefix(f, "keywordlist"):
		return SearchAttributeKeywordList
	case strings.HasPrefix(f, "keyword"):
		return SearchAttributeKeyword
	case strings.HasPrefix(f, "text"):
		return SearchAttributeText
	case strings.HasPrefix(f, "int"):
		return SearchAttributeInt
	case strings.HasPrefix(f, "double"):
		return SearchAttributeDouble
	case strings.HasPrefix(f, "bool"):
		return SearchAttributeBool
	case strings.HasPrefix(f, "datetime"):
		return SearchAttributeDatetime
	default:
		return SearchAttributeKeyword
	}
}

func searchAttributeType(t enums.IndexedValueType) SearchAttributeType {
	switch t {
	case enums.INDEXED_VALUE_TYPE_TEXT:
		return SearchAttributeText
	case enums.INDEXED_VALUE_TYPE_INT:
		return SearchAttributeInt
	case enums.INDEXED_VALUE_TYPE_DOUBLE:
		return SearchAttributeDouble
	case enums.INDEXED_VALUE_TYPE_BOOL:
		return SearchAttributeBool
	case enums.INDEXED_VALUE_TYPE_DATETIME:
		return SearchAttributeDatetime
	case enums.INDEXED_VALUE_TYPE_KEYWORD_LIST:
		return SearchAttributeKeywordList
	default:
		return SearchAttributeKeyword
	}
}
