package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// stoppedHeld runs the held post to its cancelled answer after ctrl+c and returns the
// model that quit.
func stoppedHeld(t *testing.T, f *fixture, between func()) Model {
	t.Helper()
	h := newHold(f)
	m, answer := startHeldPost(t, h)
	m, _ = feed(m, key(tea.KeyCtrlC))
	select {
	case msg := <-answer:
		if between != nil {
			between()
		}
		next, cmd := m.Update(msg)
		if !quits(cmd) {
			t.Fatal("the answer should quit")
		}
		return next.(Model)
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
	return m
}

func nowStamp() string { return time.Now().UTC().Format("20060102-150405") }

// The script answered with success after ctrl+c: the line says the post went out, with
// the room and the id.
func TestExitLineSaysALandedPostWentOut(t *testing.T) {
	spy := newSpy(synthetic(t))
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return moot.PostResult{Room: "dev", ID: "20260801-100000-atlas", Sender: "atlas"}, nil
	})
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hello")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	answer := cmd()
	m, _ = feed(m, key(tea.KeyCtrlC))
	next, c := m.Update(answer)
	if !quits(c) {
		t.Fatal("should quit")
	}
	want := "easymoot: the post had already gone out to dev (20260801-100000-atlas)"
	if got := next.(Model).ExitLine(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The cancel came back and no post of this name appeared since Enter: nothing was
// posted. An older post of this name, and a new one from another sender, don't count.
func TestExitLineSaysNothingWasPostedWhenTheCancelCameBack(t *testing.T) {
	f := synthetic(t)
	f.post("dev", "20260801-100000-atlas", "older\n")
	got := stoppedHeld(t, f, func() { f.post("dev", "20990101-000000-grace", "someone else\n") }).ExitLine()
	if want := "easymoot: stopped; nothing was posted to dev"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A post from this name appeared in the room meanwhile: it landed after all, and the
// line gives the id from the listing.
func TestExitLineSaysItLandedWhenAPostAppearedMeanwhile(t *testing.T) {
	f := synthetic(t)
	id := nowStamp() + "-atlas"
	got := stoppedHeld(t, f, func() { f.post("dev", id, "it landed\n") }).ExitLine()
	if want := "easymoot: the post had already gone out to dev (" + id + ")"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The listing is read once, and nothing is written: the line costs one List.
func TestExitLineReadsTheRoomOnce(t *testing.T) {
	f := synthetic(t)
	spy := newSpy(f)
	h := &holdSource{spySource: spy, started: make(chan struct{}), cancelled: make(chan struct{})}
	m, answer := startHeldPost(t, h)
	m, _ = feed(m, key(tea.KeyCtrlC))
	next, _ := m.Update(<-answer)
	spy.take()
	_ = next.(Model).ExitLine()
	if calls := spy.take(); len(calls) != 1 || calls[0] != "List dev" {
		t.Fatalf("calls %v, want one List of dev", calls)
	}
}

// The bound passing, or a second ctrl+c, with no answer: it may or may not have gone out.
func TestExitLineAdmitsItDoesNotKnow(t *testing.T) {
	const want = "easymoot: stopped waiting; the post to dev may or may not have gone out"
	h := newHold(synthetic(t))
	m, _ := startHeldPost(t, h)
	m, _ = feed(m, key(tea.KeyCtrlC))

	bound, c := m.Update(quitMsg{src: m.src})
	if !quits(c) || bound.(Model).ExitLine() != want {
		t.Fatalf("bound: got %q", bound.(Model).ExitLine())
	}
	second, c := m.Update(key(tea.KeyCtrlC))
	if !quits(c) || second.(Model).ExitLine() != want {
		t.Fatalf("second ctrl+c: got %q", second.(Model).ExitLine())
	}
	// The poster's own time-out answering is the same admission.
	timedOut, _ := m.Update(postMsg{src: m.src, room: "dev", err: moot.ErrTimeout})
	if got := timedOut.(Model).ExitLine(); got != want {
		t.Fatalf("time-out: got %q", got)
	}
}

// Any other quit says nothing.
func TestExitLineIsEmptyOnEveryOtherQuit(t *testing.T) {
	f := synthetic(t)
	idle := shown(t, newSpy(f))
	if next, c := idle.Update(key(tea.KeyCtrlC)); !quits(c) || next.(Model).ExitLine() != "" {
		t.Fatal("ctrl+c in the idle shell should quit silently")
	}
	bar := openBarOn(t, shown(t, newSpy(f)))
	if next, c := bar.Update(key(tea.KeyCtrlC)); !quits(c) || next.(Model).ExitLine() != "" {
		t.Fatal("ctrl+c with the bar open and idle should quit silently")
	}
	// A post that finished before any ctrl+c.
	spy := newSpy(f)
	spy.setPoster(func(room, text string) (moot.PostResult, error) { return moot.PostResult{Room: "dev"}, nil })
	m := openBarOn(t, shown(t, spy))
	m, _ = feed(m, typeText("hi")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	m = answer(t, m, cmd)
	if next, c := m.Update(key(tea.KeyCtrlC)); !quits(c) || next.(Model).ExitLine() != "" {
		t.Fatal("ctrl+c after a finished post should quit silently")
	}
	// A failed post, then the palette's Quit.
	spy.setPoster(func(room, text string) (moot.PostResult, error) { return moot.PostResult{}, errors.New("no") })
	m = openBarOn(t, m)
	m, _ = feed(m, typeText("hi")...)
	m, cmd = feed(m, key(tea.KeyEnter))
	m = answer(t, m, cmd)
	m, _ = feed(m, key(tea.KeyEsc)) // the failed post left the bar open
	m, cmd = feed(m, append(typeText(":quit"), key(tea.KeyEnter))...)
	if !quits(cmd) || m.ExitLine() != "" {
		t.Fatal("the palette's Quit should be silent")
	}
}
