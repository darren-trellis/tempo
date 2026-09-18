package temporal

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
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
	resp, err := cl.OperatorService().ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
		Namespace: namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list search attributes: %w", err)
	}
	attrs := make([]SearchAttribute, 0, len(resp.GetCustomAttributes()))
	for name, typ := range resp.GetCustomAttributes() {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		attrs = append(attrs, SearchAttribute{Name: name, Type: searchAttributeType(typ)})
	}
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Name < attrs[j].Name
	})
	return attrs, nil
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
