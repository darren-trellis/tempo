package view

import "github.com/galaxy-io/tempo/internal/temporal"

const workflowMaxPages = 4

type workflowPage struct {
	token string
	next  string
	items []temporal.Workflow
}

type workflowPager struct {
	query     string
	pages     []workflowPage
	firstPage int
	tokens    []string
	ended     bool
}

func (p *workflowPager) reset(query string) {
	*p = workflowPager{query: query, tokens: []string{""}}
}

func (p *workflowPager) items() []temporal.Workflow {
	n := 0
	for _, page := range p.pages {
		n += len(page.items)
	}
	out := make([]temporal.Workflow, 0, n)
	for _, page := range p.pages {
		out = append(out, page.items...)
	}
	return out
}

func (p *workflowPager) hasNext() bool {
	if len(p.pages) == 0 {
		return true
	}
	last := p.pages[len(p.pages)-1]
	return last.next != "" || (!p.ended && p.nextPageIndex() < len(p.tokens))
}

func (p *workflowPager) hasPrev() bool {
	return p.firstPage > 0
}

func (p *workflowPager) nextPageIndex() int {
	return p.firstPage + len(p.pages)
}

func (p *workflowPager) nextToken() string {
	if len(p.pages) == 0 {
		return ""
	}
	last := p.pages[len(p.pages)-1]
	if last.next != "" {
		return last.next
	}
	idx := p.nextPageIndex()
	if idx < len(p.tokens) {
		return p.tokens[idx]
	}
	return ""
}

func (p *workflowPager) prevToken() (string, bool) {
	if !p.hasPrev() {
		return "", false
	}
	idx := p.firstPage - 1
	if idx >= len(p.tokens) {
		return "", false
	}
	return p.tokens[idx], true
}

func (p *workflowPager) accept(pageIndex int, token, next string, items []temporal.Workflow) {
	p.rememberToken(pageIndex, token)
	if next != "" {
		p.rememberToken(pageIndex+1, next)
	} else {
		p.ended = true
	}
	page := workflowPage{token: token, next: next, items: items}
	switch {
	case len(p.pages) == 0:
		p.pages = []workflowPage{page}
		p.firstPage = pageIndex
	case pageIndex == p.nextPageIndex():
		p.pages = append(p.pages, page)
		p.trim(true)
	case pageIndex == p.firstPage-1:
		p.pages = append([]workflowPage{page}, p.pages...)
		p.firstPage = pageIndex
		p.trim(false)
	case pageIndex >= p.firstPage && pageIndex < p.nextPageIndex():
		p.pages[pageIndex-p.firstPage] = page
	default:
		p.pages = []workflowPage{page}
		p.firstPage = pageIndex
	}
}

func (p *workflowPager) rememberToken(pageIndex int, token string) {
	for len(p.tokens) <= pageIndex {
		p.tokens = append(p.tokens, "")
	}
	p.tokens[pageIndex] = token
}

func (p *workflowPager) trim(fromFront bool) {
	for len(p.pages) > workflowMaxPages {
		if fromFront {
			p.pages = p.pages[1:]
			p.firstPage++
			continue
		}
		p.pages = p.pages[:len(p.pages)-1]
	}
}

func (p *workflowPager) loadedPageIndexes() []int {
	out := make([]int, len(p.pages))
	for i := range p.pages {
		out[i] = p.firstPage + i
	}
	return out
}
