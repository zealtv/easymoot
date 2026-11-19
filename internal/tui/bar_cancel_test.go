package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// holdSource is a Source whose Post blocks until its context ends, as a slow script
// does, and records that it was cancelled.
type holdSource struct {
	*spySource
	started   chan struct{}
	cancelled chan struct{}
}

func newHold(f *fixture) *holdSource {
	return &holdSource{spySource: newSpy(f), started: make(chan struct{}), cancelled: make(chan struct{})}
}

func (h *holdSource) Post(ctx context.Context, room, text string, _ ...moot.PostOption) (moot.PostResult, error) {
	close(h.started)
	<-ctx.Done()
	close(h.cancelled)
	return moot.PostResult{}, ctx.Err()
}

func startHeldPost(t *testing.T, h *holdSource) (Model, chan tea.Msg) {
	t.Helper()
	m := openBarOn(t, shown(t, h))
	m, _ = feed(m, typeText("hello")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	if cmd == nil || !m.bar.busy {
		t.Fatal("Enter should start the post")
	}
	answer := make(chan tea.Msg, 1)
	go func() { answer <- cmd() }()
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the post never started")
	}
	return m, answer
}

// ctrl+c on a busy bar cancels the post and holds the quit until the post answers.
func TestCtrlCWhilePostingCancelsThePostThenQuits(t *testing.T) {
	h := newHold(synthetic(t))
	m, answer := startHeldPost(t, h)

	m, cmd := feed(m, key(tea.KeyCtrlC))
	if cmd == nil || m.bar == nil || !m.bar.quitting {
		t.Fatalf("ctrl+c should start the stop and keep the bar: bar %+v", m.bar)
	}
	select {
	case <-h.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("ctrl+c did not cancel the post's context")
	}
	if got := lastRow(m); !strings.Contains(got, "stopping…") {
		t.Fatalf("bar row while stopping: %q", got)
	}
	// Keys do nothing while it stops, but ctrl+c again quits at once.
	if _, c := feed(m, typeText("x")...); c != nil {
		t.Fatal("a key scheduled a command while stopping")
	}
	if _, c := m.Update(key(tea.KeyCtrlC)); !quits(c) {
		t.Fatal("a second ctrl+c should quit at once")
	}
	// The answer (cancelled) ends the program.
	var msg tea.Msg
	select {
	case msg = <-answer:
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
	if pm, ok := msg.(postMsg); !ok || pm.err == nil {
		t.Fatalf("answer %#v, want a cancelled post", msg)
	}
	if _, c := m.Update(msg); !quits(c) {
		t.Fatal("the post's answer should quit the program")
	}
}

// If the answer never comes, the bound's message still quits; a stray one does not.
func TestCtrlCBoundQuitsWithoutAnAnswer(t *testing.T) {
	h := newHold(synthetic(t))
	m, _ := startHeldPost(t, h)
	idle := openBarOn(t, shown(t, newSpy(synthetic(t))))

	if _, c := idle.Update(quitMsg{src: idle.src}); c != nil {
		t.Fatal("a quitMsg with no stop under way should do nothing")
	}
	m, _ = feed(m, key(tea.KeyCtrlC))
	if _, c := m.Update(quitMsg{src: m.src}); !quits(c) {
		t.Fatal("the bound should quit")
	}
	if _, c := m.Update(quitMsg{src: idle.src}); c != nil {
		t.Fatal("a quitMsg for another folder should do nothing")
	}
}

// An answer that says the post landed still quits when ctrl+c asked: the post stands.
func TestCtrlCAfterTheScriptLandedStillQuits(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hello")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	answer := cmd() // the post has landed, but Update has not seen the answer
	m, _ = feed(m, key(tea.KeyCtrlC))
	if _, c := m.Update(answer); !quits(c) {
		t.Fatal("the landed post's answer should quit")
	}
}

// The real path: a script held in a read, the pinned script behind it. ctrl+c kills the
// held script before it runs the pinned one, so no post lands, nothing is left in the
// room, and the program quits.
func TestCtrlCWhilePostingLeavesNothingInTheRoom(t *testing.T) {
	f := synthetic(t)
	f.installScript()
	t.Setenv("HOME", filepath.Dir(f.folder))
	moots := filepath.Join(f.folder, ".moot")
	if err := os.Rename(filepath.Join(moots, "moot.sh"), filepath.Join(moots, "moot-pinned.sh")); err != nil {
		t.Fatal(err)
	}
	ctl := t.TempDir() // outside the folder: the room must show no change
	hold := filepath.Join(ctl, "hold")
	if err := syscall.Mkfifo(hold, 0o600); err != nil {
		t.Skipf("no fifo here: %v", err)
	}
	wrapper := "#!/usr/bin/env bash\necho $$ > '" + filepath.Join(ctl, "pid") + "'\n" +
		"read -t 60 <> '" + hold + "'\nexec \"$(dirname \"$0\")/moot-pinned.sh\" \"$@\"\n"
	f.write(".moot/moot.sh", wrapper)
	if err := os.Chmod(filepath.Join(moots, "moot.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	src := f.open()
	m := shown(t, src)
	before := snapshot(t, f.folder)
	m = openBarOn(t, m)
	m, _ = feed(m, typeText("never posted")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	answer := make(chan tea.Msg, 1)
	go func() { answer <- cmd() }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(ctl, "pid")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the script never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	m, _ = feed(m, key(tea.KeyCtrlC))
	var msg tea.Msg
	select {
	case msg = <-answer:
	case <-time.After(10 * time.Second):
		t.Fatal("the post never answered after ctrl+c")
	}
	if pm, ok := msg.(postMsg); !ok || pm.err == nil {
		t.Fatalf("answer %#v, want a cancelled post", msg)
	}
	if _, c := m.Update(msg); !quits(c) {
		t.Fatal("the program should quit once the post stopped")
	}
	if after := snapshot(t, f.folder); after != before {
		t.Fatalf("the room changed:\nbefore\n%s\nafter\n%s", before, after)
	}
}
