package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// A small line-oriented markdown renderer for a post's text.
//
// Ported from Easyhog's internal/tui/markdown.go (commit 01336f8), itself ported
// from EasyLoom's (ca2f8f6). Changes from Easyhog's:
//
//   - Every line break is kept. The peers' `gather` folds a paragraph's following
//     lines into it, which in the real rooms joined a post's sign-off into the
//     paragraph above (architecture, "Rendering"). Each source line is its own
//     line here, wrapped to the width and indented as it was written.
//   - The `_` forms of strong and emphasis are gone (review note L5): `__x__` and
//     `_x_` fired inside identifiers such as read_new_ids and inside a mention such
//     as @ci_bot_2. `**` and `*` remain.
//   - `@name` is a span of its own, styled as `moot.sh read` highlights it
//     (moot.sh:643), and with colour off it stays the text it is.
//   - The styles come from this package's styles.go.
//
// It is deliberately not a markdown implementation. It covers what an agent's post
// is usually written in: headings, paragraphs, lists, fenced code, block quotes, and
// inline emphasis, code and [[id]] links. Tables, images and HTML stay as written.
//
// Two rules run through all of it:
//
//   - With colour off the output must still be readable markdown. A marker a
//     colour would have replaced is kept, not silently deleted: `# Heading`
//     without a bold to show for it is still a heading, and a bare `Heading`
//     is a lie about what is in the text.
//   - Wrapping happens over already-styled spans, never over rendered text, so
//     an escape sequence can never be split across two lines. A styled run
//     that crosses a wrap becomes two styled runs.

var (
	fenceRE   = regexp.MustCompile("^(\\s*)(`{3,}|~{3,})(.*)$")
	headingRE = regexp.MustCompile(`^(#{1,6})[ \t]+(.*)$`)
	bulletRE  = regexp.MustCompile(`^([ \t]*)([-*+]|\d+[.)])[ \t]+(.*)$`)
	quoteRE   = regexp.MustCompile(`^([ \t]*)>[ \t]?(.*)$`)
)

// isRule reports a thematic break: three or more of -, * or _, alone on a
// line. Go's regexp has no backreference, and a hand-written check is clearer
// than the alternation that would replace one.
func isRule(line string) bool {
	trimmed := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, line)
	if len(trimmed) < 3 {
		return false
	}
	switch trimmed[0] {
	case '-', '*', '_':
	default:
		return false
	}
	return strings.Count(trimmed, string(trimmed[0])) == len(trimmed)
}

// inlineRules are tried in order at every position, and the earliest match
// wins. Code comes first because a backtick span is a place where `*` means an
// asterisk.
var inlineRules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"code", regexp.MustCompile("`([^`]+)`")},
	{"wikilink", regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)},
	{"strong", regexp.MustCompile(`\*\*([^*]+)\*\*`)},
	{"emphasis", regexp.MustCompile(`\*([^*]+)\*`)},
	// A mention is moot.sh's own pattern (@ then letters, digits, dot, underscore or
	// dash) without the trailing sentence punctuation its mentions_in trims.
	{"mention", regexp.MustCompile(`(@[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)`)},
}

// span is a run of text and how to draw it. id names the style so adjacent
// runs of the same kind can be coalesced into one escape sequence rather than
// one per word.
type span struct {
	text  string
	style lipgloss.Style
	id    string
}

// codeIndent is the indent, in spaces, from which a plain line counts as
// indented code and keeps the pane's width.
const codeIndent = 4

// renderMarkdown turns a post's text into styled lines no wider than width.
// Prose (paragraphs, headings, list items, quotes) wraps at measure, or at width
// when that is narrower; fenced code, table rows and plain lines indented four or
// more spaces, which lose their meaning when cut short, keep the whole width. A
// token too long for its line breaks at the width, not the measure.
//
// It is a pure function: text and widths in, lines out. No model, no
// filesystem, no I/O. Every line break in text is a line break in the output.
func renderMarkdown(text string, width, measure int, st styles, colour bool) []string {
	if width < 1 {
		width = 1
	}
	prose := max(1, min(width, measure))
	// The trailing newline every well-formed file ends with is a terminator,
	// not a blank line, and drawing it as one wastes a row of a pane that has
	// a dozen.
	normal := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	src := strings.Split(strings.TrimRight(normal, "\n"), "\n")

	out := []string{}
	for i := 0; i < len(src); i++ {
		line := src[i]

		if fence := fenceRE.FindStringSubmatch(line); fence != nil {
			i = appendFence(&out, src, i, fence, width, st)
			continue
		}
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		if isRule(line) {
			out = append(out, st.mdRule.Render(strings.Repeat("─", min(prose, 12))))
			continue
		}
		if heading := headingRE.FindStringSubmatch(line); heading != nil {
			out = append(out, headingLines(heading, prose, width, st, colour)...)
			continue
		}
		if quote := quoteRE.FindStringSubmatch(line); quote != nil {
			out = append(out, quoteLines(quote, prose, width, st, colour)...)
			continue
		}
		if bullet := bulletRE.FindStringSubmatch(line); bullet != nil {
			out = append(out, bulletLines(bullet, bullet[3], prose, width, st, colour)...)
			continue
		}
		// A plain line keeps the indent it was written with. A table row (a line
		// opening with |) and a line indented four or more spaces (indented
		// code, a file tree) keep the pane's width, since a row or a tree cut
		// at the measure no longer reads as one.
		body := strings.TrimLeft(line, " ")
		w := prose
		if strings.HasPrefix(body, "|") || len(line)-len(body) >= codeIndent {
			w = width
		}
		indent := strings.Repeat(" ", min(len(line)-len(body), max(0, w-1)))
		out = append(out, wrap(tokens(inline(body, st, colour)), w, width, indent, indent, lipgloss.NewStyle())...)
	}
	return out
}

// appendFence emits a fenced code block and returns the index of its last
// line. An unterminated fence runs to the end of the document, which is what
// every other renderer does and what a truncated file looks like.
func appendFence(out *[]string, src []string, at int, fence []string, width int, st styles) int {
	marker := fence[2]
	*out = append(*out, st.mdCode.Render(clamp(src[at], width)))

	i := at + 1
	for ; i < len(src); i++ {
		line := src[i]
		if closing := fenceRE.FindStringSubmatch(line); closing != nil &&
			strings.TrimSpace(closing[3]) == "" && closing[2][0] == marker[0] &&
			len(closing[2]) >= len(marker) {
			*out = append(*out, st.mdCode.Render(clamp(line, width)))
			return i
		}
		// The contents are never restyled. A fence is a place where `*` means
		// an asterisk, so nothing in here is scanned for markers — the block
		// gets one style and the text inside it is left exactly as it is.
		for _, piece := range split(line, width) {
			*out = append(*out, st.mdCode.Render(piece))
		}
	}
	return i - 1
}

// headingLines draws a heading. The marker is dropped when a bold can stand in
// for it, and kept when it cannot.
func headingLines(heading []string, width, hard int, st styles, colour bool) []string {
	text := heading[2]
	prefix := ""
	if !colour {
		prefix = heading[1] + " "
	}
	spans := inline(text, st, colour)
	for i := range spans {
		spans[i].style = st.mdHeading.Inherit(spans[i].style)
		spans[i].id = "heading:" + spans[i].id
	}
	return wrap(tokens(spans), width, hard, prefix, strings.Repeat(" ", lipgloss.Width(prefix)), st.mdHeading)
}

// quoteLines draws a block quote: the marker dimmed, the contents rendered.
func quoteLines(quote []string, width, hard int, st styles, colour bool) []string {
	prefix := quote[1] + "> "
	cont := quote[1] + "  "
	if !colour {
		cont = prefix
	}
	return wrap(tokens(inline(quote[2], st, colour)), width, hard, prefix, cont, st.mdQuote)
}

// bulletLines draws a list item, keeping its marker and its indent.
func bulletLines(bullet []string, body string, width, hard int, st styles, colour bool) []string {
	indent := strings.ReplaceAll(bullet[1], "\t", "  ")
	prefix := indent + bullet[2] + " "
	cont := indent + strings.Repeat(" ", lipgloss.Width(bullet[2])+1)
	return wrap(tokens(inline(body, st, colour)), width, hard, prefix, cont, st.mdBullet)
}

// inline splits a line into styled spans.
//
// With colour off every marker is kept and every span is drawn plainly, so the
// output is the markdown that is in the file. With colour on the markers are
// replaced by the style they asked for.
func inline(text string, st styles, colour bool) []span {
	if text == "" {
		return nil
	}
	name, match := earliest(text)
	if match == nil {
		return []span{{text: text}}
	}

	inner := ""
	for i := 2; i+1 < len(match); i += 2 {
		if match[i] >= 0 {
			inner = text[match[i]:match[i+1]]
			break
		}
	}

	var style lipgloss.Style
	switch name {
	case "code":
		style = st.mdInlineCode
	case "wikilink":
		style = st.mdLink
	case "strong":
		style = st.mdStrong
	case "emphasis":
		style = st.mdEmphasis
	case "mention":
		style = st.mention
	}

	shown := inner
	if !colour {
		// The marker is what makes it readable without a colour to replace it.
		shown = text[match[0]:match[1]]
	}

	out := []span{}
	if match[0] > 0 {
		out = append(out, inline(text[:match[0]], st, colour)...)
	}
	if colour && (name == "strong" || name == "emphasis") {
		// The markers are gone, so what they wrapped is scanned in turn: a mention
		// or a `code` span inside bold keeps its own look, on top of the bold.
		for _, sp := range inline(inner, st, colour) {
			sp.style = style.Inherit(sp.style)
			sp.id = name + ":" + sp.id
			out = append(out, sp)
		}
	} else {
		out = append(out, span{text: shown, style: style, id: name})
	}
	// The remainder is rescanned rather than parsed: an unclosed marker after
	// this one is left as the text it is.
	return append(out, inline(text[match[1]:], st, colour)...)
}

// earliest returns the first inline match in the line, preferring the rule
// listed first when two start at the same place.
func earliest(text string) (string, []int) {
	best := []int(nil)
	name := ""
	for _, rule := range inlineRules {
		loc := rule.re.FindStringSubmatchIndex(text)
		if loc == nil {
			continue
		}
		if best == nil || loc[0] < best[0] {
			best, name = loc, rule.name
		}
	}
	return name, best
}

// token is a word or a run of spaces, carrying the style of the span it came
// from. Wrapping happens over these, so a line is always built from whole
// styled pieces and an escape sequence is never split.
type token struct {
	text  string
	style lipgloss.Style
	id    string
	space bool
}

func tokens(spans []span) []token {
	out := []token{}
	for _, s := range spans {
		for _, piece := range runs(s.text) {
			out = append(out, token{
				text:  piece,
				style: s.style,
				id:    s.id,
				space: strings.TrimSpace(piece) == "",
			})
		}
	}
	return out
}

// runs splits a string into alternating runs of spaces and non-spaces, so a
// break is only ever taken where there was already a space.
func runs(s string) []string {
	out := []string{}
	start := 0
	var inSpace bool
	for i, r := range s {
		space := r == ' ' || r == '\t'
		if i == 0 {
			inSpace = space
			continue
		}
		if space != inSpace {
			out = append(out, s[start:i])
			start, inSpace = i, space
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// wrap lays tokens out to width, with first before the first line and cont
// before every line after it. Prefixes are drawn in prefixStyle. A token too
// long for width is cut only at hard, the pane's width, which is never less.
//
// A line is built from whole styled runs: pieces sharing a style are gathered
// and rendered together, and nothing is ever rendered and then cut. That is
// what makes it impossible for a wrap to split an escape sequence in half.
func wrap(items []token, width, hard int, first, cont string, prefixStyle lipgloss.Style) []string {
	if width < 1 {
		width = 1
	}
	lines := []string{}

	var line, run strings.Builder
	var runStyle lipgloss.Style
	runID := ""
	inRun := false

	closeRun := func() {
		if !inRun {
			return
		}
		line.WriteString(runStyle.Render(run.String()))
		run.Reset()
		inRun = false
	}

	prefix := first
	room := width - lipgloss.Width(prefix)
	if room < 1 {
		room = 1
	}
	if prefix != "" {
		line.WriteString(prefixStyle.Render(prefix))
	}
	used, started, gap := 0, false, ""

	flush := func() {
		closeRun()
		lines = append(lines, line.String())
		line.Reset()
		prefix = cont
		room = width - lipgloss.Width(prefix)
		if room < 1 {
			room = 1
		}
		if prefix != "" {
			line.WriteString(prefixStyle.Render(prefix))
		}
		used, started, gap = 0, false, ""
	}

	for _, item := range items {
		if item.space {
			if started {
				gap = item.text
			}
			continue
		}
		// A token longer than the line goes on a line of its own and breaks
		// only at the pane's width, so a URL that fits the pane stays whole.
		for _, piece := range split(item.text, max(room, hard-lipgloss.Width(prefix))) {
			w := lipgloss.Width(piece)
			space := lipgloss.Width(gap)
			if started && used+space+w > room {
				flush()
				space = 0
			}
			if inRun && runID == item.id {
				// The same kind of text as what is already open: extend the
				// run, space and all, rather than closing and reopening it.
				if started && space > 0 {
					run.WriteString(gap)
					used += space
				}
				run.WriteString(piece)
			} else {
				closeRun()
				if started && space > 0 {
					line.WriteString(gap)
					used += space
				}
				runStyle, runID, inRun = item.style, item.id, true
				run.WriteString(piece)
			}
			used += w
			started, gap = true, ""
		}
	}
	closeRun()
	return append(lines, line.String())
}

// split breaks a word too long for the line into pieces that fit. It measures
// display width rather than bytes: posts contain box-drawing
// and other non-ASCII.
func split(word string, width int) []string {
	if width < 1 {
		width = 1
	}
	if lipgloss.Width(word) <= width {
		return []string{word}
	}
	out := []string{}
	var piece strings.Builder
	used := 0
	for _, r := range word {
		w := lipgloss.Width(string(r))
		if used+w > width && piece.Len() > 0 {
			out = append(out, piece.String())
			piece.Reset()
			used = 0
		}
		piece.WriteRune(r)
		used += w
	}
	if piece.Len() > 0 {
		out = append(out, piece.String())
	}
	return out
}

// clamp cuts a line to the width, by display cells.
func clamp(s string, width int) string {
	pieces := split(s, width)
	if len(pieces) == 0 {
		return ""
	}
	return pieces[0]
}
