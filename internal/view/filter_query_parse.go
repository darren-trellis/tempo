package view

import (
	"strings"
	"unicode"

	"github.com/galaxy-io/tempo/internal/config"
)

// filterClausesFromQuery turns a visibility query into builder clauses, falling
// back to a single raw clause when the query uses more than the form can show.
func filterClausesFromQuery(query string) []config.FilterClause {
	if clauses, ok := parseVisibilityQuery(query); ok {
		return clauses
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	return []config.FilterClause{{Key: filterOpRaw, Op: filterOpRaw, Value: query}}
}

// parseVisibilityQuery splits a query into clauses the form can edit and
// recompile without changing its meaning. Anything else (OR, grouping, IN,
// BETWEEN, >=) reports false so the caller keeps the query verbatim.
func parseVisibilityQuery(query string) ([]config.FilterClause, bool) {
	parts, ok := splitQueryConjuncts(query)
	if !ok {
		return nil, false
	}
	clauses := make([]config.FilterClause, 0, len(parts))
	for _, part := range parts {
		clause, ok := parseQueryConjunct(part)
		if !ok {
			return nil, false
		}
		clauses = append(clauses, clause)
	}
	if len(clauses) == 0 {
		return nil, false
	}
	return clauses, true
}

func splitQueryConjuncts(query string) ([]string, bool) {
	query = strings.TrimSpace(query)
	if query == "" || strings.ContainsAny(query, "()") {
		return nil, false
	}
	runes := []rune(query)
	var parts []string
	var current strings.Builder
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		word, width, ok := queryKeywordAt(runes, i)
		if ok {
			if word != "and" {
				return nil, false
			}
			parts = append(parts, current.String())
			current.Reset()
			i += width - 1
			continue
		}
		current.WriteRune(r)
	}
	if quote != 0 {
		return nil, false
	}
	parts = append(parts, current.String())

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		out = append(out, part)
	}
	return out, true
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
		const startsWith = "STARTS_WITH"
		if len(rest) > len(startsWith) && strings.EqualFold(rest[:len(startsWith)], startsWith) &&
			!isQueryIdentRune(rune(rest[len(startsWith)])) {
			op, rest = filterOpStartsWith, rest[len(startsWith):]
			break
		}
		return config.FilterClause{}, false
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
