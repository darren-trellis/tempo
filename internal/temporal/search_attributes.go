package temporal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
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
	attrs := customSearchAttributesFromMaps(custom, system, aliases)
	if len(attrs) == 0 && opErr != nil && nsErr != nil {
		return nil, fmt.Errorf("failed to list search attributes: %w", opErr)
	}
	return attrs, nil
}

func customSearchAttributesFromMaps(
	custom map[string]enums.IndexedValueType,
	system map[string]enums.IndexedValueType,
	aliases map[string]string,
) []SearchAttribute {
	byName := map[string]SearchAttribute{}
	add := func(name string, typ SearchAttributeType) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		byName[name] = SearchAttribute{Name: name, Type: typ}
	}
	for name, typ := range custom {
		add(name, searchAttributeType(typ))
	}
	for field, alias := range aliases {
		field = strings.TrimSpace(field)
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		typ := searchAttributeTypeFromFieldName(field)
		if t, ok := system[field]; ok {
			typ = searchAttributeType(t)
		} else if t, ok := custom[field]; ok {
			typ = searchAttributeType(t)
		} else if t, ok := custom[alias]; ok {
			typ = searchAttributeType(t)
		}
		delete(byName, field)
		add(alias, typ)
	}
	out := make([]SearchAttribute, 0, len(byName))
	for _, attr := range byName {
		out = append(out, attr)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
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
