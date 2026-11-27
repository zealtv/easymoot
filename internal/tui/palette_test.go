package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPaletteOffersPostAndQuit(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	var names []string
	for _, c := range m.commands() {
		if (c.name == "Post" || c.name == "Quit") && !c.strict {
			t.Errorf("%s must match a prefix only", c.name)
		}
		names = append(names, c.name)
	}
	if got := strings.Join(names, ","); got != "Post,Create room,Archive room,Refresh,Quit" {
		t.Fatalf("commands = %s", got)
	}
}

func TestPaletteOpensEmptyAndEnterAloneDoesNothing(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	m, _ = feed(m, typeText(paletteKey)...)
	if m.palette == nil || m.palette.chosen != "" {
		t.Fatalf("palette = %#v", m.palette)
	}
	next, cmd := m.Update(key(tea.KeyEnter))
	if cmd != nil || next.(Model).palette == nil {
		t.Fatal("enter on an untouched palette acted")
	}
	next, _ = m.Update(key(tea.KeyEsc))
	if next.(Model).palette != nil {
		t.Fatal("esc did not close the palette")
	}
}

func TestQuitOnlyThroughThePalette(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	m, _ = feed(m, typeText(">qui")...)
	if m.palette == nil || m.palette.chosen != actionQuit {
		t.Fatalf("typing a prefix should choose Quit: %#v", m.palette)
	}
	if _, cmd := m.Update(key(tea.KeyEnter)); !quits(cmd) {
		t.Fatal("enter on Quit did not quit")
	}
	// Arrow keys choose it too.
	m, _ = feed(shown(t, newSpy(synthetic(t))), typeText(paletteKey)...)
	for i := 0; i < len(allCommands) && m.palette.chosen != actionQuit; i++ {
		m, _ = feed(m, key(tea.KeyDown))
	}
	if _, cmd := m.Update(key(tea.KeyEnter)); !quits(cmd) {
		t.Fatal("down to Quit, enter did not quit")
	}
}

// Quit matches only a prefix of its name: no aliases, no scattered letters.
func TestQuitMatchesOnlyThePrefix(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	for _, q := range []string{"qt", "uit", "exit", "close", "x", "quitt", "quit now"} {
		p, _ := feed(m, typeText(paletteKey+q)...)
		if p.palette.chosen != "" || len(p.filtered()) != 0 {
			t.Errorf("%q found Quit", q)
		}
		if _, cmd := p.Update(key(tea.KeyEnter)); cmd != nil {
			t.Errorf("%q then enter did something", q)
		}
	}
	for _, q := range []string{"q", "qu", "QUIT", "q u i t"} {
		if p, _ := feed(m, typeText(paletteKey+q)...); len(p.filtered()) != 1 {
			t.Errorf("%q did not find Quit", q)
		}
	}
}

// What a sentence typed into the pane can reach: it opens the palette at its first
// ">" and types the rest, so only text that ends, after a ">", in a prefix of
// "quit" quits (Easyhog's `q: quit`, written down in the architecture, with the
// palette key ">" since 2026-11-27). A colon is plain text now.
func TestSentencesReachQuitOnlyByThePaletteKeyThenItsName(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	for _, s := range []string{"hello quit please\n", "quit\n", "q\n", "Note to self > remember the milk\n", "ok > exit\n", "a > later quit\n", "q: quit\n"} {
		var cmd tea.Cmd
		p := m
		for _, msg := range typeText(s) {
			p, cmd = feed(p, msg)
			if quits(cmd) {
				t.Fatalf("%q quit", s)
			}
		}
		if _, cmd := p.Update(key(tea.KeyEnter)); quits(cmd) {
			t.Fatalf("%q then enter quit", s)
		}
	}
	p, _ := feed(m, typeText("q > quit")...)
	if _, cmd := p.Update(key(tea.KeyEnter)); !quits(cmd) {
		t.Fatal("`q > quit` then Enter should reach Quit: the palette cannot refuse it")
	}
}

func TestPasteIntoThePaneOrThePaletteIsIgnoredOrInert(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">quit"), Paste: true}
	if next, cmd := m.Update(paste); cmd != nil || next.(Model).palette != nil {
		t.Fatal("a paste outside the palette did something")
	}
}

func TestPrefixAndFuzzyMatchers(t *testing.T) {
	if _, hits, ok := prefix("qu i", "Quit"); !ok || len(hits) != 3 {
		t.Errorf("prefix ignoring spaces: %v %v", hits, ok)
	}
	if _, _, ok := prefix("ui", "Quit"); ok {
		t.Error("prefix matched inside the name")
	}
	if _, _, ok := prefix("", "Quit"); !ok {
		t.Error("an empty query should match")
	}
	if s1, _, ok := fuzzy("qt", "Quit"); !ok || s1 <= 0 {
		t.Errorf("fuzzy scattered letters: %d %v", s1, ok)
	}
	if _, _, ok := fuzzy("zz", "Quit"); ok {
		t.Error("fuzzy matched absent letters")
	}
}

// The palette key is ">" across the easy apps and tireless-watcher (Bob,
// 2026-11-27: "let's do the same across the easy apps so the shortcut is
// consistent"); ":", the old key, opens nothing and has no alias.
func TestPaletteKeyIsGreaterThanAndColonDoesNothing(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	if next, cmd := m.Update(key2(":")); next.(Model).palette != nil || cmd != nil {
		t.Fatal(`":" should do nothing`)
	}
	if next, _ := m.Update(key2(">")); next.(Model).palette == nil {
		t.Fatal(`">" should open the palette`)
	}
}

// Inside the palette ">" is text of the query, as it is in the bar.
func TestGreaterThanIsTextInThePaletteQuery(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	m, _ = feed(m, typeText(">>q")...)
	if m.palette == nil || m.palette.query != ">q" || len(m.filtered()) != 0 {
		t.Fatalf("palette = %+v", m.palette)
	}
}
