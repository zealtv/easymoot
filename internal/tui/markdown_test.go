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
	return plain(renderMarkdown(text, width, newStyles(), colour))
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
	got := renderMarkdown(in, 80, newStyles(), false)
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
	for id, want := range map[string]string{
		"20260801-090000-ada":    "── 2026-08-01 19:00 · ada ──",
		"20261345-250000-grace":  "── 2026-13-45 25:00:00 UTC · grace ──",
		"README":                 "── README ──",
		"20260801-090000-a.b-2":  "── 2026-08-01 19:00 · a.b-2 ──",
		"20260801-0900-ada":      "── 20260801-0900-ada ──",
		"20260801-090000-e\x1bv": "── 2026-08-01 19:00 · e·v ──",
	} {
		if got := head(id); got != want {
			t.Errorf("heading(%q) = %q, want %q", id, got, want)
		}
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
		"── 2026-08-01 09:00 · ada ──\nfirst line\n— ada\n\n── 2026-08-01 09:01 · bob ──\nwith files\n\nattached: notes.txt\n\n── 2026-08-01 09:02 · eve ──\n(no post.md in posts/20260801-090200-eve)",
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
