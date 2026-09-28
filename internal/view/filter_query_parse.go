package view

import (
	"strings"
	"unicode"

	"github.com/galaxy-io/tempo/internal/config"
)

// filterClausesFromQuery turns a visibility query into the builder's rows,
// which are joined with AND. Each top-level AND operand becomes a row;
// operands the form cannot show, OR groups included, stay as raw rows of their
// original text, and a query that is an OR at the top is one raw row.
func filterClausesFromQuery(query string) []config.FilterClause {
	root := parseFilterTree(query)
	switch {
	case root == nil:
		return nil
	case root.isLeaf():
		return []config.FilterClause{root.clause}
	case root.op != filterGroupAnd:
		return []config.FilterClause{rawFilterLeaf(strings.TrimSpace(query)).clause}
	}
	clauses := make([]config.FilterClause, 0, len(root.children))
	for _, child := range root.children {
		if child.isLeaf() {
			clauses = append(clauses, child.clause)
			continue
		}
		clauses = append(clauses, rawFilterLeaf("("+child.source+")").clause)
	}
	return clauses
}

// queryKeywordAt reports a boolean connective starting at i, so that an
// attribute named Android or a value of 'and' is not mistaken for one.
func queryKeywordAt(runes []rune, i int) (word string, width int, ok bool) {
	if i > 0 && isQueryIdentRune(runes[i-1]) {
		return "", 0, false
	}
	for _, candidate := range []string{"and", "or"} {
		width = len(candidate)
		if i+width > len(runes) {
			continue
		}
		if !strings.EqualFold(string(runes[i:i+width]), candidate) {
			continue
		}
		if i+width < len(runes) && isQueryIdentRune(runes[i+width]) {
			continue
		}
		return candidate, width, true
	}
	return "", 0, false
}

func parseQueryConjunct(part string) (config.FilterClause, bool) {
	runes := []rune(strings.TrimSpace(part))
	if len(runes) == 0 || !isQueryIdentStart(runes[0]) {
		return config.FilterClause{}, false
	}
	end := 0
	for end < len(runes) && isQueryIdentRune(runes[end]) {
		end++
	}
	key := string(runes[:end])
	rest := strings.TrimSpace(string(runes[end:]))

	if op, ok := nullaryQueryOp(rest); ok {
		return config.FilterClause{Key: key, Op: op}, true
	}

	var op string
	switch {
	case strings.HasPrefix(rest, "=="):
		return config.FilterClause{}, false
	case strings.HasPrefix(rest, "!="):
		op, rest = filterOpNeq, rest[2:]
	case strings.HasPrefix(rest, ">="):
		op, rest = filterOpOnOrAfter, rest[2:]
	case strings.HasPrefix(rest, "<="):
		op, rest = filterOpOnOrBefore, rest[2:]
	case strings.HasPrefix(rest, "="):
		op, rest = filterOpEq, rest[1:]
	case strings.HasPrefix(rest, ">"):
		op, rest = filterOpAfter, rest[1:]
	case strings.HasPrefix(rest, "<"):
		op, rest = filterOpBefore, rest[1:]
	default:
		if matched, value, ok := parseSetOrRangeOp(rest); ok {
			return config.FilterClause{Key: key, Op: matched, Value: value}, true
		}
		// NOT STARTS_WITH has to win over STARTS_WITH, or the shorter match
		// would leave "NOT" behind as part of the value.
		for _, candidate := range []struct {
			text string
			op   string
		}{
			{"NOT STARTS_WITH", filterOpNotStartsWith},
			{"STARTS_WITH", filterOpStartsWith},
		} {
			if len(rest) <= len(candidate.text) || !strings.EqualFold(rest[:len(candidate.text)], candidate.text) {
				continue
			}
			if isQueryIdentRune(rune(rest[len(candidate.text)])) {
				continue
			}
			op, rest = candidate.op, rest[len(candidate.text):]
			break
		}
		if op == "" {
			return config.FilterClause{}, false
		}
	}

	value, ok := parseQueryValue(strings.TrimSpace(rest))
	if !ok {
		return config.FilterClause{}, false
	}
	return config.FilterClause{Key: key, Op: op, Value: value}, true
}

// nullaryQueryOp matches the operators that take no value. IS NOT NULL is
// tested first so it is not read as IS NULL with trailing junk.
func nullaryQueryOp(rest string) (string, bool) {
	for _, candidate := range []struct {
		text string
		op   string
	}{
		{"IS NOT NULL", filterOpIsNotNull},
		{"IS NULL", filterOpIsNull},
	} {
		if len(rest) != len(candidate.text) {
			continue
		}
		if strings.EqualFold(rest, candidate.text) {
			return candidate.op, true
		}
	}
	return "", false
}

// parseSetOrRangeOp matches IN, NOT IN, and BETWEEN, whose values are a list
// or a pair rather than the single value parseQueryValue reads.
func parseSetOrRangeOp(rest string) (op, value string, ok bool) {
	for _, candidate := range []struct {
		text string
		op   string
	}{
		{"NOT IN", filterOpNotIn},
		{"IN", filterOpIn},
		{"BETWEEN", filterOpBetween},
	} {
		if len(rest) <= len(candidate.text) || !strings.EqualFold(rest[:len(candidate.text)], candidate.text) {
			continue
		}
		if isQueryIdentRune(rune(rest[len(candidate.text)])) {
			continue
		}
		body := strings.TrimSpace(rest[len(candidate.text):])
		switch candidate.op {
		case filterOpBetween:
			low, high, found := splitBetweenBounds(body)
			if !found {
				return "", "", false
			}
			return candidate.op, joinFilterValues([]string{low, high}), true
		default:
			items, found := parseQueryList(body)
			if !found {
				return "", "", false
			}
			return candidate.op, joinFilterValues(items), true
		}
	}
	return "", "", false
}

// splitBetweenBounds divides "x AND y" on the AND that joins the bounds, not
// on one that happens to sit inside a quoted value.
func splitBetweenBounds(body string) (low, high string, ok bool) {
	runes := []rune(body)
	inQuote := rune(0)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = r
			continue
		}
		word, width, found := queryKeywordAt(runes, i)
		if !found || word != "and" {
			continue
		}
		left, leftOK := parseQueryValue(strings.TrimSpace(string(runes[:i])))
		right, rightOK := parseQueryValue(strings.TrimSpace(string(runes[i+width:])))
		if leftOK && rightOK {
			return left, right, true
		}
	}
	return "", "", false
}

// parseQueryList reads the parenthesised values of an IN clause.
func parseQueryList(body string) ([]string, bool) {
	runes := []rune(strings.TrimSpace(body))
	if len(runes) < 2 || runes[0] != '(' || runes[len(runes)-1] != ')' {
		return nil, false
	}
	inner := strings.TrimSpace(string(runes[1 : len(runes)-1]))
	if inner == "" {
		return nil, false
	}
	var items []string
	var current []rune
	inQuote := rune(0)
	flush := func() bool {
		value, ok := parseQueryValue(strings.TrimSpace(string(current)))
		current = nil
		if !ok {
			return false
		}
		items = append(items, value)
		return true
	}
	for _, r := range []rune(inner) {
		if inQuote != 0 {
			current = append(current, r)
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = r
			current = append(current, r)
			continue
		}
		if r == ',' {
			if !flush() {
				return nil, false
			}
			continue
		}
		current = append(current, r)
	}
	if inQuote != 0 || !flush() || len(items) == 0 {
		return nil, false
	}
	return items, true
}

func parseQueryValue(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if quote := rune(raw[0]); quote == '\'' || quote == '"' {
		runes := []rune(raw)
		if len(runes) < 2 || runes[len(runes)-1] != quote {
			return "", false
		}
		inner := string(runes[1 : len(runes)-1])
		doubled := strings.Repeat(string(quote), 2)
		for _, piece := range strings.Split(inner, doubled) {
			if strings.ContainsRune(piece, quote) {
				return "", false
			}
		}
		return strings.ReplaceAll(inner, doubled, string(quote)), true
	}
	for _, r := range raw {
		if unicode.IsSpace(r) {
			return "", false
		}
	}
	return raw, true
}

func isQueryIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isQueryIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
