package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// fixture is a synthetic moot under t.TempDir(): <root>/<folder>/.moot/. Nothing
// here reads or runs anything from a real moot.
type fixture struct {
	t      *testing.T
	folder string
}

func newFixture(t *testing.T, folderName string) *fixture {
	t.Helper()
	// An empty MOOT_NAME is skipped by the name chain, so the developer's own
	// environment never reaches the header.
	t.Setenv("MOOT_NAME", "")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, folder: filepath.Join(root, folderName)}
	f.mkdir(".moot/rooms")
	return f
}

func (f *fixture) mkdir(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Join(f.folder, rel), 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) write(rel, text string) {
	f.t.Helper()
	p := filepath.Join(f.folder, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// installScript puts a copy of the pinned moot.sh into the fixture's .moot/, the only
// way a test reaches a real script: never one from a real moot.
func (f *fixture) installScript() {
	f.t.Helper()
	script, err := os.ReadFile("../moot/testdata/upstream-c25b5bb/moot.sh")
	if err != nil {
		f.t.Fatal(err)
	}
	p := filepath.Join(f.folder, ".moot", "moot.sh")
	if err := os.WriteFile(p, script, 0o755); err != nil {
		f.t.Fatal(err)
	}
}

// room makes rooms/<name>/posts.
func (f *fixture) room(name string) { f.t.Helper(); f.mkdir(".moot/rooms/" + name + "/posts") }

// post writes rooms/<room>/posts/<id>.md.
func (f *fixture) post(room, id, text string) {
	f.t.Helper()
	f.write(".moot/rooms/"+room+"/posts/"+id+".md", text)
}

func (f *fixture) open() *moot.Moot {
	f.t.Helper()
	m, err := moot.Open(f.folder)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

// spySource passes every call to the moot and records it, so a test can say what
// the shell reads, and that Update and View read nothing.
type spySource struct {
	*moot.Moot
	mu     sync.Mutex
	calls  []string
	poster func(room, text string) (moot.PostResult, error)
}

func (s *spySource) record(c string) { s.mu.Lock(); s.calls = append(s.calls, c); s.mu.Unlock() }
func (s *spySource) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.calls
	s.calls = nil
	return c
}

func (s *spySource) Rooms() ([]moot.Room, error) { s.record("Rooms"); return s.Moot.Rooms() }
func (s *spySource) List(room string) ([]moot.Post, error) {
	s.record("List " + room)
	return s.Moot.List(room)
}
func (s *spySource) PostsModTime(room string) (time.Time, error) {
	s.record("Stat " + room)
	return s.Moot.PostsModTime(room)
}
func (s *spySource) Body(room, id string) (moot.Body, error) {
	s.record("Body " + room + " " + id)
	return s.Moot.Body(room, id)
}
func (s *spySource) Name() (string, error) { s.record("Name"); return s.Moot.Name() }

// Post records the call and answers from the spy's own poster, which says posted
// unless a test sets another. It never runs a script: the real one is reached only by
// a test that opens the *moot.Moot itself (TestPostThroughThePinnedScript).
func (s *spySource) Post(_ context.Context, room, text string, _ ...moot.PostOption) (moot.PostResult, error) {
	s.record("Post " + room + " " + strconv.Quote(text))
	s.mu.Lock()
	poster := s.poster
	s.mu.Unlock()
	if poster != nil {
		return poster(room, text)
	}
	return moot.PostResult{Room: room, ID: "20260801-120000-spy", Sender: "spy"}, nil
}

func (s *spySource) setPoster(p func(room, text string) (moot.PostResult, error)) {
	s.mu.Lock()
	s.poster = p
	s.mu.Unlock()
}

func newSpy(f *fixture) *spySource { return &spySource{Moot: f.open()} }

// utc keeps the headings the same on every machine.
var utc = WithLocation(time.UTC)

// shown is a model at 100x30 over src, with its first read applied. Poll ticks are
// so slow that none arrives unless a test asks.
func shown(t *testing.T, src Source, opts ...Option) Model {
	t.Helper()
	m := New(src, append([]Option{utc, WithPollInterval(time.Hour)}, opts...)...)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	return answer(t, m, m.Init())
}

// answer runs cmd, which has to be a read, and feeds the answer to the model.
func answer(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

// flatten runs cmd and returns the messages it produces, expanding batches. A
// command that blocks (a tick) is not run unless run says so.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, flatten(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func feed(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func typeText(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return out
}

// openBarOn runs the palette's Post by the keys that reach it: a colon, the name's
// letters, Enter. It fails the test if the bar did not open.
func openBarOn(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := feed(m, append(typeText(":post"), key(tea.KeyEnter))...)
	if cmd != nil {
		t.Fatalf("opening the bar scheduled a command")
	}
	if m.bar == nil {
		t.Fatalf("the bar did not open; status %q", m.notice)
	}
	return m
}

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

// quits reports whether running cmd produces a quit, expanding batches. It runs the
// command, so only call it on one that does not block.
func quits(cmd tea.Cmd) bool {
	for _, msg := range flatten(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

// screen is the frame as plain text.
func screen(m Model) string { return m.View() }

func lines(s string) []string { return strings.Split(s, "\n") }

func contextCancelled() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, cancel
}
