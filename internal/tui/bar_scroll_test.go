package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// scrolledUpBar is a model over the long moot's dev room, scrolled up two pages, with
// the bar open and "my new post" typed and sent. It returns the model, the poster's
// answer as a command, and the scroll state to compare against.
func scrolledUpBar(t *testing.T, src Source) (Model, tea.Cmd) {
	t.Helper()
	m := shown(t, src)
	m, _ = feed(m, key(tea.KeyPgUp), key(tea.KeyPgUp))
	if m.follow || m.anchorID == "" {
		t.Fatal("not scrolled up")
	}
	m = openBarOn(t, m)
	m, _ = feed(m, typeText("my new post")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	if cmd == nil || m.bar == nil || !m.bar.busy {
		t.Fatal("Enter should start the post")
	}
	return m, cmd
}

func viewHolds(t *testing.T, m Model, want string, present bool) {
	t.Helper()
	if got := strings.Contains(ansi.Strip(m.View()), want); got != present {
		t.Fatalf("view contains %q = %v, want %v:\n%s", want, got, present, ansi.Strip(m.View()))
	}
}

// A success from a scrolled-up view returns the pane to following the end, so the new
// post is in view once the read lands.
func TestSuccessFollowsTheEndAndShowsTheNewPost(t *testing.T) {
	f := longMoot(t)
	spy := newSpy(f)
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		f.post(room, "20260802-120000-spy", text+"\n")
		return moot.PostResult{Room: room, ID: "20260802-120000-spy", Sender: "spy"}, nil
	})
	m, cmd := scrolledUpBar(t, spy)
	viewHolds(t, m, "post 39", false)

	m, read := feed(m, cmd())
	if !m.follow || m.anchorID != "" || m.top != m.maxTop() {
		t.Fatalf("not following after the success: follow %v anchor %q top %d of %d", m.follow, m.anchorID, m.top, m.maxTop())
	}
	if read == nil {
		t.Fatal("the success should read the room")
	}
	m = answer(t, m, read)
	if !m.follow || m.top != m.maxTop() {
		t.Fatalf("the read let go of the end: follow %v top %d of %d", m.follow, m.top, m.maxTop())
	}
	viewHolds(t, m, "my new post", true)
	viewHolds(t, m, "post 39", true)
}

// The same through the pinned script over a synthetic moot.
func TestSuccessFollowsTheEndThroughThePinnedScript(t *testing.T) {
	f := longMoot(t)
	f.installScript()
	t.Setenv("MOOT_NAME", "")
	t.Setenv("HOME", filepath.Dir(f.folder))
	m, cmd := scrolledUpBar(t, f.open())
	msg := cmd()
	if pm, ok := msg.(postMsg); !ok || pm.err != nil {
		t.Fatalf("the post failed: %#v", msg)
	}
	m, read := feed(m, msg)
	if !m.follow {
		t.Fatal("not following after the success")
	}
	m = answer(t, m, read)
	if !m.follow || m.top != m.maxTop() {
		t.Fatalf("follow %v top %d of %d", m.follow, m.top, m.maxTop())
	}
	viewHolds(t, m, "my new post", true)
}

// A failure or a time-out leaves a scrolled-up view where it was.
func TestFailureAndTimeoutLeaveTheScrolledViewAlone(t *testing.T) {
	for name, err := range map[string]error{
		"failure": &moot.ScriptError{Message: "no such room", ExitCode: 1},
		"timeout": moot.ErrTimeout,
	} {
		t.Run(name, func(t *testing.T) {
			spy := newSpy(longMoot(t))
			spy.setPoster(func(room, text string) (moot.PostResult, error) { return moot.PostResult{}, err })
			m, cmd := scrolledUpBar(t, spy)
			top, anchor, off := m.top, m.anchorID, m.anchorOff
			rows := ansi.Strip(m.View())

			m, read := feed(m, cmd())
			if read != nil {
				m = answer(t, m, read) // a time-out reads the room too
			}
			if m.follow || m.top != top || m.anchorID != anchor || m.anchorOff != off {
				t.Fatalf("the view moved: top %d->%d anchor %q+%d -> %q+%d follow %v",
					top, m.top, anchor, off, m.anchorID, m.anchorOff, m.follow)
			}
			// Only the bar and status rows differ; the posts pane shows the same lines.
			a, b := lines(rows), lines(ansi.Strip(m.View()))
			if strings.Join(a[2:len(a)-2], "\n") != strings.Join(b[2:len(b)-2], "\n") {
				t.Fatal("the posts pane shows other lines than before")
			}
		})
	}
}

// An answer naming a room other than the one on show leaves the view alone: the post
// did not go to the room being read.
func TestSuccessInAnotherRoomLeavesTheViewAlone(t *testing.T) {
	spy := newSpy(longMoot(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{Room: "lobby", ID: "20260802-120000-spy", Sender: "spy"}, nil
	})
	m, cmd := scrolledUpBar(t, spy)
	top, anchor := m.top, m.anchorID
	m, _ = feed(m, cmd())
	if m.follow || m.top != top || m.anchorID != anchor {
		t.Fatalf("the view moved: follow %v top %d->%d", m.follow, top, m.top)
	}
}

// A success while the pane already follows keeps following (the end key's state).
func TestSuccessWhileFollowingKeepsFollowing(t *testing.T) {
	spy := newSpy(longMoot(t))
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hi")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	m, _ = feed(m, cmd())
	if !m.follow || m.top != m.maxTop() {
		t.Fatalf("follow %v top %d of %d", m.follow, m.top, m.maxTop())
	}
}
