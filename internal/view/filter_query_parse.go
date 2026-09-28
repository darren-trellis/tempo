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
	case strings.HasPrefix(rest, ">="), strings.HasPrefix(rest, "<="), strings.HasPrefix(rest, "=="):
		return config.FilterClause{}, false
	case strings.HasPrefix(rest, "!="):
		op, rest = filterOpNeq, rest[2:]
	case strings.HasPrefix(rest, "="):
		op, rest = filterOpEq, rest[1:]
	case strings.HasPrefix(rest, ">"):
		op, rest = filterOpAfter, rest[1:]
	case strings.HasPrefix(rest, "<"):
		op, rest = filterOpBefore, rest[1:]
	default:
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
