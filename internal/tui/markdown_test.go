package tui

// Tests for the ported renderer (see markdown.go for what changed from Easyhog's):
// every line break kept, no `_` emphasis, a mention span, colour-off output. As in
// Easyhog's tests, `go test` has no colour profile, so what is asserted is the text
// that is kept and the spans and style definitions, not escape bytes.

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

var escapes = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ansi.Strip(line)
	}
	return out
}

func render(text string, width int, colour bool) []string {
	return plain(renderMarkdown(text, width, width, newStyles(), colour))
}

// A post's own line breaks stay: the sign-off under a paragraph is a line of its
// own, which Easyhog's renderer would have folded into the paragraph above.
func TestEveryLineBreakIsKept(t *testing.T) {
	in := "The nightly index rebuilt.\nIt is 40% smaller.\n— alice\n"
	want := []string{"The nightly index rebuilt.", "It is 40% smaller.", "— alice"}
	for _, colour := range []bool{false, true} {
		if got := render(in, 80, colour); !reflect.DeepEqual(got, want) {
			t.Errorf("colour %v: got %q, want %q", colour, got, want)
		}
	}
	// Blank lines are lines too, and a list item's following line is not folded in.
	if got, want := render("a\n\nb\n- item\nnext line\n", 80, false), []string{"a", "", "b", "- item", "next line"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAPlainLineKeepsItsIndentAndLongLinesWrapWithinThemselves(t *testing.T) {
	if got := render("    indented words\n", 80, false); got[0] != "    indented words" {
		t.Errorf("indent lost: %q", got)
	}
	got := render("one two three four five six\nnext\n", 12, false)
	if want := []string{"one two", "three four", "five six", "next"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, line := range got {
		if lipgloss.Width(line) > 12 {
			t.Errorf("line wider than the width: %q", line)
		}
	}
}

// Review note L5: the `_` forms of emphasis fired inside identifiers and mentions.
func TestUnderscoreFormsAreLiteral(t *testing.T) {
	in := "read_new_ids and __init__ and _x_ and @ci_bot_2 and snake_case_name"
	for _, colour := range []bool{false, true} {
		if got := render(in, 200, colour); len(got) != 1 || got[0] != in {
			t.Errorf("colour %v: got %q, want the text unchanged", colour, got)
		}
	}
	for _, sp := range inline(in, newStyles(), true) {
		if sp.id == "strong" || sp.id == "emphasis" {
			t.Errorf("span %q treated as %s", sp.text, sp.id)
		}
	}
	// `*` still emphasises.
	if got := render("an *asterisk* and **bold**", 80, true); got[0] != "an asterisk and bold" {
		t.Errorf("asterisk forms lost: %q", got)
	}
}

func TestMentionIsASpanOfItsOwn(t *testing.T) {
	st := newStyles()
	var got []string
	for _, sp := range inline("hi @ada, and @ci_bot_2. also @a-b- or mail@host", st, true) {
		if sp.id == "mention" {
			got = append(got, sp.text)
		}
	}
	// Trailing sentence punctuation stays out, as moot.sh's mentions_in trims it.
	if want := []string{"@ada", "@ci_bot_2", "@a-b", "@host"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mentions = %q, want %q", got, want)
	}
	if !st.mention.GetBold() || st.mention.GetForeground() == (lipgloss.NoColor{}) {
		t.Error("the mention style is plain with colour on")
	}
}

func TestMentionInsideBoldAndInsideCode(t *testing.T) {
	st := newStyles()
	var ids []string
	for _, sp := range inline("**@ada** and `@bob`", st, true) {
		ids = append(ids, sp.id)
	}
	if want := []string{"strong:mention", "", "code"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("span ids = %q, want %q", ids, want)
	}
}

// With colour off nothing is styled and every marker stays: a mention is the text
// it is, and `**bold**` is still readable as bold.
func TestColourOffKeepsEveryMarkerAndEmitsNoEscapes(t *testing.T) {
	in := "# Heading\n\n@ada said **this** and `that` and *more*, see [[note]]\n> quoted\n- item\n"
	got := renderMarkdown(in, 80, 80, newStyles(), false)
	for _, line := range got {
		if escapes.MatchString(line) {
			t.Errorf("escape in colour-off output: %q", line)
		}
	}
	want := []string{"# Heading", "", "@ada said **this** and `that` and *more*, see [[note]]", "> quoted", "- item"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkersGoWithColourOn(t *testing.T) {
	got := render("# Title\nthe **bold** `code` [[link]]\n", 80, true)
	if want := []string{"Title", "the bold code link"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFencedCodeIsLeftAlone(t *testing.T) {
	got := render("```go\nx := *p * q // **not bold** @ada\n```\nafter\n", 80, true)
	if got[1] != "x := *p * q // **not bold** @ada" || got[3] != "after" {
		t.Errorf("fence restyled: %q", got)
	}
}

func TestCleanerMakesControlsVisibleAndKeepsLineBreaks(t *testing.T) {
	got := clean("a\tb\x1b[31mred\x00\r\nnext line\rend")
	if want := "a    b·[31mred·\nnext·line·end"; got != want {
		t.Errorf("clean = %q, want %q", got, want)
	}
	if got := cleanLine("x\ny\x07"); got != "x y·" {
		t.Errorf("cleanLine = %q", got)
	}
}

func TestHeadingShowsLocalTimeSenderAndOddIds(t *testing.T) {
	m := New(&spySource{}, WithLocation(time.FixedZone("AEST", 10*3600)))
	head := func(id string) string {
		var p moot.Post
		for _, q := range listIDs(t, id) {
			p = q
		}
		return ansi.Strip(m.heading(p, 80))
	}
	// The sender at the left, the time at the right, the rule between: 76 columns
	// (the measure) in an 80-column pane.
	joined := func(sender, when string) string {
		left, right := "── "+sender+" ", " "+when+" ──"
		return left + strings.Repeat("─", measure-lipgloss.Width(left)-lipgloss.Width(right)) + right
	}
	for id, want := range map[string]string{
		"20260801-090000-ada":    joined("ada", "2026-08-01 19:00"),
		"20261345-250000-grace":  joined("grace", "2026-13-45 25:00:00 UTC"),
		"README":                 "── README ──",
		"20260801-090000-a.b-2":  joined("a.b-2", "2026-08-01 19:00"),
		"20260801-0900-ada":      "── 20260801-0900-ada ──",
		"20260801-090000-e\x1bv": joined("e·v", "2026-08-01 19:00"),
	} {
		if got := head(id); got != want {
			t.Errorf("heading(%q) = %q, want %q", id, got, want)
		}
	}
	// In a pane narrower than the measure the time sits at the pane's right edge,
	// and the rule between never vanishes.
	p := moot.Post{ID: "20260801-090000-ada", Stamp: true, Sender: "ada", Time: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)}
	if got := ansi.Strip(m.heading(p, 40)); lipgloss.Width(got) != 40 || !strings.HasSuffix(got, " 2026-08-01 19:00 ──") || !strings.HasPrefix(got, "── ada ─") {
		t.Errorf("heading at 40 = %q", got)
	}
	// A heading wider than the pane is cut, not wrapped.
	if got := m.heading(moot.Post{ID: "20260801-090000-" + strings.Repeat("x", 80), Stamp: true, Sender: strings.Repeat("x", 80)}, 30); lipgloss.Width(got) > 30 {
		t.Errorf("heading is %d wide", lipgloss.Width(got))
	}
}

// listIDs classifies an id the way internal/moot does, through a real listing.
func listIDs(t *testing.T, id string) []moot.Post {
	t.Helper()
	f := newFixture(t, "h")
	f.room("r")
	f.write(".moot/rooms/r/posts/"+id+".md", "x\n")
	posts, err := f.open().List("r")
	if err != nil {
		t.Fatal(err)
	}
	return posts
}

// A post as the poll reads it: its body under its heading, a sign-off on its own
// line, an attachment line, the missing-body line.
func TestTranscriptShowsBodiesAttachmentsAndMissingBodies(t *testing.T) {
	f := newFixture(t, "t")
	f.room("r")
	f.post("r", "20260801-090000-ada", "first line\n— ada\n")
	f.write(".moot/rooms/r/posts/20260801-090100-bob/post.md", "with files\n")
	f.write(".moot/rooms/r/posts/20260801-090100-bob/notes.txt", "n")
	f.mkdir(".moot/rooms/r/posts/20260801-090200-eve")
	m := shown(t, f.open())
	got := strings.Join(plain(m.lines), "\n")
	for _, want := range []string{
		"── ada ─",
		" 2026-08-01 09:00 ──\nfirst line\n— ada\n\n\n── bob ─",
		" 2026-08-01 09:01 ──\nwith files\n\nattached: notes.txt\n\n\n── eve ─",
		" 2026-08-01 09:02 ──\n(no post.md in posts/20260801-090200-eve)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript lacks\n%s\nin\n%s", want, got)
		}
	}
	if len(m.starts) != 3 || m.starts[0] != 0 {
		t.Errorf("starts = %v", m.starts)
	}
}

func TestNoColourEnvironmentTurnsColourOff(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colourOn() {
		t.Fatal("NO_COLOR left colour on")
	}
	m := shown(t, newSpy(synthetic(t)))
	if v := m.View(); escapes.MatchString(v) {
		t.Fatalf("escape in colour-off view: %q", v)
	}
}

// resized is the model at a given terminal size.
func resized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

const longParagraph = "The index rebuild now clears the cache on every pass, so the second run no longer sees stale entries; the lock file moved into the temporary directory so a crashed run cannot leave it behind."

func TestPostBodyWrapsToTheMeasureInAWidePane(t *testing.T) {
	f := newFixture(t, "t")
	f.room("r")
	f.post("r", "20260801-090000-ada", longParagraph+"\n— ada\n")
	m := resized(t, shown(t, f.open()), 200, 40)
	if m.postsWidth() <= measure {
		t.Fatalf("pane text width %d is not wider than the measure %d", m.postsWidth(), measure)
	}
	body := plain(m.lines)[1:]
	if len(body) < 3 {
		t.Fatalf("paragraph did not wrap: %q", body)
	}
	longest := 0
	for _, l := range body {
		longest = max(longest, lipgloss.Width(l))
	}
	if longest > measure || longest < measure-20 {
		t.Errorf("longest body line is %d columns, want just under %d: %q", longest, measure, body)
	}
}

func TestPostBodyWrapsToANarrowPane(t *testing.T) {
	f := newFixture(t, "t")
	f.room("r")
	f.post("r", "20260801-090000-ada", longParagraph+"\n")
	m := resized(t, shown(t, f.open()), 60, 40)
	if m.postsWidth() >= measure {
		t.Fatalf("pane text width %d is not narrower than the measure %d", m.postsWidth(), measure)
	}
	for _, l := range plain(m.lines)[1:] {
		if lipgloss.Width(l) > m.postsWidth() {
			t.Errorf("line of %d columns in a pane text width of %d: %q", lipgloss.Width(l), m.postsWidth(), l)
		}
	}
}

func TestPostsAreSeparatedByPostGapBlankLines(t *testing.T) {
	m := shown(t, synthetic(t).open())
	lines := plain(m.lines)
	for i := 1; i < len(m.starts); i++ {
		at := m.starts[i]
		for j := 1; j <= postGap; j++ {
			if lines[at-j] != "" {
				t.Errorf("line %d before post %d is %q, want blank", j, i, lines[at-j])
			}
		}
		if lines[at-postGap-1] == "" {
			t.Errorf("more than %d blank lines before post %d", postGap, i)
		}
		if m.blockLen(i-1) != at-m.starts[i-1] {
			t.Errorf("blockLen(%d) disagrees with starts", i-1)
		}
	}
}

func TestCodeAndTableRowsKeepThePaneWidthAboveTheMeasure(t *testing.T) {
	code := "func rebuild(ctx context.Context, root string, opts ...Option) (*Index, error) { return nil, nil }"
	row := "| cache | cleared on every pass of the index rebuild so nothing stale is left behind in the run |"
	for _, l := range []string{code, row} {
		if w := lipgloss.Width(l); w <= measure || w >= 120 {
			t.Fatalf("test line of %d columns should sit between the measure %d and 120", w, measure)
		}
	}
	text := "```\n" + code + "\n```\n\n" + row + "\n\n" + longParagraph + "\n"
	got := plain(renderMarkdown(text, 120, measure, newStyles(), false))
	want := map[string]bool{"```": true, "": true, code: true, row: true}
	prose := 0
	for _, l := range got {
		if want[l] {
			continue
		}
		prose++
		if lipgloss.Width(l) > measure {
			t.Errorf("prose line of %d columns, over the measure: %q", lipgloss.Width(l), l)
		}
	}
	if prose < 3 {
		t.Errorf("paragraph did not wrap to the measure: %q", got)
	}
	for _, line := range []string{code, row} {
		found := false
		for _, l := range got {
			found = found || l == line
		}
		if !found {
			t.Errorf("line was cut: %q\nin %q", line, got)
		}
	}
	// In a pane narrower than the line, code still wraps to the pane.
	for _, l := range plain(renderMarkdown("```\n"+code+"\n```\n", 60, measure, newStyles(), false)) {
		if lipgloss.Width(l) > 60 {
			t.Errorf("code line of %d columns in a 60-column pane", lipgloss.Width(l))
		}
	}
}

func TestALongTokenBreaksAtThePaneWidthNotTheMeasure(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("path/segment-", 7) + "end"
	if w := len(url); w <= measure || w >= 150 {
		t.Fatalf("test URL of %d columns should sit between the measure %d and 150", w, measure)
	}
	text := "See " + url + " for the details."
	got := plain(renderMarkdown(text, 150, measure, newStyles(), false))
	found := false
	for _, l := range got {
		found = found || strings.Contains(l, url)
	}
	if !found {
		t.Errorf("URL was cut in a 150-column pane: %q", got)
	}
	// In a pane narrower than the URL it breaks at the pane.
	narrow := plain(renderMarkdown(text, 60, measure, newStyles(), false))
	if strings.Join(narrow, "") == "" || len(narrow) < 3 {
		t.Fatalf("narrow render too short: %q", narrow)
	}
	for _, l := range narrow {
		if lipgloss.Width(l) > 60 {
			t.Errorf("line of %d columns in a 60-column pane: %q", lipgloss.Width(l), l)
		}
	}
	if joined := strings.ReplaceAll(strings.Join(narrow, ""), " ", ""); !strings.Contains(joined, strings.ReplaceAll(url, " ", "")) {
		t.Errorf("URL lost when cut: %q", narrow)
	}
}

func TestIndentedPlainLinesKeepThePaneWidth(t *testing.T) {
	line := "    " + strings.Repeat("src/internal/component/", 4) + "file.go  # the entry point of the program"
	if w := lipgloss.Width(line); w <= measure || w >= 150 {
		t.Fatalf("test line of %d columns should sit between the measure %d and 150", w, measure)
	}
	// Three spaces is still prose.
	prose := "   " + longParagraph
	text := line + "\n\n" + prose + "\n"
	got := plain(renderMarkdown(text, 150, measure, newStyles(), false))
	if got[0] != line {
		t.Errorf("indented line was wrapped in a 150-column pane: %q", got)
	}
	for _, l := range got[1:] {
		if lipgloss.Width(l) > measure {
			t.Errorf("3-space line not wrapped to the measure: %q", l)
		}
	}
	// In a narrow pane the indented line wraps to the pane.
	narrow := plain(renderMarkdown(line, 60, measure, newStyles(), false))
	if len(narrow) < 2 {
		t.Errorf("indented line not wrapped in a 60-column pane: %q", narrow)
	}
	for _, l := range narrow {
		if lipgloss.Width(l) > 60 {
			t.Errorf("line of %d columns in a 60-column pane: %q", lipgloss.Width(l), l)
		}
	}
}
