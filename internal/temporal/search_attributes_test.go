package temporal

import (
	"testing"

	"go.temporal.io/api/enums/v1"
)

func TestCustomSearchAttributesPreferCloudAliases(t *testing.T) {
	got := customSearchAttributesFromMaps(
		nil,
		map[string]enums.IndexedValueType{
			"Keyword01": enums.INDEXED_VALUE_TYPE_KEYWORD,
			"Int01":     enums.INDEXED_VALUE_TYPE_INT,
		},
		map[string]string{
			"Keyword01": "CustomerId",
			"Int01":     "Amount",
		},
		nil,
	)
	if len(got) != 2 || got[0].Name != "Amount" || got[0].Type != SearchAttributeInt {
		t.Fatalf("aliases=%+v", got)
	}
	if got[1].Name != "CustomerId" || got[1].Type != SearchAttributeKeyword {
		t.Fatalf("CustomerId=%+v", got[1])
	}
}

func TestCustomSearchAttributesReplaceUnderlyingFields(t *testing.T) {
	got := customSearchAttributesFromMaps(
		map[string]enums.IndexedValueType{
			"Keyword01": enums.INDEXED_VALUE_TYPE_KEYWORD,
		},
		nil,
		map[string]string{"Keyword01": "CustomerId"},
		nil,
	)
	if len(got) != 1 || got[0].Name != "CustomerId" {
		t.Fatalf("should expose the alias, got %+v", got)
	}
}

func TestCustomSearchAttributesKeepOperatorNames(t *testing.T) {
	got := customSearchAttributesFromMaps(
		map[string]enums.IndexedValueType{
			"CustomerId": enums.INDEXED_VALUE_TYPE_KEYWORD,
		},
		nil,
		nil,
		nil,
	)
	if len(got) != 1 || got[0].Name != "CustomerId" {
		t.Fatalf("operator custom=%+v", got)
	}
}

func TestCustomSearchAttributesAcceptInvertedAliases(t *testing.T) {
	got := customSearchAttributesFromMaps(
		nil,
		nil,
		map[string]string{"CustomerId": "Keyword01"},
		nil,
	)
	if len(got) != 1 || got[0].Name != "CustomerId" {
		t.Fatalf("inverted alias=%+v", got)
	}
}

func TestCustomSearchAttributesMergeGetSearchAttributes(t *testing.T) {
	got := customSearchAttributesFromMaps(
		nil,
		nil,
		nil,
		map[string]enums.IndexedValueType{
			"CustomerId":  enums.INDEXED_VALUE_TYPE_KEYWORD,
			"WorkflowId":  enums.INDEXED_VALUE_TYPE_KEYWORD,
			"Keyword01":   enums.INDEXED_VALUE_TYPE_KEYWORD,
			"TransformId": enums.INDEXED_VALUE_TYPE_KEYWORD,
		},
	)
	if len(got) != 2 || got[0].Name != "CustomerId" || got[1].Name != "TransformId" {
		t.Fatalf("extra keys=%+v", got)
	}
}

func TestSearchAttributeTypeFromFieldName(t *testing.T) {
	cases := map[string]SearchAttributeType{
		"Keyword01":     SearchAttributeKeyword,
		"KeywordList03": SearchAttributeKeywordList,
		"Text02":        SearchAttributeText,
		"Int01":         SearchAttributeInt,
		"Double04":      SearchAttributeDouble,
		"Bool01":        SearchAttributeBool,
		"Datetime01":    SearchAttributeDatetime,
	}
	for field, want := range cases {
		if got := searchAttributeTypeFromFieldName(field); got != want {
			t.Fatalf("%s type=%v want=%v", field, got, want)
		}
	}
}
