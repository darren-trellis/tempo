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

type jsonTreeRow struct {
	text     string
	value    string
	path     string
	foldable bool
}

type jsonTreeBuilder struct {
	rows   []jsonTreeRow
	folded map[string]bool
}

func formatJSONTree(s string) string {
	rows, ok := buildJSONTreeRows(s, nil)
	if !ok {
		return highlightFormattedJSONWorkflow(strings.TrimSpace(s))
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = row.text
	}
	return strings.Join(lines, "\n")
}

func buildJSONTreeRows(s string, folded map[string]bool) ([]jsonTreeRow, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	values, ok := decodeJSONValues(s)
	if !ok {
		return nil, false
	}
	b := &jsonTreeBuilder{folded: folded}
	if len(values) == 1 {
		b.appendRoot(values[0])
	} else {
		b.appendRoot(values)
	}
	return b.rows, true
}

func (b *jsonTreeBuilder) appendRoot(v any) {
	switch val := v.(type) {
	case map[string]any:
		keys := sortedJSONKeys(val)
		for i, key := range keys {
			b.appendEntry(key, val[key], nil, "", i+1 == len(keys), false)
		}
	case []any:
		folded := b.isFolded(nil)
		b.add(jsonTreeDim(jsonTreeFoldMarker(folded)+fmt.Sprintf("[%d]", len(val)))+jsonTreeEllipsis(folded), val, nil, true)
		if folded {
			return
		}
		for i, item := range val {
			b.appendEntry(strconv.Itoa(i), item, nil, "", i+1 == len(val), true)
		}
	default:
		b.add(jsonTreeScalar(v), v, nil, false)
	}
}

func (b *jsonTreeBuilder) appendEntry(key string, value any, parent []string, prefix string, isLast, branched bool) {
	branch := ""
	if branched {
		branch = jsonTreeBranch(isLast)
	}
	lead := jsonTreeDim(prefix + branch)
	childPrefix := prefix
	if branched {
		childPrefix = prefix + jsonTreeGuide(isLast)
	}
	path := append(append(make([]string, 0, len(parent)+1), parent...), key)

	switch val := value.(type) {
	case map[string]any:
		if len(val) == 0 {
			b.add(lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val), val, path, false)
			return
		}
		folded := b.isFolded(path)
		b.add(lead+jsonTreeDim(jsonTreeFoldMarker(folded))+jsonTreeKey(key)+jsonTreeEllipsis(folded), val, path, true)
		if folded {
			return
		}
		keys := sortedJSONKeys(val)
		for i, child := range keys {
			b.appendEntry(child, val[child], path, childPrefix, i+1 == len(keys), true)
		}
	case []any:
		if len(val) == 0 {
			b.add(lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val), val, path, false)
			return
		}
		folded := b.isFolded(path)
		b.add(lead+jsonTreeDim(jsonTreeFoldMarker(folded))+jsonTreeKey(key)+jsonTreeDim(fmt.Sprintf(" [%d]", len(val)))+jsonTreeEllipsis(folded), val, path, true)
		if folded {
			return
		}
		for i, item := range val {
			b.appendEntry(strconv.Itoa(i), item, path, childPrefix, i+1 == len(val), true)
		}
	default:
		b.add(lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(value), value, path, false)
	}
}

func (b *jsonTreeBuilder) isFolded(path []string) bool {
	return b.folded[jsonTreePathKey(path)]
}

func (b *jsonTreeBuilder) add(text string, value any, path []string, foldable bool) {
	b.rows = append(b.rows, jsonTreeRow{
		text:     text,
		value:    jsonTreeCopyValue(value),
		path:     jsonTreePathKey(path),
		foldable: foldable,
	})
}

// jsonTreeCopyValue is what y copies. A string is its text; the tree still
// draws that text in quotes.
func jsonTreeCopyValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

// jsonTreePathKey names a node by its ancestry so folds and the highlight
// survive a rebuild. The separator cannot appear in a JSON key.
func jsonTreePathKey(path []string) string {
	return strings.Join(path, "\x00")
}

func jsonTreeFoldMarker(folded bool) string {
	if folded {
		return workflowTreeCollapsed + " "
	}
	return workflowTreeExpanded + " "
}

func jsonTreeEllipsis(folded bool) string {
	if !folded {
		return ""
	}
	return jsonTreeDim(" …")
}

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
