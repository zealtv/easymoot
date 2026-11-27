package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// allowedRead says whether the spy's calls are what one read of room may make:
// the room list, the name, that room's posts, and a stat of any other room's posts/ and, when that changed, a listing (names only) of it, which the poll chain makes for the unread marks. No body of another
// room's, ever.
func allowedRead(calls []string, room string) error {
	for _, c := range calls {
		switch {
		case c == "Rooms", c == "Name", strings.HasPrefix(c, "List "), strings.HasPrefix(c, "Stat "), strings.HasPrefix(c, "Body "+room+" "):
		default:
			return fmt.Errorf("call %q is no part of a read of room %q", c, room)
		}
	}
	return nil
}

// onlyAReadOf checks what cmd produces: nothing, or a read of the selected room
// outside the poll chain (review note M1). It runs the command over the spy.
func onlyAReadOf(t *testing.T, spy *spySource, cmd tea.Cmd, next Model, what string) {
	t.Helper()
	spy.take()
	if cmd == nil {
		return
	}
	msg := cmd()
	read, ok := msg.(pollMsg)
	if !ok {
		t.Fatalf("%s: the command produced %T, want nothing or a pollMsg", what, msg)
	}
	if read.chain {
		t.Fatalf("%s: the read starts a second poll chain", what)
	}
	if read.asked != next.room || next.room == "" {
		t.Fatalf("%s: read of %q, but the selected room is %q", what, read.asked, next.room)
	}
	if err := allowedRead(spy.take(), next.room); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// TestKeyRoutingNeverQuitsOrActs is the key-safety table. Outside the palette, no
// key of any type (every named key, every control code, every printable ASCII rune
// with and without alt) quits, opens the palette or reaches anything that writes,
// except ctrl+c, which quits and ">", which opens the palette (":", the old key, now does nothing). The one work a
// looking key may schedule is a read of the selected room, and the test checks the
// type of message the command produces. A regression such as "q quits" fails here.
func TestKeyRoutingNeverQuitsOrActs(t *testing.T) {
	f := longMoot(t)
	f.room("old.archived") // hidden, and no letter shows it
	f.post("dev", "20260801-080000-pinned", "# Customs\n")
	spy := newSpy(f)
	base := shown(t, spy)
	spy.take()

	keys := everyKey()

	// A participant selected in the sidebar is one more place to start from.
	people := seeing(base, somePeople(30))
	moved := false
	for _, focus := range []pane{paneRooms, panePosts} {
		for _, view := range []string{"rooms", "scrolled", "participants"} {
			start := base
			start.focus = focus
			switch view {
			case "scrolled":
				start = start.scrollTo(2)
			case "participants":
				start = people
				start.focus = focus
			}
			for _, k := range keys {
				if k.Type == tea.KeyCtrlC && !k.Alt {
					continue
				}
				if k.String() == paletteKey {
					continue // opens the palette; covered below
				}
				next, cmd := start.Update(k)
				nm := next.(Model)
				if nm.palette != nil || nm.bar != nil {
					t.Fatalf("key %q (type %d) opened the palette or the bar", k.String(), k.Type)
				}
				if nm.showArchived != start.showArchived {
					t.Fatalf("key %q (type %d) toggled the archived rooms", k.String(), k.Type)
				}
				if cmd != nil && quits(cmd) {
					t.Fatalf("key %q (type %d) quit", k.String(), k.Type)
				}
				if cmd != nil {
					moved = true
				}
				arrow := k.String() == "up" || k.String() == "down"
				if !arrow && (nm.onPerson != start.onPerson || nm.person != start.person || nm.sel != start.sel) {
					t.Fatalf("key %q (type %d) moved the selection", k.String(), k.Type)
				}
				if resizes := k.String() == "shift+left" || k.String() == "shift+right"; !resizes && nm.sideWidth != start.sideWidth {
					t.Fatalf("key %q (type %d) resized the sidebar", k.String(), k.Type)
				}
				onlyAReadOf(t, spy, cmd, nm, fmt.Sprintf("key %q (type %d)", k.String(), k.Type))
			}
		}
	}
	if !moved {
		t.Fatal("no key selected a room: the test has not exercised the read it allows")
	}

	_, cmd := base.Update(key(tea.KeyCtrlC))
	if !quits(cmd) {
		t.Fatal("ctrl+c did not quit")
	}
	if _, cmd := base.openPalette().Update(key(tea.KeyCtrlC)); !quits(cmd) {
		t.Fatal("ctrl+c did not quit with the palette open")
	}

	// With the entry bar open. Nothing but a typed Enter on non-blank text, with the
	// bar idle, reaches the poster, and a paste never does; nothing quits but ctrl+c,
	// opens the palette, or moves off the room the bar names. The test checks the
	// type of the message a returned command produces, and the spy's calls.
	open := openBarOn(t, base)
	if open.bar == nil {
		t.Fatal("the palette's Post did not open the bar")
	}
	posted := false
	for _, state := range []struct {
		name string
		text string
		busy bool
	}{{"empty", "", false}, {"blank", " \n  ", false}, {"text", "hello there", false}, {"multi-line", "a\nb", false}, {"busy", "hello there", true}} {
		for _, k := range keys {
			if k.Type == tea.KeyCtrlC && !k.Alt {
				continue
			}
			what := fmt.Sprintf("bar %s, key %q (type %d, alt %v, paste %v)", state.name, k.String(), k.Type, k.Alt, k.Paste)
			b := *open.bar
			b.text, b.busy = state.text, state.busy
			start := open
			start.bar = &b
			spy.take()
			next, cmd := start.Update(k)
			nm := next.(Model)
			if nm.palette != nil {
				t.Fatalf("%s: opened the palette", what)
			}
			if nm.room != start.room {
				t.Fatalf("%s: moved off the room", what)
			}
			typedEnter := k.Type == tea.KeyEnter && !k.Alt && !k.Paste
			wantPost := typedEnter && !state.busy && strings.TrimSpace(state.text) != ""
			if !wantPost {
				if cmd != nil {
					t.Fatalf("%s: scheduled a command (%T)", what, cmd())
				}
				// Esc closes the bar, and so does an Alt key (a fused Esc+key), unless
				// the bar is busy, which ignores every key but ctrl+c.
				if nm.bar == nil && k.Type != tea.KeyEsc && !(k.Alt && !state.busy) {
					t.Fatalf("%s: closed the bar", what)
				}
				if nm.bar != nil && k.Alt && !state.busy && !k.Paste {
					t.Fatalf("%s: an Alt key should close the bar", what)
				}
				continue
			}
			posted = true
			msg, ok := cmd().(postMsg)
			if !ok {
				t.Fatalf("%s: the command produced something other than a post", what)
			}
			if msg.room != "dev" {
				t.Fatalf("%s: posted to %q", what, msg.room)
			}
			if calls := spy.take(); len(calls) != 1 || calls[0] != "Post dev "+strconv.Quote(state.text) {
				t.Fatalf("%s: poster calls %v", what, calls)
			}
			if nm.bar == nil || !nm.bar.busy {
				t.Fatalf("%s: the bar should stay up, inert, until the answer", what)
			}
		}
	}
	if !posted {
		t.Fatal("no Enter reached the poster: the test has not exercised what it allows")
	}
	// ctrl+c quits from the bar, busy or not.
	for _, busy := range []bool{false, true} {
		b := *open.bar
		b.busy = busy
		start := open
		start.bar = &b
		if _, cmd := start.Update(key(tea.KeyCtrlC)); !quits(cmd) {
			t.Fatalf("ctrl+c did not quit with the bar open (busy %v)", busy)
		}
	}
	// And the mouse: it looks at nothing while the bar is open.
	for _, b := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
		next, cmd := open.Update(tea.MouseMsg{X: 3, Y: 4, Button: b, Action: tea.MouseActionPress})
		if cmd != nil || next.(Model).room != open.room || next.(Model).bar == nil {
			t.Fatalf("mouse %v reached past the open bar", b)
		}
	}
}

// everyKey is every key type, every printable rune with and without alt, and the
// pastes and alt-modified keys worth trying, for the key-table tests.
func everyKey() []tea.KeyMsg {
	var keys []tea.KeyMsg
	for kt := tea.KeyType(-300); kt <= 127; kt++ {
		keys = append(keys, tea.KeyMsg{Type: kt})
	}
	for r := rune(0x20); r < 0x7f; r++ {
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
	}
	keys = append(keys, tea.KeyMsg{Type: tea.KeyEnter, Alt: true}, tea.KeyMsg{Type: tea.KeyUp, Alt: true},
		tea.KeyMsg{Type: tea.KeyEsc, Alt: true}, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}, Alt: true})
	for _, p := range []string{"quit", ":", ">", "", "\n", "\r", "\r\n", "hello\r", "hello\r\n", "a\nb\n", ":post\nhello\n", ">post\nhello\n", "\x1b[2J"} {
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(p), Paste: true})
	}
	// A paste flagged as another key type must not confirm either.
	keys = append(keys, tea.KeyMsg{Type: tea.KeyEnter, Paste: true}, tea.KeyMsg{Type: tea.KeyEnter, Runes: []rune("\n"), Paste: true})
	return keys
}

func TestSpecificKeysDoNothingDangerous(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Q")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")},
		tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}, key(tea.KeyEnter), key(tea.KeyEsc),
		key(tea.KeyLeft), key(tea.KeyRight),
	} {
		next, cmd := m.Update(msg)
		if cmd != nil {
			t.Fatalf("%v scheduled a command", msg)
		}
		if nm := next.(Model); nm.palette != nil || nm.room != m.room {
			t.Fatalf("%v changed something", msg)
		}
	}
}

// TestMouseOnlyLooks feeds every button, action and a spread of positions, with
// the palette closed and open: nothing quits or opens the palette, and the only work
// is the read of the room a click selected.
func TestMouseOnlyLooks(t *testing.T) {
	spy := newSpy(synthetic(t))
	base := shown(t, spy)
	buttons := []tea.MouseButton{tea.MouseButtonNone, tea.MouseButtonLeft, tea.MouseButtonMiddle, tea.MouseButtonRight,
		tea.MouseButtonWheelUp, tea.MouseButtonWheelDown, tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight}
	actions := []tea.MouseAction{tea.MouseActionPress, tea.MouseActionRelease, tea.MouseActionMotion}
	selected := false
	for _, withPalette := range []bool{false, true} {
		m := base
		if withPalette {
			m = m.openPalette()
		}
		for _, b := range buttons {
			for _, a := range actions {
				for _, pos := range [][2]int{{0, 0}, {5, 2}, {5, 3}, {5, 5}, {70, 5}, {99, 29}, {500, 500}, {-1, -1}} {
					for clicks := 0; clicks < 2; clicks++ { // a double-click is two presses
						next, cmd := m.Update(tea.MouseMsg{X: pos[0], Y: pos[1], Button: b, Action: a})
						nm := next.(Model)
						if cmd != nil && quits(cmd) {
							t.Fatalf("mouse %v %v at %v quit", b, a, pos)
						}
						onlyAReadOf(t, spy, cmd, nm, fmt.Sprintf("mouse %v %v at %v", b, a, pos))
						if withPalette && (cmd != nil || nm.room != m.room) {
							t.Fatalf("mouse reached past the open palette at %v", pos)
						}
						if !withPalette && (nm.palette != nil) {
							t.Fatalf("mouse opened the palette")
						}
						selected = selected || cmd != nil
						m = nm
					}
				}
			}
		}
	}
	if !selected {
		t.Fatal("no click selected a room: the test has not exercised the read it allows")
	}
}

func TestClickSelectsARoomAndReadsIt(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := shown(t, spy)
	spy.take()
	// Rooms start on the row under the sidebar's title (body y=1, so rows 2, 3, 4).
	next, cmd := m.Update(tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	nm := next.(Model)
	if nm.room != "lobby" || nm.focus != paneRooms {
		t.Fatalf("click: room %q focus %v", nm.room, nm.focus)
	}
	if _, ok := cmd().(pollMsg); !ok {
		t.Fatal("a selection should read the room")
	}
	// A click in the posts pane focuses it and reads nothing.
	next, cmd = m.Update(tea.MouseMsg{X: 60, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd != nil || next.(Model).focus != panePosts {
		t.Fatalf("a click on the posts should only focus them")
	}
}

func TestArrowsSelectRoomsOrScrollPosts(t *testing.T) {
	m := shown(t, newSpy(longMoot(t)))
	if m.room != "dev" || m.sel != 0 {
		t.Fatalf("the first room should be selected at start: %q %d", m.room, m.sel)
	}
	next, cmd := m.Update(key(tea.KeyDown))
	nm := next.(Model)
	if nm.room != "lobby" || cmd == nil {
		t.Fatalf("down with the sidebar focused: room %q", nm.room)
	}
	if len(nm.posts) != 0 {
		t.Fatal("the posts of the room left behind are still shown")
	}
	// Past the ends nothing moves and nothing is read.
	next, cmd = m.Update(key(tea.KeyUp))
	if cmd != nil || next.(Model).room != "dev" {
		t.Fatal("up at the first room should do nothing")
	}
	// With the posts focused the arrows scroll, and read nothing.
	m.focus = panePosts
	m = m.scrollTo(5)
	next, cmd = m.Update(key(tea.KeyDown))
	if cmd != nil || next.(Model).top != m.top+1 || next.(Model).room != "dev" {
		t.Fatalf("down with the posts focused: top %d -> %d, cmd %v", m.top, next.(Model).top, cmd)
	}
}

func TestTabSwapsFocusAndSinglePaneShowsOne(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	m, _ = feed(m, tea.WindowSizeMsg{Width: 50, Height: 12})
	if v := m.View(); !strings.Contains(v, "Rooms") || strings.Contains(v, "2026-08-01") {
		t.Fatalf("rooms only expected:\n%s", v)
	}
	m, _ = feed(m, key(tea.KeyTab))
	if v := m.View(); strings.Contains(v, "Rooms") || !strings.Contains(v, "2026-08-02") {
		t.Fatalf("posts only expected:\n%s", v)
	}
}

func TestFocusedPaneRuleIsHeavyAndTheOtherLight(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "━━ Rooms") || !strings.Contains(v, "── dev") {
		t.Fatalf("sidebar focused: want a heavy Rooms rule and a light dev rule:\n%s", v)
	}
	m, _ = feed(m, key(tea.KeyTab))
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "── Rooms") || !strings.Contains(v, "━━ dev") {
		t.Fatalf("posts focused: want a light Rooms rule and a heavy dev rule:\n%s", v)
	}
}

func TestLayoutModes(t *testing.T) {
	cases := []struct {
		w, h int
		want layoutMode
	}{{160, 45, layoutSplit}, {100, 30, layoutSplit}, {64, 20, layoutSplit}, {63, 30, layoutSingle}, {40, 7, layoutSingle},
		{19, 30, layoutTiny}, {80, 2, layoutTiny}}
	for _, c := range cases {
		if got := computeLayout(c.w, c.h, false, 0).mode; got != c.want {
			t.Errorf("%dx%d: mode %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

// TestResizeTheSidebar: shift+←/→ and a drag on the divider column set the sidebar's
// width for the session, between leastSide and half the window; the posts wrap to
// what is left. The one-pane layout has no divider and ignores both. No letter
// resizes (TestKeyRoutingNeverQuitsOrActs).
func TestResizeTheSidebar(t *testing.T) {
	m := shown(t, newSpy(synthetic(t))) // 100x30: the default is a quarter, 25
	side := func(m Model) int { return m.layout().side.w }
	press := func(x int, a tea.MouseAction) tea.MouseMsg {
		return tea.MouseMsg{X: x, Y: 5, Button: tea.MouseButtonLeft, Action: a}
	}
	release := tea.MouseMsg{X: 0, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionRelease}
	cases := []struct {
		name string
		msgs []tea.Msg
		want int
	}{
		{"default", nil, 25},
		{"shift+right", []tea.Msg{key(tea.KeyShiftRight)}, 27},
		{"shift+left", []tea.Msg{key(tea.KeyShiftLeft)}, 23},
		{"clamped narrow", repeat(key(tea.KeyShiftLeft), 20), leastSide},
		{"clamped wide", repeat(key(tea.KeyShiftRight), 40), 50},
		{"drag", []tea.Msg{press(25, tea.MouseActionPress), press(40, tea.MouseActionMotion), release}, 40},
		{"drag clamped", []tea.Msg{press(25, tea.MouseActionPress), press(90, tea.MouseActionMotion), press(2, tea.MouseActionMotion)}, leastSide},
		{"motion after release", []tea.Msg{press(25, tea.MouseActionPress), release, press(40, tea.MouseActionMotion)}, 25},
		{"press off the divider", []tea.Msg{press(24, tea.MouseActionPress), press(40, tea.MouseActionMotion)}, 25},
	}
	for _, c := range cases {
		got, _ := feed(m, c.msgs...)
		if side(got) != c.want || got.postsWidth() != 100-c.want-1-2 {
			t.Errorf("%s: sidebar %d, posts wrap at %d; want %d", c.name, side(got), got.postsWidth(), c.want)
		}
	}
	wide, _ := feed(m, repeat(key(tea.KeyShiftRight), 40)...)
	if got := side(resized(t, wide, 80, 24)); got != 40 {
		t.Errorf("a 50-wide sidebar in an 80-wide window is %d, want 40", got)
	}
	narrow := resized(t, m, 60, 20)
	for _, msgs := range [][]tea.Msg{{key(tea.KeyShiftRight)}, {press(15, tea.MouseActionPress), press(30, tea.MouseActionMotion)}} {
		if got, _ := feed(narrow, msgs...); got.sideWidth != 0 || got.View() != narrow.View() {
			t.Errorf("the one-pane layout took a resize: %d", got.sideWidth)
		}
	}
}

func repeat(msg tea.Msg, n int) []tea.Msg {
	out := make([]tea.Msg, n)
	for i := range out {
		out[i] = msg
	}
	return out
}

// TestViewFitsEverySize sweeps every mode over a grid of sizes, plus the two the frames
// are drawn at (80x24 and 60x20, colour off): no line wider or taller than the screen.
// At those two it also checks the bars' bottom row and no escape, and writes the frames
// to EASYMOOT_CAPTURE_DIR when that is set.
func TestViewFitsEverySize(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	f := synthetic(t)
	t.Setenv("HOME", filepath.Dir(f.folder))
	base := shown(t, newSpy(f))
	// The kinds modes: archived rooms on show, and an archived room open.
	shownArchived, _ := toggle(t, shown(t, newSpy(kindsMoot(t))), "s")
	next, cmd := shownArchived.jump(shownArchived.indexByName(t, "old.archived"))
	// The participants frames show nine rows with the second selected.
	nine, _ := feed(seeing(base, somePeople(9)), key(tea.KeyDown))
	bases := map[string]Model{"archived shown": shownArchived, "archived open": answer(t, next.(Model), cmd),
		"participants": seeing(base, somePeople(30)), "participants 9": nine, "dm bar": nine}
	// The modes whose frames are kept, and the file each is written to.
	frames := map[string]string{"archive confirm": "actions-archive-confirm", "create name": "actions-create-name",
		"participants 9": "participants", "dm bar": "dm-bar"}
	modes := []string{"plain", "palette", "bar", "bar busy", "archived shown", "archived open", "participants",
		"archive confirm", "create name", "participants 9", "dm bar"}
	sizes := [][2]int{{80, 24}, {60, 20}}
	for w := 1; w <= 130; w += 3 {
		for h := 1; h <= 40; h += 3 {
			sizes = append(sizes, [2]int{w, h})
		}
	}
	out := os.Getenv("EASYMOOT_CAPTURE_DIR")
	for _, mode := range modes {
		start, ok := bases[mode]
		if !ok {
			start = base
		}
		for _, focus := range []pane{paneRooms, panePosts} {
			for i, size := range sizes {
				w, h := size[0], size[1]
				m := start
				m.focus = focus
				next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m = next.(Model)
				switch {
				case mode == "palette":
					m = m.openPalette()
					m, _ = feed(m, key(tea.KeyDown))
				case strings.HasPrefix(mode, "bar"):
					m = openBarOn(t, m)
					m, _ = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a long line of text 日本語 to push the bar past its width, then\r\nmore"), Paste: true})
					if mode == "bar busy" {
						m.bar.busy = true
					}
				case mode == "archive confirm":
					m, _ = feed(m, typed("archive")...)
				case mode == "create name":
					m, _ = feed(m, append(typed("create"), typeText("run-notes")...)...)
				case mode == "dm bar":
					m, _ = feed(m, append(typed("dm"), typeText("are you free for a review of the archive frames?")...)...)
				}
				view := m.View()
				rows := strings.Split(view, "\n")
				if len(rows) > h {
					t.Fatalf("%s %dx%d: %d lines", mode, w, h, len(rows))
				}
				for j, line := range rows {
					if lipgloss.Width(line) > w {
						t.Fatalf("%s %dx%d line %d is %d wide: %q", mode, w, h, j, lipgloss.Width(line), ansi.Strip(line))
					}
				}
				file, framed := frames[mode]
				if focus == panePosts && mode == "participants 9" && w < splitWidth {
					file += "-strip" // one pane at a time: the strip is its own frame
				} else if focus != paneRooms {
					continue
				}
				if i >= 2 || !framed {
					continue
				}
				last := ansi.Strip(rows[len(rows)-1])
				// The confirmation always keeps its keys; a bar, like the post bar, gives
				// them up when the text needs the room.
				if len(rows) != h || (mode == "archive confirm" && !strings.Contains(last, "esc cancel")) ||
					(strings.HasSuffix(mode, "bar") || strings.HasSuffix(mode, "name") || mode == "archive confirm") && !strings.Contains(last, "›") ||
					escapes.MatchString(view) {
					t.Errorf("%s at %dx%d: %d lines, bottom row %q, or an escape", mode, w, h, len(rows), last)
				}
				if out != "" {
					if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("%s-%dx%d.txt", file, w, h)), []byte(view+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

// Text from a moot is inert: room names, senders, post bodies and the folder name
// pass through the cleaner, so no escape or control character reaches the terminal.
func TestUntrustedTextIsInert(t *testing.T) {
	f := newFixture(t, "ev\x1b[2Jil")
	f.room("ro\x1b]0;title\x07om")
	f.post("ro\x1b]0;title\x07om", "20260101-090000-ev\x1b[31mil", "a\x1b[31mred\x00\tb\n\x1b[2Jgone\r\nnext\n")
	f.post("ro\x1b]0;title\x07om", "READ\x1bME", "stray\n")
	m := shown(t, f.open())
	for _, v := range []string{m.View(), m.openPalette().View()} {
		if strings.ContainsAny(strings.ReplaceAll(ansi.Strip(v), "\n", ""), "\x1b\x07\x00\r") {
			t.Fatalf("control character leaked: %q", v)
		}
		if strings.Contains(v, "\x1b[2J") || strings.Contains(v, "\x1b]0;") {
			t.Fatalf("escape sequence leaked: %q", v)
		}
	}
	if !strings.Contains(m.View(), "·") {
		t.Fatal("the control characters should show as middle dots")
	}
}

// Init, Update and View start no read: every read is a command, and the spy sees a
// read only when a command runs.
func TestUpdateAndViewPerformNoIO(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := New(spy, utc)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return the first read as a command")
	}
	if calls := spy.take(); len(calls) != 0 {
		t.Fatalf("Init read: %v", calls)
	}
	m = answer(t, m, cmd)
	if len(spy.take()) == 0 {
		t.Fatal("the command did not read")
	}
	if m.View() != m.View() {
		t.Fatal("View is not pure")
	}
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 90, Height: 20}, key(tea.KeyTab), key(tea.KeyPgUp),
		key(tea.KeyEnd), key2(paletteKey), key(tea.KeyEsc)} {
		m, _ = feed(m, msg)
		_ = m.View()
	}
	if calls := spy.take(); len(calls) != 0 {
		t.Fatalf("Update or View read: %v", calls)
	}
}

func TestEmptyAndErrorStates(t *testing.T) {
	// A moot with no rooms says how rooms get there, and makes none.
	f := newFixture(t, "bare")
	m := shown(t, f.open())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No rooms.") || !strings.Contains(v, "SKILL.md") {
		t.Fatalf("no rooms:\n%s", v)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.folder, ".moot", "rooms")); len(entries) != 0 {
		t.Fatal("Easymoot made a room")
	}
	// A room with no posts.
	f.room("quiet")
	m = answer(t, m, tickPoll(m))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "quiet") || !strings.Contains(v, "No posts yet.") {
		t.Fatalf("no posts:\n%s", v)
	}
	// A rooms directory that cannot be read: the error shows, nothing is invented.
	g := newFixture(t, "broken")
	if err := os.RemoveAll(filepath.Join(g.folder, ".moot", "rooms")); err != nil {
		t.Fatal(err)
	}
	g.write(".moot/rooms", "not a directory")
	m = shown(t, g.open())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "could not read") || !strings.Contains(v, "! ") {
		t.Fatalf("read error not shown:\n%s", v)
	}
}

// tickPoll is the command a poll tick starts, for tests that step the loop by hand.
func tickPoll(m Model) tea.Cmd {
	return pollCmd(m.src, m.room, m.bodies, nil, false, pollOpts{show: m.showArchived})
}

func TestHeaderNamesTheFolderTheNameAndTheRoomCount(t *testing.T) {
	f := synthetic(t)
	t.Setenv("HOME", filepath.Dir(f.folder))
	m := shown(t, f.open())
	head := ansi.Strip(strings.Split(m.View(), "\n")[0])
	for _, want := range []string{"Easymoot", "3 rooms", "as atlas", "~/atlas"} {
		if !strings.Contains(head, want) {
			t.Errorf("header lacks %q: %q", want, head)
		}
	}
	f.write(".moot/name", "  ada lovelace \n")
	m = answer(t, m, tickPoll(m))
	if head := ansi.Strip(strings.Split(m.View(), "\n")[0]); !strings.Contains(head, "as adalovelace") {
		t.Errorf("the name should follow .moot/name, whitespace removed: %q", head)
	}
	f.write(".moot/name", "bad/name\n")
	m = answer(t, m, tickPoll(m))
	if head := ansi.Strip(strings.Split(m.View(), "\n")[0]); !strings.Contains(head, "no usable name") {
		t.Errorf("an unsafe name should say so: %q", head)
	}
}

func TestScrollKeysAndTheEndFollowsAgain(t *testing.T) {
	m := shown(t, newSpy(longMoot(t)))
	if !m.follow || m.top != m.maxTop() {
		t.Fatalf("a room opens at the bottom: top %d of %d follow %v", m.top, m.maxTop(), m.follow)
	}
	bottom := m.top
	m, _ = feed(m, key(tea.KeyPgUp))
	if m.follow || m.top >= bottom {
		t.Fatalf("page up: top %d follow %v", m.top, m.follow)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "scrolled up") {
		t.Fatalf("the title should say the view is not at the bottom:\n%s", v)
	}
	m, _ = feed(m, key(tea.KeyHome))
	if m.top != 0 {
		t.Fatalf("home: top %d", m.top)
	}
	m, _ = feed(m, key(tea.KeyEnd))
	if !m.follow || m.top != bottom {
		t.Fatalf("end: top %d follow %v", m.top, m.follow)
	}
	m, _ = feed(m, key(tea.KeyCtrlU), key(tea.KeyCtrlD))
	if !m.follow {
		t.Fatal("ctrl+u then ctrl+d should come back to the bottom")
	}
	// shift+up scrolls the posts from the sidebar too.
	m.focus = paneRooms
	m, _ = feed(m, tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.top != bottom-1 || m.room != "dev" {
		t.Fatalf("shift+up: top %d room %q", m.top, m.room)
	}
}

func TestWheelScrollsThePostsUnderIt(t *testing.T) {
	m := shown(t, newSpy(longMoot(t)))
	bottom := m.top
	next, cmd := m.Update(tea.MouseMsg{X: 60, Y: 10, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if cmd != nil || next.(Model).top != bottom-3 {
		t.Fatalf("wheel up over the posts: top %d, cmd %v", next.(Model).top, cmd)
	}
}

// TestSessionChangesNothingUnderTheMoot: a session never moves a cursor's position
// or changes any file under the moot. The session runs the folder's own moot.sh rooms
// at open (link repair, which can rename a link and its cursor's folder, never a
// position); this moot has no link to mend, so nothing changes.
func TestSessionChangesNothingUnderTheMoot(t *testing.T) {
	f := synthetic(t)
	f.installScript()
	f.write(".moot/seen/dev/atlas", "20260801-090000-ada\n")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(f.folder, ".moot/seen/dev/atlas"), old, old); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, f.folder)
	m := shown(t, f.open())
	for _, msg := range []tea.Msg{key(tea.KeyDown), key(tea.KeyTab), key(tea.KeyPgUp), key2(paletteKey), key(tea.KeyEsc)} {
		var cmd tea.Cmd
		m, cmd = feed(m, msg)
		if cmd != nil {
			if p, ok := cmd().(pollMsg); ok {
				m, _ = feed(m, p)
			}
		}
	}
	m = answer(t, m, tickPoll(m))
	_ = m.View()
	after := snapshot(t, f.folder)
	if before != after {
		t.Fatalf("the session changed the moot:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// snapshot lists every path under dir with its size and modification time.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(&b, "%s %d %d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
