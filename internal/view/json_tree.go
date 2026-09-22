package view

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
)

const jsonTreeTabWidth = 4

func formatJSONTree(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	values, ok := decodeJSONValues(s)
	if !ok {
		return highlightFormattedJSONWorkflow(s)
	}
	var lines []string
	if len(values) == 1 {
		appendJSONTreeRoot(&lines, values[0])
	} else {
		appendJSONTreeRoot(&lines, values)
	}
	return strings.Join(lines, "\n")
}

func appendJSONTreeRoot(lines *[]string, v any) {
	switch val := v.(type) {
	case map[string]any:
		keys := sortedJSONKeys(val)
		for i, key := range keys {
			appendJSONTreeEntry(lines, key, val[key], "", i+1 == len(keys), false)
		}
	case []any:
		*lines = append(*lines, jsonTreeDim(jsonTreeFoldMarker()+fmt.Sprintf("[%d]", len(val))))
		for i, item := range val {
			appendJSONTreeEntry(lines, strconv.Itoa(i), item, "", i+1 == len(val), true)
		}
	default:
		*lines = append(*lines, jsonTreeScalar(v))
	}
}

func appendJSONTreeEntry(lines *[]string, key string, value any, prefix string, isLast, branched bool) {
	branch := ""
	if branched {
		branch = jsonTreeBranch(isLast)
	}
	lead := jsonTreeDim(prefix + branch)
	childPrefix := prefix
	if branched {
		childPrefix = prefix + jsonTreeGuide(isLast)
	}

	switch val := value.(type) {
	case map[string]any:
		if len(val) == 0 {
			*lines = append(*lines, lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val))
			return
		}
		*lines = append(*lines, lead+jsonTreeDim(jsonTreeFoldMarker())+jsonTreeKey(key))
		keys := sortedJSONKeys(val)
		for i, child := range keys {
			appendJSONTreeEntry(lines, child, val[child], childPrefix, i+1 == len(keys), true)
		}
	case []any:
		if len(val) == 0 {
			*lines = append(*lines, lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val))
			return
		}
		*lines = append(*lines, lead+jsonTreeDim(jsonTreeFoldMarker())+jsonTreeKey(key)+jsonTreeDim(fmt.Sprintf(" [%d]", len(val))))
		for i, item := range val {
			appendJSONTreeEntry(lines, strconv.Itoa(i), item, childPrefix, i+1 == len(val), true)
		}
	default:
		*lines = append(*lines, lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(value))
	}
}

func jsonTreeFoldMarker() string { return "▾ " }

func jsonTreeBranch(isLast bool) string {
	if isLast {
		return "└" + strings.Repeat("─", jsonTreeTabWidth-2) + " "
	}
	return "├" + strings.Repeat("─", jsonTreeTabWidth-2) + " "
}

func jsonTreeGuide(isLast bool) string {
	if isLast {
		return strings.Repeat(" ", jsonTreeTabWidth)
	}
	return "│" + strings.Repeat(" ", jsonTreeTabWidth-1)
}

func jsonTreeKey(key string) string {
	return fmt.Sprintf("[%s]%s[-]", theme.TagAccent(), escapeForTView(key))
}

func jsonTreeDim(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), escapeForTView(s))
}

func jsonTreeScalar(v any) string {
	switch val := v.(type) {
	case nil:
		return fmt.Sprintf("[%s]null[-]", theme.TagFgDim())
	case bool:
		if val {
			return fmt.Sprintf("[%s]true[-]", temporal.StatusCompleted.ColorTag())
		}
		return fmt.Sprintf("[%s]false[-]", temporal.StatusFailed.ColorTag())
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), strconv.FormatInt(int64(val), 10))
		}
		return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), strconv.FormatFloat(val, 'f', -1, 64))
	case json.Number:
		return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), val.String())
	case string:
		encoded, err := json.Marshal(val)
		if err != nil {
			encoded = []byte(strconv.Quote(val))
		}
		return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(string(encoded)))
	case map[string]any:
		return fmt.Sprintf("[%s]{}[-]", theme.TagFgDim())
	case []any:
		return fmt.Sprintf("[%s]%s[-]", theme.TagFgDim(), escapeForTView("[]"))
	default:
		encoded, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(fmt.Sprint(val)))
		}
		return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(string(encoded)))
	}
}

func sortedJSONKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
