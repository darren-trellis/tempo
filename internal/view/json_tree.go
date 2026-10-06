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
	// match marks a row whose key holds the filter.
	match bool
}

type jsonTreeBuilder struct {
	rows   []jsonTreeRow
	folded map[string]bool
	// query, lowercased, keeps only the keys that hold it, what is under
	// them, and the rows above them. matches counts those keys.
	query   string
	matches int
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
	rows, _, ok := buildFilteredJSONTreeRows(s, folded, "")
	return rows, ok
}

// buildFilteredJSONTreeRows builds the tree with only the keys that hold
// query, what is under them, and the rows that lead to them, and counts the
// keys that matched. An empty query keeps every row. ok is false when s is
// not JSON.
func buildFilteredJSONTreeRows(s string, folded map[string]bool, query string) (rows []jsonTreeRow, matches int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, 0, false
	}
	values, ok := decodeJSONValues(s)
	if !ok {
		return nil, 0, false
	}
	b := &jsonTreeBuilder{folded: folded, query: strings.ToLower(query)}
	if len(values) == 1 {
		b.appendRoot(values[0])
	} else {
		b.appendRoot(values)
	}
	if b.query != "" && b.matches == 0 {
		return nil, 0, true
	}
	return b.rows, b.matches, true
}

func (b *jsonTreeBuilder) appendRoot(v any) {
	filtering := b.query != ""
	switch val := v.(type) {
	case map[string]any:
		keys := b.shownKeys(val, filtering)
		for i, key := range keys {
			b.appendEntry(key, val[key], nil, "", i+1 == len(keys), false, filtering, true)
		}
	case []any:
		folded := b.isFolded(nil) && !filtering
		b.add(jsonTreeDim(jsonTreeFoldMarker(folded)+fmt.Sprintf("[%d]", len(val)))+jsonTreeEllipsis(folded), val, nil, !filtering)
		if folded {
			return
		}
		items := b.shownItems(val, filtering)
		for i, item := range items {
			b.appendEntry(strconv.Itoa(item), val[item], nil, "", i+1 == len(items), true, filtering, false)
		}
	default:
		if !filtering {
			b.add(jsonTreeScalar(v), v, nil, false)
		}
	}
}

// shownKeys lists an object's keys in order, keeping only the ones the filter
// shows when filtering.
func (b *jsonTreeBuilder) shownKeys(obj map[string]any, filtering bool) []string {
	keys := sortedJSONKeys(obj)
	if !filtering {
		return keys
	}
	shown := keys[:0:0]
	for _, key := range keys {
		if b.keyMatches(key) || jsonHasKeyHolding(obj[key], b.query) {
			shown = append(shown, key)
		}
	}
	return shown
}

// shownItems lists an array's indexes, keeping only the items the filter
// shows when filtering. An index is not a key, so it never matches itself.
func (b *jsonTreeBuilder) shownItems(items []any, filtering bool) []int {
	shown := make([]int, 0, len(items))
	for i, item := range items {
		if !filtering || jsonHasKeyHolding(item, b.query) {
			shown = append(shown, i)
		}
	}
	return shown
}

func (b *jsonTreeBuilder) keyMatches(key string) bool {
	return b.query != "" && strings.Contains(strings.ToLower(key), b.query)
}

// jsonHasKeyHolding reports whether any key at or below value holds query,
// which is lowercased.
func jsonHasKeyHolding(value any, query string) bool {
	switch val := value.(type) {
	case map[string]any:
		for key, child := range val {
			if strings.Contains(strings.ToLower(key), query) || jsonHasKeyHolding(child, query) {
				return true
			}
		}
	case []any:
		for _, child := range val {
			if jsonHasKeyHolding(child, query) {
				return true
			}
		}
	}
	return false
}

// appendEntry adds key's row and the rows under it. filtering is whether the
// filter still applies at this depth: below a matching key everything shows.
// keyed is whether key is an object key rather than an array index. A row
// shown only to lead to a match stays open and cannot fold.
func (b *jsonTreeBuilder) appendEntry(key string, value any, parent []string, prefix string, isLast, branched, filtering, keyed bool) {
	matched := filtering && keyed && b.keyMatches(key)
	if matched {
		b.matches++
	}
	childFiltering := filtering && !matched
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
	own := len(b.rows)
	defer func() {
		if matched && own < len(b.rows) {
			b.rows[own].match = true
		}
	}()

	switch val := value.(type) {
	case map[string]any:
		if len(val) == 0 {
			b.add(lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val), val, path, false)
			return
		}
		folded := b.isFolded(path) && !childFiltering
		b.add(lead+jsonTreeDim(jsonTreeFoldMarker(folded))+jsonTreeKey(key)+jsonTreeEllipsis(folded), val, path, !childFiltering)
		if folded {
			return
		}
		keys := b.shownKeys(val, childFiltering)
		for i, child := range keys {
			b.appendEntry(child, val[child], path, childPrefix, i+1 == len(keys), true, childFiltering, true)
		}
	case []any:
		if len(val) == 0 {
			b.add(lead+jsonTreeKey(key)+jsonTreeDim(": ")+jsonTreeScalar(val), val, path, false)
			return
		}
		folded := b.isFolded(path) && !childFiltering
		b.add(lead+jsonTreeDim(jsonTreeFoldMarker(folded))+jsonTreeKey(key)+jsonTreeDim(fmt.Sprintf(" [%d]", len(val)))+jsonTreeEllipsis(folded), val, path, !childFiltering)
		if folded {
			return
		}
		items := b.shownItems(val, childFiltering)
		for i, item := range items {
			b.appendEntry(strconv.Itoa(item), val[item], path, childPrefix, i+1 == len(items), true, childFiltering, false)
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
		if val == "" {
			return fmt.Sprintf("[%s]\"\"[-]", theme.TagFgDim())
		}
		return fmt.Sprintf("[%s]%s[-]", theme.TagFg(), escapeForTView(jsonTreeStringText(val)))
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

// jsonTreeStringText shows a string without quotes, escaping only the control
// characters that would break the row onto more lines.
func jsonTreeStringText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortedJSONKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
