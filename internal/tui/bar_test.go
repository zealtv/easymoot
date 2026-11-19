package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

func paste(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true} }

// lastRow is the bottom line of the frame, as plain text.
func lastRow(m Model) string {
	rows := lines(ansi.Strip(m.View()))
	return strings.TrimRight(rows[len(rows)-1], " ")
}

// statusRow is the line above the bottom one.
func statusRow(m Model) string {
	rows := lines(ansi.Strip(m.View()))
	return strings.TrimRight(rows[len(rows)-2], " ")
}

// poster returns the calls the spy saw that were posts.
func postCalls(spy *spySource) []string {
	var out []string
	for _, c := range spy.take() {
		if strings.HasPrefix(c, "Post ") {
			out = append(out, c)
		}
	}
	return out
}

func TestPostOpensTheBarForTheSelectedRoom(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := shown(t, spy)
	m, _ = feed(m, key(tea.KeyDown)) // select lobby; the bar names the room selected when it opens
	m = answer(t, m, m.readSelected())
	m = openBarOn(t, m)
	if m.bar.room != "lobby" || m.bar.name != "atlas" || m.bar.text != "" || m.palette != nil {
		t.Fatalf("bar = %+v", *m.bar)
	}
	if got := lastRow(m); !strings.HasPrefix(got, " post to lobby as atlas ›") || !strings.Contains(got, "enter post · esc cancel") {
		t.Fatalf("bar row: %q", got)
	}
	if !strings.Contains(statusRow(m), "can't be taken back") {
		t.Fatalf("status row: %q", statusRow(m))
	}
	if len(postCalls(spy)) != 0 {
		t.Fatal("opening the bar reached the poster")
	}
}

// Post matches only a prefix of its name, as Quit does.
func TestPostMatchesOnlyThePrefix(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	for _, q := range []string{"ost", "pst", "send", "write", "postt", "post now"} {
		p, _ := feed(m, typeText(":"+q)...)
		if p.palette.chosen != "" || len(p.filtered()) != 0 {
			t.Errorf("%q found a command", q)
		}
		if p, cmd := p.Update(key(tea.KeyEnter)); cmd != nil || p.(Model).bar != nil {
			t.Errorf("%q then enter did something", q)
		}
	}
	for _, q := range []string{"p", "po", "POST", "p o s t"} {
		p, _ := feed(m, typeText(":"+q)...)
		if p.palette.chosen != actionPost {
			t.Errorf("%q did not choose Post", q)
		}
	}
}

func TestBarEditing(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	step := func(msg tea.Msg, want string) {
		t.Helper()
		var cmd tea.Cmd
		m, cmd = feed(m, msg)
		if cmd != nil {
			t.Fatalf("%v scheduled a command", msg)
		}
		if m.bar == nil || m.bar.text != want {
			t.Fatalf("after %v: text %q, want %q", msg, m.bar.text, want)
		}
	}
	for _, msg := range typeText("hello wor") {
		m, _ = feed(m, msg)
	}
	step(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, "hello wor ")
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ld")}, "hello wor ld") // several runes at once
	step(key(tea.KeyBackspace), "hello wor l")
	step(tea.KeyMsg{Type: tea.KeyCtrlH}, "hello wor ")
	step(key(tea.KeyCtrlW), "hello ")
	step(key(tea.KeyCtrlW), "")
	step(key(tea.KeyBackspace), "") // nothing to delete
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ünï 日本")}, "ünï 日本")
	step(key(tea.KeyBackspace), "ünï 日")
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\x00b\x1bc")}, "ünï 日abc") // control runes are dropped
	step(key(tea.KeyCtrlU), "")
	// ctrl+w treats a line break as a word boundary, whitespace first.
	step(paste("one two\nthree  \n"), "one two\nthree  \n")
	step(key(tea.KeyCtrlW), "one two\n")
	step(key(tea.KeyCtrlW), "one ")
	// Tab, arrows and the paging keys neither type nor move anything.
	for _, k := range []tea.KeyType{tea.KeyTab, tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight, tea.KeyPgUp, tea.KeyHome, tea.KeyEnd, tea.KeyDelete} {
		step(key(k), "one ")
	}
	if got := lastRow(m); !strings.Contains(got, "› one ▌") {
		t.Fatalf("the typed text should show before the cursor: %q", got)
	}
}

// M1 (final review): a terminal sends Alt+key as Esc then the key, and Bubble Tea
// delivers the pair fused as one Alt key when they arrive in one read. The lost Esc
// must still close the bar and drop the draft, so "Esc, then :quit Enter" cannot post
// the draft joined to "quit".
func TestAltKeyClosesTheBarAndDropsTheText(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune(":"), Alt: true}, // the fused Esc and ":"
		{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, // Option+x as Meta
		{Type: tea.KeyLeft, Alt: true},                      // Option+left as Meta
		{Type: tea.KeyEnter, Alt: true},
	} {
		spy := newSpy(synthetic(t))
		m := openBarOn(t, shown(t, spy))
		m, _ = feed(m, typeText("draft")...)
		spy.take()
		m, cmd := feed(m, k)
		if cmd != nil || m.bar != nil {
			t.Fatalf("%v: the bar should close with no command", k)
		}
		// What Bob types next is the shell's, not more of the draft: "quit" and Enter
		// post nothing.
		m, cmd = feed(m, append(typeText("quit"), key(tea.KeyEnter))...)
		if cmd != nil && quits(cmd) {
			t.Fatalf("%v: typed text quit the program with the bar closed", k)
		}
		if calls := spy.take(); len(calls) != 0 {
			for _, c := range calls {
				if strings.HasPrefix(c, "Post ") {
					t.Fatalf("%v: posted after the fused Esc: %v", k, calls)
				}
			}
		}
		if m.bar != nil {
			t.Fatalf("%v: the bar came back", k)
		}
	}
}

// L7: a paste goes in whole, with its line breaks as \n, drawn as ↵; it never posts,
// and a paste that ends in a line break does not post either.
func TestPasteEntersWholeAndNeverPosts(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := openBarOn(t, shown(t, spy))
	spy.take()
	cases := []struct{ in, want string }{
		{"one\r\ntwo\rthree\nfour", "one\ntwo\nthree\nfour"},
		{"ends here\n", "ends here\n"},
		{"ends here\r\n", "ends here\n"},
		{"ends here\r", "ends here\n"},
		{"\r\n\r\n", "\n\n"},
		{":post\nquit\n", ":post\nquit\n"},
	}
	for _, c := range cases {
		next, cmd := m.Update(paste(c.in))
		nm := next.(Model)
		if cmd != nil {
			t.Fatalf("paste %q scheduled a command", c.in)
		}
		if nm.bar == nil || nm.bar.text != c.want || nm.bar.busy || nm.palette != nil {
			t.Fatalf("paste %q: bar %+v", c.in, nm.bar)
		}
		if strings.ContainsAny(nm.bar.text, "\r") {
			t.Fatalf("paste %q kept a carriage return", c.in)
		}
		row := lastRow(nm)
		if want := "› " + strings.ReplaceAll(c.want, "\n", "↵") + "▌"; !strings.Contains(row, want) {
			t.Fatalf("paste %q drawn as %q, want it to contain %q", c.in, row, want)
		}
		if strings.Contains(row, "\n") || strings.Contains(row, "\r") {
			t.Fatalf("a line break reached the bar row: %q", row)
		}
	}
	if len(postCalls(spy)) != 0 {
		t.Fatal("a paste reached the poster")
	}
	// Pastes add to what was typed, and a typed Enter then posts the whole of it.
	m, _ = feed(m, typeText("see:")...)
	m, _ = feed(m, paste("\r\nline two\r\n"))
	if m.bar.text != "see:\nline two\n" {
		t.Fatalf("text %q", m.bar.text)
	}
	m, cmd := feed(m, key(tea.KeyEnter))
	if _, ok := cmd().(postMsg); !ok {
		t.Fatal("a typed Enter after a paste should post")
	}
	if calls := postCalls(spy); len(calls) != 1 || calls[0] != "Post dev "+strconv.Quote("see:\nline two\n") {
		t.Fatalf("poster calls: %v", calls)
	}
}

// A paste of only line breaks is blank text: a typed Enter does not post it either.
func TestEnterOnBlankPostsNothing(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := openBarOn(t, shown(t, spy))
	spy.take()
	for _, text := range []string{"", " ", "   ", "\n", " \n \n", "\t"} {
		b := *m.bar
		b.text = text
		m.bar = &b
		next, cmd := m.Update(key(tea.KeyEnter))
		if cmd != nil || next.(Model).bar == nil || next.(Model).bar.busy {
			t.Fatalf("Enter on %q acted", text)
		}
	}
	if len(postCalls(spy)) != 0 {
		t.Fatal("blank text reached the poster")
	}
}

func TestEscDropsTheBarAndItsText(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	m, _ = feed(m, typeText("draft")...)
	m, cmd := feed(m, key(tea.KeyEsc))
	if cmd != nil || m.bar != nil || m.palette != nil {
		t.Fatal("esc should close the bar")
	}
	if strings.Contains(lastRow(m), "post to") || strings.Contains(ansi.Strip(m.View()), "draft") {
		t.Fatal("the bar is still drawn")
	}
	m = openBarOn(t, m)
	if m.bar.text != "" {
		t.Fatalf("the dropped text came back: %q", m.bar.text)
	}
	// The keys of the shell work again once it is closed.
	m, _ = feed(m, key(tea.KeyEsc))
	if next, _ := m.Update(key(tea.KeyTab)); next.(Model).focus == m.focus {
		t.Fatal("tab should swap focus with the bar closed")
	}
}

// The bar takes every key, so a typed 'q', ':' or arrow is text or nothing, never the
// shell's.
func TestBarTakesEveryKeyFromTheShell(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	room, focus := m.room, m.focus
	for _, msg := range append(typeText("q: :quit"), key(tea.KeyTab), key(tea.KeyDown), key(tea.KeyPgUp), key(tea.KeyEnd)) {
		var cmd tea.Cmd
		m, cmd = feed(m, msg)
		if cmd != nil {
			t.Fatalf("%v scheduled a command", msg)
		}
	}
	if m.palette != nil || m.room != room || m.focus != focus || m.bar == nil || m.bar.text != "q: :quit" {
		t.Fatalf("the shell saw a key: room %q focus %v bar %+v", m.room, m.focus, m.bar)
	}
}

func TestBarIsInertWhileThePostRuns(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hello")...)
	spy.take()
	m, cmd := feed(m, key(tea.KeyEnter))
	if cmd == nil || m.bar == nil || !m.bar.busy {
		t.Fatal("Enter should start the post and keep the bar up")
	}
	if got := lastRow(m); !strings.Contains(got, "posting…") || strings.Contains(got, "esc cancel") {
		t.Fatalf("busy bar row: %q", got)
	}
	if !strings.Contains(statusRow(m), "posting to dev…") {
		t.Fatalf("busy status row: %q", statusRow(m))
	}
	for _, msg := range append(typeText("more"), key(tea.KeyEnter), key(tea.KeyEsc), key(tea.KeyBackspace), key(tea.KeyCtrlW),
		key(tea.KeyCtrlU), paste("pasted\n"), tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) {
		next, c := m.Update(msg)
		if c != nil {
			t.Fatalf("%v scheduled a command while posting", msg)
		}
		if nm := next.(Model); nm.bar == nil || nm.bar.text != "hello" || !nm.bar.busy {
			t.Fatalf("%v changed the bar while posting: %+v", msg, nm.bar)
		}
	}
	// ctrl+c while posting stops the post first and quits on the answer; the tests in
	// bar_cancel_test.go cover that. Here it only has to be taken, not ignored.
	if _, c := m.Update(key(tea.KeyCtrlC)); c == nil {
		t.Fatal("ctrl+c should act while posting")
	}
	// Run the post: one call, and the answer closes the bar.
	if _, ok := cmd().(postMsg); !ok {
		t.Fatal("the command did not produce the answer")
	}
	if calls := postCalls(spy); len(calls) != 1 {
		t.Fatalf("poster calls: %v", calls)
	}
}

func TestSuccessClosesTheBarSaysWhatWentOutAndReadsTheRoom(t *testing.T) {
	spy := newSpy(synthetic(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{Room: room, ID: "20260801-120000-bob", Sender: "bob",
			Notices: []string{"a push failed"}}, nil
	})
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hi")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	spy.take()
	next, read := m.Update(cmd())
	nm := next.(Model)
	if nm.bar != nil {
		t.Fatal("success should close the bar")
	}
	// The name in the answer is the script's own, which may differ from the bar's.
	if want := "posted to dev as bob · a push failed"; !strings.Contains(statusRow(nm), want) {
		t.Fatalf("status row %q, want %q", statusRow(nm), want)
	}
	onlyAReadOf(t, spy, read, nm, "the read after a post")
	if !strings.Contains(lastRow(nm), "commands") {
		t.Fatalf("the key hints should be back: %q", lastRow(nm))
	}
}

func TestFailureKeepsTheTextForAnotherTry(t *testing.T) {
	spy := newSpy(synthetic(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{}, &moot.ScriptError{Message: "room 'dev' has no posts/ directory\x1b[2J", ExitCode: 1}
	})
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("keep me")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	next, read := m.Update(cmd())
	nm := next.(Model)
	if read != nil {
		t.Fatal("a failure should read nothing")
	}
	if nm.bar == nil || nm.bar.busy || nm.bar.text != "keep me" {
		t.Fatalf("bar after a failure: %+v", nm.bar)
	}
	status := statusRow(nm)
	if !strings.Contains(status, "! Could not post to dev: room 'dev' has no posts/ directory") {
		t.Fatalf("status row: %q", status)
	}
	if strings.Contains(nm.View(), "\x1b[2J") {
		t.Fatal("an escape from the script's message reached the screen")
	}
	// Typing again clears the old failure; Enter tries once more.
	spy.setPoster(nil)
	nm, _ = feed(nm, typeText("!")...)
	if strings.Contains(statusRow(nm), "Could not post") {
		t.Fatal("the failure should go once the text changes")
	}
	nm, cmd = feed(nm, key(tea.KeyEnter))
	if cmd == nil || !nm.bar.busy {
		t.Fatal("Enter after a failure should post again")
	}
	cmd()
	if calls := postCalls(spy); len(calls) != 2 || calls[1] != "Post dev "+strconv.Quote("keep me!") {
		// the first call was recorded when the first command ran
		t.Fatalf("poster calls: %v", calls)
	}
}

func TestTimeoutClosesTheBarAndSaysSo(t *testing.T) {
	spy := newSpy(synthetic(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) { return moot.PostResult{}, moot.ErrTimeout })
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("slow")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	spy.take()
	next, read := m.Update(cmd())
	nm := next.(Model)
	if nm.bar != nil {
		t.Fatal("a time-out should close the bar, as after a success")
	}
	if s := statusRow(nm); !strings.Contains(s, "did not finish") || !strings.Contains(s, "next poll") || !strings.Contains(s, "! ") {
		t.Fatalf("status row: %q", s)
	}
	onlyAReadOf(t, spy, read, nm, "the read after a time-out")
}

// An answer for another folder's model is dropped.
func TestAnAnswerForAnotherSourceIsDropped(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	other := newSpy(synthetic(t))
	next, cmd := m.Update(postMsg{src: other, room: "dev", res: moot.PostResult{Room: "dev"}})
	if cmd != nil || next.(Model).bar == nil || next.(Model).notice != "" {
		t.Fatal("an answer from another source changed the model")
	}
}

func TestUnsafeNameRefusesToOpenTheBar(t *testing.T) {
	f := synthetic(t)
	f.write(".moot/name", "bad/name\n")
	spy := newSpy(f)
	m := shown(t, spy)
	spy.take()
	m, cmd := feed(m, append(typeText(":post"), key(tea.KeyEnter))...)
	if cmd != nil || m.bar != nil || m.palette != nil {
		t.Fatalf("the bar opened for an unsafe name: bar %v palette %v", m.bar, m.palette)
	}
	row := statusRow(m)
	if !strings.Contains(row, "! Can't post: sender name 'bad/name' is not filename-safe") {
		t.Fatalf("status row: %q", row)
	}
	if len(postCalls(spy)) != 0 {
		t.Fatal("the poster was reached")
	}
	// Typing into the pane afterwards still posts nothing.
	for _, msg := range append(typeText("hello"), key(tea.KeyEnter)) {
		if m, cmd = feed(m, msg); cmd != nil {
			t.Fatal("a key scheduled a command")
		}
	}
}

func TestNoBarWithoutARoomOrBeforeTheFirstRead(t *testing.T) {
	// No rooms: nothing to post to.
	f := newFixture(t, "bare")
	m := shown(t, f.open())
	m, _ = feed(m, append(typeText(":post"), key(tea.KeyEnter))...)
	if m.bar != nil || !strings.Contains(statusRow(m), "Can't post: no room is selected") {
		t.Fatalf("no room: bar %v status %q", m.bar, statusRow(m))
	}
	// Before anything is read, no name is known.
	m = New(newSpy(synthetic(t)), utc)
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.room = "dev"
	m, _ = feed(m, append(typeText(":post"), key(tea.KeyEnter))...)
	if m.bar != nil || !strings.Contains(statusRow(m), "Can't post: no name") {
		t.Fatalf("no name: bar %v status %q", m.bar, statusRow(m))
	}
}

// The bar's text is drawn through the cleaner: an escape in a paste shows as a dot.
func TestBarTextIsInert(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	m, _ = feed(m, paste("a\x1b[2Jb\x1b]0;x\x07c\x00d\te"))
	v := m.View()
	if strings.Contains(v, "\x1b[2J") || strings.Contains(v, "\x1b]0;") || strings.ContainsAny(ansi.Strip(v), "\x1b\x07\x00\r") {
		t.Fatalf("control characters reached the screen: %q", v)
	}
	if !strings.Contains(lastRow(m), "a·[2Jb·]0;x·c·d    e") {
		t.Fatalf("row %q", lastRow(m))
	}
}

func TestLongTextShowsItsEnd(t *testing.T) {
	m := openBarOn(t, shown(t, newSpy(synthetic(t))))
	m, _ = feed(m, typeText(strings.Repeat("abcdefghij ", 20)+"THE END")...)
	row := lastRow(m)
	if !strings.Contains(row, "› …") || !strings.Contains(row, "THE END▌") || len([]rune(row)) > 100 {
		t.Fatalf("row: %q", row)
	}
}

// A short window has no status row: a status waiting to be read shows in the bar's
// place until the next key, then the bar returns.
func TestShortWindowShowsTheFailureInsteadOfTheBarUntilAKey(t *testing.T) {
	spy := newSpy(synthetic(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{}, errors.New("boom")
	})
	m := shown(t, spy)
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 8})
	m = openBarOn(t, m)
	if got := lastRow(m); !strings.Contains(got, "post to dev as atlas") {
		t.Fatalf("bar row: %q", got)
	}
	m, _ = feed(m, typeText("x")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	m, _ = feed(m, cmd())
	if got := lastRow(m); !strings.Contains(got, "Could not post to dev: boom") {
		t.Fatalf("failure row: %q", got)
	}
	m, _ = feed(m, typeText("y")...)
	if got := lastRow(m); !strings.Contains(got, "› xy▌") {
		t.Fatalf("the bar should return on the next key: %q", got)
	}
}

// What a typed sentence can and can't reach, key by key into the pane (the
// architecture's "What a stray sentence can still reach").
func TestSentencesReachThePosterOnlyThroughTheWholeRoute(t *testing.T) {
	type outcome struct {
		posts []string // what reached the poster
		bar   bool     // the bar is open at the end
	}
	cases := []struct {
		name, keys string
		want       outcome
	}{
		// No colon: nothing is open, so letters and Enters are ignored.
		{"plain sentence", "please post this now\n\nOK\n", outcome{}},
		{"quit and post as words", "post\npost hello\nquit\n", outcome{}},
		// A colon opens the palette; the rest is its query, and Enter runs a command
		// only if the query is a prefix of one.
		{"colon sentence that is no command", "Note to self: post the milk\n\n", outcome{}},
		{"palette, no match, more Enters", ":posted\n\nhello\n", outcome{}},
		{"escaped palette", ":post\x1b" + "hello\n", outcome{}},
		// The whole route: colon, a prefix of "post", Enter opens the bar. That alone
		// posts nothing, and neither does Enter on blank text.
		{"route stops at the bar", ":post\n", outcome{bar: true}},
		{"a prefix is enough", ":p\n", outcome{bar: true}},
		{"a sentence ending in the name", "see note: post\n", outcome{bar: true}},
		{"blank Enters do nothing", ":post\n\n\n   \n", outcome{bar: true}},
		{"text without Enter", ":post\nhello", outcome{bar: true}},
		// ... and the last Enter posts.
		{"the whole route", ":post\nhello there\n", outcome{posts: []string{"hello there"}}},
		{"the whole route, after a sentence", "see note: post\nhello\n", outcome{posts: []string{"hello"}}},
		// Once the bar is open, a typed sentence is text: colons, 'quit' and Esc-less
		// words do nothing else, and each typed Enter posts the text before it.
		{"text keeps its colons", ":post\nquit: now\n", outcome{posts: []string{"quit: now"}}},
		{"typed Esc drops the draft", ":post\nhello\x1b" + "\n", outcome{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spy := newSpy(synthetic(t))
			m := shown(t, spy)
			spy.take()
			var got []string
			for _, r := range c.keys {
				var msg tea.Msg
				switch r {
				case '\n':
					msg = key(tea.KeyEnter)
				case '\x1b':
					msg = key(tea.KeyEsc)
				default:
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
					if r == ' ' {
						msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
					}
				}
				next, cmd := m.Update(msg)
				m = next.(Model)
				if cmd == nil {
					continue
				}
				if quits(cmd) {
					t.Fatalf("a key quit")
				}
				// quits ran the command once; the poster's calls say what it reached.
				for _, call := range postCalls(spy) {
					got = append(got, strings.TrimPrefix(call, "Post dev "))
				}
			}
			var want []string
			for _, p := range c.want.posts {
				want = append(want, strconv.Quote(p))
			}
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("reached the poster with %v, want %v", got, want)
			}
			if (m.bar != nil) != c.want.bar && len(c.want.posts) == 0 {
				t.Fatalf("bar open = %v, want %v", m.bar != nil, c.want.bar)
			}
		})
	}
}

// A model-level post through the pinned script over a synthetic moot: the post lands
// as one file with the text as typed (line breaks of a paste included), the only change
// under .moot/, and the next poll shows it.
func TestPostThroughThePinnedScript(t *testing.T) {
	f := synthetic(t)
	f.installScript()
	t.Setenv("MOOT_NAME", "")
	t.Setenv("HOME", filepath.Dir(f.folder))
	src := f.open()
	m := shown(t, src, WithPollInterval(time.Hour))
	before := snapshot(t, f.folder)
	m = openBarOn(t, m)
	m, _ = feed(m, typeText("hello from the bar")...)
	m, _ = feed(m, paste("\r\nsecond line\r\n"))
	m, cmd := feed(m, key(tea.KeyEnter))
	if cmd == nil || !m.bar.busy {
		t.Fatal("Enter should start the post")
	}
	msg := cmd()
	pm, ok := msg.(postMsg)
	if !ok || pm.err != nil {
		t.Fatalf("the post failed: %#v", msg)
	}
	if pm.res.Room != "dev" || pm.res.Sender != "atlas" || pm.res.ID == "" {
		t.Fatalf("answer: %+v", pm.res)
	}
	next, read := m.Update(msg)
	m = next.(Model)
	if m.bar != nil || !strings.Contains(statusRow(m), "posted to dev as atlas") {
		t.Fatalf("after the answer: bar %v status %q", m.bar, statusRow(m))
	}
	// The post is one new file with the text, ending in one newline.
	path := filepath.Join(f.folder, ".moot", "rooms", "dev", "posts", pm.res.ID+".md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello from the bar\nsecond line\n" {
		t.Fatalf("post file: %q", got)
	}
	known := map[string]bool{}
	for _, l := range lines(before) {
		if fs := strings.Fields(l); len(fs) > 0 {
			known[fs[0]] = true
		}
	}
	var added []string
	for _, l := range lines(snapshot(t, f.folder)) {
		if fs := strings.Fields(l); len(fs) > 0 && !known[fs[0]] {
			added = append(added, fs[0])
		}
	}
	if len(added) != 1 || added[0] != filepath.Join(".moot", "rooms", "dev", "posts", pm.res.ID+".md") {
		t.Fatalf("the post changed more than its own file: %v", added)
	}
	// The read the answer asked for shows the post.
	m = answer(t, m, read)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "hello from the bar") || !strings.Contains(v, "second line") || !strings.Contains(v, "atlas") {
		t.Fatalf("the post is not on screen:\n%s", v)
	}
	// So does the next poll, when the first read was lost.
	again := New(src, utc, WithPollInterval(time.Hour))
	again, _ = feed(again, tea.WindowSizeMsg{Width: 100, Height: 30})
	again = answer(t, again, again.Init())
	if !strings.Contains(ansi.Strip(again.View()), "second line") {
		t.Fatal("a fresh poll does not show the post")
	}
}

// A failing script leaves the text in the bar: here the room vanished while the bar
// was open (architecture, "Posting": no separate check).
func TestRoomVanishedWhileTheBarWasOpen(t *testing.T) {
	f := synthetic(t)
	f.installScript()
	m := shown(t, f.open())
	m = openBarOn(t, m)
	if err := os.RemoveAll(filepath.Join(f.folder, ".moot", "rooms", "dev")); err != nil {
		t.Fatal(err)
	}
	m, _ = feed(m, typeText("hello")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	m, read := feed(m, cmd())
	if read != nil || m.bar == nil || m.bar.busy || m.bar.text != "hello" {
		t.Fatalf("bar after the failure: %+v", m.bar)
	}
	if s := statusRow(m); !strings.Contains(s, "Could not post to dev:") {
		t.Fatalf("status row: %q", s)
	}
	if _, err := os.Stat(filepath.Join(f.folder, ".moot", "rooms", "dev")); err == nil {
		t.Fatal("the failed post made the room")
	}
}
