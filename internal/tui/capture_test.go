package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// synthetic builds the moot the captures and several tests look at: three rooms,
// posts from four senders with a mention, a list, a sign-off and code.
func synthetic(t *testing.T) *fixture {
	f := newFixture(t, "atlas")
	f.room("dev")
	f.room("lobby")
	f.room("runs")
	f.post("dev", "20260801-090000-ada", "Starting on the index rebuild.\nI will take `build.go` first.\n— ada\n")
	f.post("dev", "20260801-091500-grace", "@ada the **rebuild** is flaky on the second pass.\n\n- the cache is not cleared\n- the lock file lingers\n\nI will look at read_new_ids next; @ci_bot_2 please hold the nightly.\n— grace\n")
	f.post("dev", "20260801-093000-ada", "Thanks. Pushed a fix:\n\n```go\nfunc clear() { cache = nil }\n```\n\n— ada\n")
	f.post("dev", "20260802-080000-linus", "Nightly is green.\nIndex is 40% smaller.\n— linus\n")
	f.post("runs", "20260802-081000-ci-bot", "run 412 passed\nrun 413 passed\n")
	return f
}

// longMoot is a moot whose dev room is longer than any pane: forty posts of two
// lines, oldest "post 00". The other rooms are short.
func longMoot(t *testing.T) *fixture {
	f := newFixture(t, "long")
	f.room("dev")
	f.room("lobby")
	f.room("runs")
	for i := 0; i < 40; i++ {
		f.post("dev", fmt.Sprintf("20260801-09%04d-ada", i), fmt.Sprintf("post %02d\nsecond line of %02d\n", i, i))
	}
	f.post("lobby", "20260801-100000-grace", "hello lobby\n")
	f.post("runs", "20260801-110000-linus", "hello runs\n")
	return f
}

// TestCapture writes the terminal captures of the synthetic moot, colour off, at a
// fixed size, when EASYMOOT_CAPTURE_DIR names a directory. It is how the captures in
// the stitch directory were made, and it runs as an ordinary render test otherwise.
func TestCapture(t *testing.T) {
	f := synthetic(t)
	// The header shows the folder with ~ for the home directory; make the fixture's
	// root that, so the capture says ~/atlas and not a temporary path.
	t.Setenv("HOME", filepath.Dir(f.folder))
	m := shown(t, f.open())
	out := os.Getenv("EASYMOOT_CAPTURE_DIR")
	write := func(name string, m Model) {
		t.Helper()
		v := m.View()
		if out == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(out, name), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("capture-100x30.txt", m)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 56, Height: 16})
	write("capture-56x16-single-pane.txt", next.(Model))
	next, _ = m.Update(key2(paletteKey))
	write("capture-100x30-palette.txt", next.(Model))

	// The entry bar: open and empty, with typed text and a pasted block (its line
	// breaks drawn as ↵), while the post runs, and after it was posted. The poster is
	// the spy's, so nothing is written.
	spy := &spySource{Moot: f.open()}
	b := shown(t, spy)
	b = openBarOn(t, b)
	write("capture-100x30-bar.txt", b)
	b, _ = feed(b, typeText("The index rebuild is done; notes follow:")...)
	b, _ = feed(b, paste("\r\n- cache cleared\r\n- lock file removed\r\n"))
	write("capture-100x30-bar-text.txt", b)
	b, cmd := feed(b, key(tea.KeyEnter))
	write("capture-100x30-bar-posting.txt", b)
	b, _ = feed(b, cmd())
	write("capture-100x30-bar-posted.txt", b)
	b = openBarOn(t, b)
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{}, &moot.ScriptError{Message: "room 'dev' has no posts/ directory", ExitCode: 1}
	})
	b, _ = feed(b, typeText("this one fails")...)
	b, cmd = feed(b, key(tea.KeyEnter))
	b, _ = feed(b, cmd())
	write("capture-100x30-bar-failed.txt", b)
}

func key2(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
