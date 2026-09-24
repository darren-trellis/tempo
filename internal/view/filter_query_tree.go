package view

import (
	"strings"
	"unicode"

	"github.com/galaxy-io/tempo/internal/config"
)

const (
	filterGroupAnd = "AND"
	filterGroupOr  = "OR"
)

// filterNode is a visibility query as a tree: a leaf holds one clause, a
// group joins its children with op. Groups never directly contain a group of
// the same op, and never have fewer than two children.
type filterNode struct {
	op       string
	children []*filterNode
	clause   config.FilterClause
	// source is the query text the node was parsed from, without enclosing
	// parentheses. Nodes built by edits have none.
	source string
}

func (n *filterNode) isLeaf() bool {
	return n.op == ""
}

// parseFilterTree reads a query into a tree. Pieces the clause form cannot
// represent stay as raw leaves, and a query that does not parse at all
// becomes a single raw leaf, so the tree always compiles back to an
// equivalent query.
func parseFilterTree(query string) *filterNode {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	p := &filterQueryParser{runes: []rune(query)}
	if node, ok := p.parseOr(); ok {
		p.skipSpace()
		if p.pos == len(p.runes) {
			return node
		}
	}
	return rawFilterLeaf(query)
}

func rawFilterLeaf(text string) *filterNode {
	return &filterNode{clause: config.FilterClause{Key: filterOpRaw, Op: filterOpRaw, Value: text}, source: text}
}

func newFilterGroup(op string, children []*filterNode) *filterNode {
	var flat []*filterNode
	for _, child := range children {
		if child == nil {
			continue
		}
		if child.op == op {
			flat = append(flat, child.children...)
			continue
		}
		flat = append(flat, child)
	}
	switch len(flat) {
	case 0:
		return nil
	case 1:
		return flat[0]
	}
	return &filterNode{op: op, children: flat}
}

// query compiles the tree back to a visibility query. Parsed nodes reuse
// their source text so untouched parts of a query keep their exact form.
func (n *filterNode) query(wl *WorkflowList) string {
	if n == nil {
		return ""
	}
	if n.source != "" {
		return n.source
	}
	if n.isLeaf() {
		return compileFilterClauseFor(wl, n.clause)
	}
	parts := make([]string, 0, len(n.children))
	for _, child := range n.children {
		part := child.query(wl)
		if part == "" {
			continue
		}
		if !child.isLeaf() {
			part = "(" + part + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " "+n.op+" ")
}

// replaced returns a copy of the tree with the node at path swapped for
// with, or removed when with is nil. Groups left with one child collapse
// into it, and a replacement with the same op as its new parent merges in.
func (n *filterNode) replaced(path []int, with *filterNode) *filterNode {
	if len(path) == 0 {
		return with
	}
	if n == nil || n.isLeaf() || path[0] < 0 || path[0] >= len(n.children) {
		return n
	}
	children := make([]*filterNode, 0, len(n.children))
	for i, child := range n.children {
		if i == path[0] {
			child = child.replaced(path[1:], with)
		}
		children = append(children, child)
	}
	return newFilterGroup(n.op, children)
}

type filterQueryParser struct {
	runes []rune
	pos   int
}

func (p *filterQueryParser) parseOr() (*filterNode, bool) {
	return p.parseJoined(filterGroupOr, p.parseAnd)
}

func (p *filterQueryParser) parseAnd() (*filterNode, bool) {
	return p.parseJoined(filterGroupAnd, p.parsePrimary)
}

func (p *filterQueryParser) parseJoined(op string, operand func() (*filterNode, bool)) (*filterNode, bool) {
	p.skipSpace()
	start := p.pos
	first, ok := operand()
	if !ok {
		return nil, false
	}
	children := []*filterNode{first}
	for {
		p.skipSpace()
		if !queryWordAt(p.runes, p.pos, op) {
			break
		}
		p.pos += len(op)
		next, ok := operand()
		if !ok {
			return nil, false
		}
		children = append(children, next)
	}
	node := newFilterGroup(op, children)
	if len(children) > 1 {
		node.source = strings.TrimSpace(string(p.runes[start:p.pos]))
	}
	return node, true
}

func (p *filterQueryParser) parsePrimary() (*filterNode, bool) {
	p.skipSpace()
	if p.pos < len(p.runes) && p.runes[p.pos] == '(' {
		p.pos++
		node, ok := p.parseOr()
		if !ok {
			return nil, false
		}
		p.skipSpace()
		if p.pos >= len(p.runes) || p.runes[p.pos] != ')' {
			return nil, false
		}
		p.pos++
		return node, true
	}
	return p.parseLeaf()
}

// parseLeaf reads one comparison, stopping at a top-level AND, OR or closing
// parenthesis. The AND inside BETWEEN x AND y and the parentheses of IN (...)
// belong to the comparison.
func (p *filterQueryParser) parseLeaf() (*filterNode, bool) {
	start := p.pos
	depth := 0
	inBetween := false
	var quote rune
	for p.pos < len(p.runes) {
		r := p.runes[p.pos]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			p.pos++
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
		case r == '(':
			depth++
		case r == ')':
			if depth == 0 {
				return p.leaf(start)
			}
			depth--
		case depth == 0 && queryWordAt(p.runes, p.pos, "between"):
			inBetween = true
			p.pos += len("between")
			continue
		case depth == 0 && queryWordAt(p.runes, p.pos, filterGroupAnd):
			if !inBetween {
				return p.leaf(start)
			}
			inBetween = false
			p.pos += len(filterGroupAnd)
			continue
		case depth == 0 && queryWordAt(p.runes, p.pos, filterGroupOr):
			return p.leaf(start)
		}
		p.pos++
	}
	if quote != 0 || depth != 0 {
		return nil, false
	}
	return p.leaf(start)
}

func (p *filterQueryParser) leaf(start int) (*filterNode, bool) {
	text := strings.TrimSpace(string(p.runes[start:p.pos]))
	if text == "" {
		return nil, false
	}
	if clause, ok := parseQueryConjunct(text); ok {
		return &filterNode{clause: clause, source: text}, true
	}
	return rawFilterLeaf(text), true
}

func (p *filterQueryParser) skipSpace() {
	for p.pos < len(p.runes) && unicode.IsSpace(p.runes[p.pos]) {
		p.pos++
	}
}

// queryWordAt reports whether word, in any case, starts at i as a whole
// word, so an attribute named Android is not read as AND.
func queryWordAt(runes []rune, i int, word string) bool {
	if i > 0 && isQueryIdentRune(runes[i-1]) {
		return false
	}
	end := i + len(word)
	if end > len(runes) || !strings.EqualFold(string(runes[i:end]), word) {
		return false
	}
	return end == len(runes) || !isQueryIdentRune(runes[end])
}
