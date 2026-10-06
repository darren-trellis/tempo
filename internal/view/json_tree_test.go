package view

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/config"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func stripTreeTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '[':
			inTag = true
		case r == ']' && inTag:
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestFormatJSONTreeExpandsNestedObject(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"annotations":{"url":"http://x","tags":["a","b"]}}`))
	if !strings.Contains(got, "◇ annotations") {
		t.Fatalf("root object key should fold, got %q", got)
	}
	if !strings.Contains(got, "url: http://x") {
		t.Fatalf("nested string should render, got %q", got)
	}
	if !strings.Contains(got, "◇ tags") {
		t.Fatalf("arrays should show a count, got %q", got)
	}
	if !strings.Contains(got, "0: a") || !strings.Contains(got, "1: b") {
		t.Fatalf("array items should be indexed, got %q", got)
	}
	if !strings.Contains(got, "├── ") || !strings.Contains(got, "└── ") {
		t.Fatalf("tree should use box-drawing branches, got %q", got)
	}
}

func TestFormatJSONTreeShowsStringsWithoutQuotes(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"label":"say \"hi\"","path":"C:\\tmp","multi":"a\nb","empty":""}`))
	for _, want := range []string{`label: say "hi"`, `path: C:\tmp`, `multi: a\nb`, `empty: ""`} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in %q", want, got)
		}
	}
}

func TestFormatJSONTreeEmptyContainers(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"empty_obj":{},"empty_arr":[],"nested":{"a":{},"b":[]}}`))
	if !strings.Contains(got, "empty_obj: {}") {
		t.Fatalf("empty object, got %q", got)
	}
	if !strings.Contains(got, "empty_arr: ［]") {
		t.Fatalf("empty array, got %q", got)
	}
	if !strings.Contains(got, "a: {}") || !strings.Contains(got, "b: ［]") {
		t.Fatalf("nested empty containers, got %q", got)
	}
}

func TestFormatJSONTreeCommaSeparatedPayloads(t *testing.T) {
	got := stripTreeTags(formatJSONTree(`{"order":1}, false`))
	if !strings.Contains(got, "◇") || !strings.Contains(got, "2") {
		t.Fatalf("multi-arg payloads should be a root array, got %q", got)
	}
	if !strings.Contains(got, "order: 1") {
		t.Fatalf("object payload should expand, got %q", got)
	}
	if !strings.Contains(got, "1: false") {
		t.Fatalf("boolean payload should stay, got %q", got)
	}
}

func TestFormatJSONTreeLeavesPlainText(t *testing.T) {
	raw := "not json at all"
	if got := formatJSONTree(raw); !strings.Contains(stripTreeTags(got), raw) {
		t.Fatalf("plain text should survive, got %q", got)
	}
}

func TestFormatIOContentUsesTreeWhenAsked(t *testing.T) {
	pretty := formatIOContent("Input", `{"a":1}`, false)
	if strings.Contains(stripTreeTags(pretty), "◇") {
		t.Fatalf("pretty mode should not draw a tree, got %q", pretty)
	}
	tree := formatIOContent("Input", `{"a":1}`, true)
	if !strings.Contains(stripTreeTags(tree), "a: 1") {
		t.Fatalf("tree mode should flatten the object, got %q", tree)
	}
}

func TestJSONTreeRowsCarryTheirJSONValues(t *testing.T) {
	rows, ok := buildJSONTreeRows(`{"object":{"name":"tempo"},"scalar":7,"label":"say \"hi\""}`, nil)
	if !ok {
		t.Fatal("expected valid tree")
	}
	var object, scalar, label string
	for _, row := range rows {
		plain := stripTreeTags(row.text)
		switch {
		case strings.Contains(plain, "object"):
			object = row.value
		case strings.Contains(plain, "scalar"):
			scalar = row.value
		case strings.Contains(plain, "label"):
			label = row.value
		}
	}
	if object != "{\n  \"name\": \"tempo\"\n}" {
		t.Fatalf("object row value=%q", object)
	}
	if scalar != "7" {
		t.Fatalf("scalar row value=%q", scalar)
	}
	if label != `say "hi"` {
		t.Fatalf("string row value=%q", label)
	}
}

func TestJSONTreeSelectionHighlightsAndMovesRows(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"a":1,"b":{"nested":true}}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	if value, ok := selection.value(); !ok || value != "1" {
		t.Fatalf("first row value=%q ok=%v", value, ok)
	}
	if !selection.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) {
		t.Fatal("down should move the tree highlight")
	}
	if value, ok := selection.value(); !ok || !strings.Contains(value, `"nested": true`) {
		t.Fatalf("object row value=%q ok=%v", value, ok)
	}
}

// The highlight is painted into the text because tview's region highlighting
// only inverts styled runes, which striped the row in several colors and
// stopped at the end of the text.
func TestJSONTreeHighlightCoversTheWholeRow(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 40, 10)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"a":1,"bbbbbbbb":2}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	lines := strings.Split(view.GetText(false), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%q", lines)
	}
	selected, other := lines[0], lines[1]
	open := "[" + theme.ColorToHex(accentTextColor()) + ":" + theme.TagAccent() + ":b]"
	if !strings.HasPrefix(selected, open) {
		t.Fatalf("selected row should carry the selection style, got %q", selected)
	}
	if rest := strings.ReplaceAll(strings.ReplaceAll(selected, open, ""), "[-:-:-]", ""); strings.Contains(rest, "[") {
		t.Fatalf("selected row should be one uniform style, got %q", selected)
	}
	if width := tview.TaggedStringWidth(selected); width != 40 {
		t.Fatalf("selected row should span the pane, width=%d", width)
	}
	if strings.Contains(other, theme.TagAccent()+":b]") {
		t.Fatalf("unselected row should stay unstyled, got %q", other)
	}
}

func TestJSONTreeSpaceFoldsAndUnfoldsTheSelectedNode(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 40, 10)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"outer":{"inner":1},"tail":2}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	if got := len(selection.rows); got != 3 {
		t.Fatalf("expected outer, inner and tail rows, got %d", got)
	}

	space := tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)
	if !selection.handleKey(space) {
		t.Fatal("space should fold the selected node")
	}
	if got := len(selection.rows); got != 2 {
		t.Fatalf("folding should hide the child row, got %d rows", got)
	}
	plain := stripTreeTags(view.GetText(false))
	if !strings.Contains(plain, workflowTreeCollapsed) || strings.Contains(plain, "inner") {
		t.Fatalf("folded node should collapse, got %q", plain)
	}

	if !selection.handleKey(space) {
		t.Fatal("space should unfold the selected node")
	}
	if got := len(selection.rows); got != 3 {
		t.Fatalf("unfolding should restore the child row, got %d rows", got)
	}
	if row, ok := selection.selectedRow(); !ok || row.path != "outer" {
		t.Fatalf("the cursor should stay on the folded node, got %+v", row)
	}
}

// A row padded out to the widest line spilled its blank tail onto extra lines
// in a wrapped pane, which looked like the folded row had grown.
func TestJSONTreeHighlightStopsAtThePaneEdge(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true).SetWordWrap(true)
	view.SetRect(0, 0, 20, 8)
	selection := newJSONTreeSelection(view)
	long := strings.Repeat("x", 120)
	if !selection.setContent(`{"a":{"b":1},"z":"`+long+`"}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	selection.handleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	selected := strings.Split(view.GetText(false), "\n")[0]
	bar := selected[:strings.LastIndex(selected, "[-:-:-]")]
	if width := tview.TaggedStringWidth(stripStyleTags(bar)); width != 20 {
		t.Fatalf("the highlight should stop at the pane edge, width=%d", width)
	}
}

// A wrapped value is still one row: every visual line is highlighted, and moving
// down lands on the next value rather than the continuation.
func TestJSONTreeWrapHighlightsEveryVisualLine(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	setTextViewWrap(view, true)
	view.SetRect(0, 0, 24, 8)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"name":"abcdefghijklmnopqrstuvwxyz","tail":1}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	start, span := selection.visualSpan(0)
	if start != 0 || span < 2 {
		t.Fatalf("the long value should wrap onto several lines, start=%d span=%d", start, span)
	}
	lines := strings.Split(view.GetText(false), "\n")
	open := "[" + theme.ColorToHex(accentTextColor()) + ":" + theme.TagAccent() + ":b]"
	for i := 0; i < span; i++ {
		if !strings.HasPrefix(lines[i], open) {
			t.Fatalf("visual line %d should be highlighted, got %q", i, lines[i])
		}
		if width := tview.TaggedStringWidth(lines[i]); width != 24 {
			t.Fatalf("visual line %d should fill the pane, width=%d", i, width)
		}
	}
	if strings.HasPrefix(lines[span], open) {
		t.Fatalf("the next value should not be highlighted, got %q", lines[span])
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(24, 8)
	view.Draw(screen)
	_, _, width, _ := view.GetInnerRect()
	_, firstBG, _ := func() (tcell.Color, tcell.Color, tcell.AttrMask) {
		_, _, style, _ := screen.GetContent(0, 0)
		return style.Decompose()
	}()
	if firstBG == theme.Bg() {
		t.Fatal("the selected row should not use the pane background")
	}
	for row := 0; row < span && row < 8; row++ {
		for col := 0; col < width; col++ {
			_, _, style, _ := screen.GetContent(col, row)
			_, bg, _ := style.Decompose()
			if bg != firstBG {
				t.Fatalf("row %d col %d background=%v, want the highlight %v", row, col, bg, firstBG)
			}
		}
	}
	if _, horiz := textViewScrollMetrics(view, width, 8); horiz.overflow() {
		t.Fatalf("wrap should retire the horizontal scrollbar, horiz=%+v", horiz)
	}

	if !selection.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) {
		t.Fatal("down should leave the wrapped value")
	}
	if value, ok := selection.value(); !ok || value != "1" {
		t.Fatalf("down should select the next value, got %q ok=%v", value, ok)
	}
}

func TestJSONTreeWrapOffKeepsOneLine(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	setTextViewWrap(view, false)
	view.SetRect(0, 0, 24, 6)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"name":"abcdefghijklmnopqrstuvwxyz"}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	if _, span := selection.visualSpan(0); span != 1 {
		t.Fatalf("wrap off should keep the value on one line, span=%d", span)
	}
	_, _, width, height := view.GetInnerRect()
	if _, horiz := textViewScrollMetrics(view, width, height); !horiz.overflow() {
		t.Fatalf("wrap off should keep the horizontal scrollbar, horiz=%+v", horiz)
	}
}

func TestJSONTreeJumpsToFirstAndLastRow(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 30, 3)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"a":1,"b":2,"c":3,"d":4,"e":5}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	if !selection.handleKey(tcell.NewEventKey(tcell.KeyRune, 'G', tcell.ModNone)) {
		t.Fatal("G should stay inside the tree")
	}
	if selection.selected != 4 {
		t.Fatalf("G should land on the last row, got %d", selection.selected)
	}
	if offset, _ := view.GetScrollOffset(); offset != 2 {
		t.Fatalf("G should scroll the last row into view, offset=%d", offset)
	}
	if !selection.handleKey(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone)) {
		t.Fatal("g should stay inside the tree")
	}
	if selection.selected != 0 {
		t.Fatalf("g should land on the first row, got %d", selection.selected)
	}
	if offset, _ := view.GetScrollOffset(); offset != 0 {
		t.Fatalf("g should scroll back to the top, offset=%d", offset)
	}
}

// A scalar has nothing to fold, so space must leave the tree alone rather than
// rebuilding it around a path that cannot collapse.
func TestJSONTreeSpaceOnAScalarDoesNothing(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"a":1,"b":2}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	before := view.GetText(false)
	if !selection.handleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)) {
		t.Fatal("space should stay inside the tree")
	}
	if got := view.GetText(false); got != before {
		t.Fatalf("a scalar row should not change when folded, got %q", got)
	}
}

// Preview panes re-render on every refresh, which must not throw away folds or
// move the cursor back to the top.
func TestJSONTreeKeepsFoldsWhenTheSameContentIsRendered(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 40, 10)
	selection := newJSONTreeSelection(view)
	content := `{"outer":{"inner":1},"tail":2}`
	selection.setContent(content, true)
	selection.handleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	selection.setContent(content, true)
	if got := len(selection.rows); got != 2 {
		t.Fatalf("a refresh should keep the fold, got %d rows", got)
	}

	selection.setContent(`{"other":3}`, true)
	if len(selection.folded) != 0 || selection.selected != 0 {
		t.Fatalf("new content should start fresh, folded=%v selected=%d", selection.folded, selection.selected)
	}
}

func TestJSONTreeSelectionIsInactiveForPlainText(t *testing.T) {
	view := tview.NewTextView()
	selection := newJSONTreeSelection(view)
	if selection.setContent("plain text", true) {
		t.Fatal("plain text should not become selectable tree rows")
	}
	if selection.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) {
		t.Fatal("inactive tree should leave navigation to the text view")
	}
}

func TestSelectingASentenceBesideAWideRowDrawsQuickly(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"blob":    strings.Repeat("x", 40000),
		"message": "Done.",
		"success": true,
	})
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	view.SetWrap(false)
	view.SetRect(0, 0, 80, 20)
	tree := newJSONTreeSelection(view)
	if !tree.setContent(string(payload), true) {
		t.Fatal("payload should render as a tree")
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(80, 20)

	tree.selectRow(1)
	start := time.Now()
	view.Draw(screen)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("padding a row that ends a sentence out to a 40k-wide row took %v", elapsed)
	}
}

// The highlight shows only while the pane has focus. The row stays selected
// underneath, so focus coming back puts it where it was.
func TestJSONTreeHidesTheHighlightWithoutFocus(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(`{"a":1,"b":2}`, true) {
		t.Fatal("expected selectable JSON tree")
	}
	selection.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	bar := "[" + theme.ColorToHex(accentTextColor()) + ":" + theme.TagAccent() + ":b]"
	if !strings.Contains(view.GetText(false), bar) {
		t.Fatal("a focused tree should draw the highlight")
	}

	selection.setFocused(false)
	if strings.Contains(view.GetText(false), bar) {
		t.Fatalf("an unfocused tree should draw no highlight, text=%q", view.GetText(false))
	}
	if value, ok := selection.value(); !ok || value != "2" {
		t.Fatalf("the selection should stay put, value=%q", value)
	}

	selection.setFocused(true)
	if !strings.Contains(view.GetText(false), bar) {
		t.Fatal("focus coming back should bring the highlight back")
	}
}

func TestIOTreesFollowTheIOPaneFocus(t *testing.T) {
	wl := NewWorkflowList(&App{}, "default")
	wl.focusPane = focusWorkflows
	wl.applyFocusStyles()
	if !wl.workflowIOTree.unfocused || !wl.eventDetailTree.unfocused {
		t.Fatal("the I/O trees should hide their highlight while another pane has focus")
	}
	wl.focusPane = focusEventDetail
	wl.applyFocusStyles()
	if wl.workflowIOTree.unfocused || wl.eventDetailTree.unfocused {
		t.Fatal("the I/O trees should highlight while their pane has focus")
	}
}

func jsonTreeRowPaths(rows []jsonTreeRow) string {
	paths := make([]string, len(rows))
	for i, row := range rows {
		paths[i] = strings.ReplaceAll(row.path, "\x00", ".")
	}
	return strings.Join(paths, ",")
}

const jsonTreeFilterPayload = `{"customer":{"ID":1,"name":"x"},"order":{"items":[{"sku":"a","id":2},{"sku":"b"}]},"total":5}`

// A filter keeps the keys that hold it, case aside, and the rows that lead to
// them. A sibling that holds no match is dropped, and an array index is not a
// key, so it never matches by itself.
func TestJSONTreeFilterKeepsMatchingKeysAndTheirAncestors(t *testing.T) {
	rows, matches, ok := buildFilteredJSONTreeRows(jsonTreeFilterPayload, nil, "Id")
	if !ok {
		t.Fatal("expected JSON")
	}
	if got := jsonTreeRowPaths(rows); got != "customer,customer.ID,order,order.items,order.items.0,order.items.0.id" {
		t.Fatalf("rows=%s", got)
	}
	if matches != 2 {
		t.Fatalf("matches=%d", matches)
	}
	for _, row := range rows {
		want := strings.HasSuffix(strings.ToLower(row.path), "\x00id")
		if row.match != want {
			t.Fatalf("row %q match=%v", row.path, row.match)
		}
		if row.foldable && !row.match {
			t.Fatalf("a row shown only to lead to a match should not fold, %q", row.path)
		}
	}

	if _, matches, _ := buildFilteredJSONTreeRows(jsonTreeFilterPayload, nil, "1"); matches != 0 {
		t.Fatalf("an array index should not match, matches=%d", matches)
	}
}

// Everything under a matching key shows, so its value can be read.
func TestJSONTreeFilterShowsWhatIsUnderAMatch(t *testing.T) {
	rows, matches, _ := buildFilteredJSONTreeRows(jsonTreeFilterPayload, nil, "customer")
	if got := jsonTreeRowPaths(rows); got != "customer,customer.ID,customer.name" || matches != 1 {
		t.Fatalf("rows=%s matches=%d", got, matches)
	}
}

// A fold the reader made does not hide a match: the rows that lead to it stay
// open while the filter is on, and the fold is back once it is cleared.
func TestJSONTreeFilterOpensFoldedAncestors(t *testing.T) {
	folded := map[string]bool{"customer": true}
	rows, _, _ := buildFilteredJSONTreeRows(jsonTreeFilterPayload, folded, "name")
	if got := jsonTreeRowPaths(rows); got != "customer,customer.name" {
		t.Fatalf("rows=%s", got)
	}
	rows, _, _ = buildFilteredJSONTreeRows(jsonTreeFilterPayload, folded, "")
	if got := jsonTreeRowPaths(rows); got != "customer,order,order.items,order.items.0,order.items.0.id,order.items.0.sku,order.items.1,order.items.1.sku,total" {
		t.Fatalf("rows=%s", got)
	}
}

func TestJSONTreeSelectionFiltersAndSelectsTheFirstMatch(t *testing.T) {
	view := tview.NewTextView().SetDynamicColors(true)
	selection := newJSONTreeSelection(view)
	if !selection.setContent(jsonTreeFilterPayload, true) {
		t.Fatal("expected selectable JSON tree")
	}

	selection.setFilter("sku")
	if row, _ := selection.selectedRow(); row.path != "order\x00items\x000\x00sku" {
		t.Fatalf("the highlight should land on the first match, got %q", row.path)
	}
	if !selection.filtering() || selection.matches != 2 || len(selection.rows) != 6 {
		t.Fatalf("matches=%d rows=%s", selection.matches, jsonTreeRowPaths(selection.rows))
	}

	selection.setFilter("nothing")
	if !selection.active() || len(selection.rows) != 1 || !strings.Contains(view.GetText(true), "No keys match nothing") {
		t.Fatalf("no match should say so, text=%q", view.GetText(true))
	}
	if selection.matches != 0 {
		t.Fatalf("matches=%d", selection.matches)
	}

	selection.setFilter("")
	if selection.filtering() || len(selection.rows) != 11 {
		t.Fatalf("clearing the filter should show everything, rows=%s", jsonTreeRowPaths(selection.rows))
	}
}

// The pane's / search filters the tree in io_tree mode and only scrolls in
// plain text.
func TestIOSearchFiltersOnlyTheTree(t *testing.T) {
	on := true
	wl := NewWorkflowList(&App{config: &config.Config{IOTree: &on}}, "default")
	wl.previewKind = previewDetails
	wl.workflowIOKind = workflowIOInput
	wl.workflowIOTree.setContent(jsonTreeFilterPayload, true)

	wl.applyFocusedIOSearch("id")
	if !wl.workflowIOTree.filtering() || wl.workflowIOTree.matches != 2 {
		t.Fatalf("the tree should narrow to the matching keys, rows=%s", jsonTreeRowPaths(wl.workflowIOTree.rows))
	}

	wl.applyFocusedIOSearch("")
	wl.workflowIOTree.setContent("", false)
	wl.workflowIOView.SetText(formatIOContent("Input", jsonTreeFilterPayload, false))
	before := wl.workflowIOView.GetText(false)
	wl.applyFocusedIOSearch("id")
	if wl.workflowIOTree.filtering() || wl.workflowIOView.GetText(false) != before {
		t.Fatal("plain text should not be filtered")
	}
}
