package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// poll runs one tick of the poll chain over the model and applies the read's answer.
func poll(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := feed(m, tickMsg{src: m.src})
	return answer(t, m, cmd)
}

// marked is the rooms the sidebar shows with the unread bullet.
func marked(m Model) []string {
	var out []string
	for _, l := range lines(ansi.Strip(m.View())) {
		if i := strings.Index(l, "• "); i >= 0 {
			out = append(out, strings.Fields(l[i+len("• "):])[0])
		}
	}
	return out
}

func wantMarked(t *testing.T, m Model, want ...string) {
	t.Helper()
	got := marked(m)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rooms marked unread = %v, want %v\n%s", got, want, ansi.Strip(m.View()))
	}
}

// Posts already there at launch count as read, in every room.
func TestLaunchShowsNoUnreadMarks(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	wantMarked(t, m)
	wantMarked(t, poll(t, m))
}

// A post landing in an unselected room marks it, and only it; selecting it clears
// the mark at once, and it stays clear.
func TestAPostInAnUnselectedRoomMarksItAndSelectingClearsIt(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	if m.room != "dev" {
		t.Fatalf("selected %q, want dev", m.room)
	}
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	wantMarked(t, m) // not before a poll
	m = poll(t, m)
	wantMarked(t, m, "runs")
	wantMarked(t, poll(t, m), "runs") // the mark stays while it is unselected

	m, cmd := feed(m, key(tea.KeyDown), key(tea.KeyDown)) // lobby, then runs
	if m.room != "runs" {
		t.Fatalf("selected %q, want runs", m.room)
	}
	wantMarked(t, m) // gone before the read comes back
	m = answer(t, m, cmd)
	wantMarked(t, m)
	wantMarked(t, poll(t, m))

	// Leaving it again does not bring the mark back.
	m, _ = feed(m, key(tea.KeyUp))
	wantMarked(t, m)
	wantMarked(t, poll(t, m))
}

// The selected room never takes the mark, whether the post lands while it is
// selected or it is left afterwards.
func TestTheSelectedRoomNeverTakesTheMark(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	f.post("dev", "20260802-090000-grace", "Landing while selected.\n")
	m = poll(t, m)
	wantMarked(t, m)
	if !strings.Contains(ansi.Strip(m.View()), "Landing while selected.") {
		t.Fatal("the new post is not shown")
	}
	m, _ = feed(m, key(tea.KeyDown)) // leave dev, which the poll had seen
	wantMarked(t, m)
	wantMarked(t, poll(t, m))
}

// A post that arrives in an unselected room and is then deleted leaves no mark.
func TestADeletedPostLeavesNoStuckMark(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	f.post("lobby", "20260802-090000-ada", "hi\n")
	m = poll(t, m)
	wantMarked(t, m, "lobby")
	if err := os.Remove(filepath.Join(f.folder, ".moot/rooms/lobby/posts/20260802-090000-ada.md")); err != nil {
		t.Fatal(err)
	}
	wantMarked(t, poll(t, m))
	// And a later post marks it again.
	f.post("lobby", "20260802-100000-ada", "hi again\n")
	wantMarked(t, poll(t, m), "lobby")
}

// A room that goes away and comes back starts read, and a mark it held goes with it.
func TestARoomThatGoesAwayAndComesBackStartsRead(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	m = poll(t, m)
	wantMarked(t, m, "runs")

	room := filepath.Join(f.folder, ".moot/rooms/runs")
	if err := os.Rename(room, room+".away"); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	wantMarked(t, m)
	if err := os.Rename(room+".away", room); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	wantMarked(t, m)
	// Posts after it came back mark it as usual.
	f.post("runs", "20260802-100000-ci-bot", "run 415 passed\n")
	wantMarked(t, poll(t, m), "runs")
}

// When the selected room goes away, its neighbour is selected and does not take a
// mark it had held: not in the model, where the drawn row would hide it, and not
// after leaving it before the next chained poll either.
func TestTheNeighbourSelectedAfterARoomGoesIsNotMarked(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	f.post("lobby", "20260802-090000-ada", "hi\n")
	m = poll(t, m)
	wantMarked(t, m, "lobby")
	if err := os.RemoveAll(filepath.Join(f.folder, ".moot/rooms/dev")); err != nil {
		t.Fatal(err)
	}
	m = poll(t, m)
	if m.room != "lobby" {
		t.Fatalf("selected %q, want lobby", m.room)
	}
	wantMarked(t, m)
	if m.unread["lobby"] {
		t.Fatal("the model holds a mark for the selected neighbour")
	}
	m = answer(t, m, m.readSelected()) // the neighbour's read comes back
	m, _ = feed(m, key(tea.KeyDown))   // leave it before the next chained poll
	if m.room != "runs" || m.unread["lobby"] {
		t.Fatalf("after leaving: selected %q, lobby marked %v", m.room, m.unread["lobby"])
	}
	wantMarked(t, m)
	wantMarked(t, poll(t, m))
}

// Bob's own post, shown by the read that follows it, does not mark its room when he
// leaves it before the next chained poll; nor does a post he watched arrive through
// a read a key asked for.
func TestAPostBobSawDoesNotMarkTheRoomHeLeaves(t *testing.T) {
	for _, c := range []struct {
		name string
		read func(Model) tea.Cmd
	}{
		{"read after a post", func(m Model) tea.Cmd { return m.readAfterPost(m.room) }},
		{"read a key asked for", func(m Model) tea.Cmd { return m.readSelected() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := synthetic(t)
			m := shown(t, newSpy(f))
			f.post("dev", "20260802-090000-bob", "my own post\n")
			m = answer(t, m, c.read(m))
			if !strings.Contains(ansi.Strip(m.View()), "my own post") {
				t.Fatal("the new post is not shown")
			}
			m, _ = feed(m, key(tea.KeyDown)) // leave dev before any chained poll
			if m.room != "lobby" || m.unread["dev"] {
				t.Fatalf("after leaving: selected %q, dev marked %v", m.room, m.unread["dev"])
			}
			wantMarked(t, m)
			wantMarked(t, poll(t, m))
			wantMarked(t, poll(t, m))
		})
	}
}

// A room is unread when a post was added that Bob has not seen, whatever its id
// sorts as: one that sorts after every stamp (a README in posts/) and one that sorts
// before the newest (a late sync).
func TestAPostThatDoesNotSortLastStillMarksTheRoom(t *testing.T) {
	t.Run("a README in posts", func(t *testing.T) {
		f := synthetic(t)
		f.write(".moot/rooms/runs/posts/README.md", "About this room.\n")
		m := shown(t, newSpy(f))
		wantMarked(t, m)
		f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
		wantMarked(t, poll(t, m), "runs")
	})
	t.Run("a late-synced post", func(t *testing.T) {
		f := synthetic(t)
		m := shown(t, newSpy(f))
		f.post("runs", "20260801-120000-ci-bot", "run 411 passed, synced late\n") // before 20260802-081000
		wantMarked(t, poll(t, m), "runs")
	})
	t.Run("then deleted, no stuck mark", func(t *testing.T) {
		f := synthetic(t)
		m := shown(t, newSpy(f))
		f.post("runs", "20260801-120000-ci-bot", "late\n")
		m = poll(t, m)
		wantMarked(t, m, "runs")
		if err := os.Remove(filepath.Join(f.folder, ".moot/rooms/runs/posts/20260801-120000-ci-bot.md")); err != nil {
			t.Fatal(err)
		}
		wantMarked(t, poll(t, m))
	})
}

// age sets a room's posts/ directory stamp an hour back, as a room nothing has
// touched lately has it.
func age(t *testing.T, f *fixture, room string) {
	t.Helper()
	back := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(f.folder, ".moot/rooms", room, "posts"), back, back); err != nil {
		t.Fatal(err)
	}
}

func listed(calls []string, room string) bool {
	for _, c := range calls {
		if c == "List "+room {
			return true
		}
	}
	return false
}

// A room whose posts/ has not changed since it was listed is not listed again: one
// stat per room per poll, and a listing only for the room that changed.
func TestAnUnchangedRoomIsNotListedAgain(t *testing.T) {
	f := synthetic(t)
	age(t, f, "lobby")
	age(t, f, "runs")
	spy := newSpy(f)
	m := shown(t, spy)
	m = poll(t, m) // the first polls list each room once and note its stamp
	spy.take()
	m = poll(t, m)
	calls := spy.take()
	if err := allowedRead(calls, "dev"); err != nil {
		t.Fatal(err)
	}
	if listed(calls, "lobby") || listed(calls, "runs") {
		t.Fatalf("an unchanged room was listed again: %v", calls)
	}
	if got := strings.Join(calls, "|"); !strings.Contains(got, "Stat lobby") || !strings.Contains(got, "Stat runs") {
		t.Fatalf("the poll did not stat the other rooms: %v", calls)
	}

	// A change lists that room and no other, and marks it.
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	spy.take()
	m = poll(t, m)
	calls = spy.take()
	if !listed(calls, "runs") || listed(calls, "lobby") {
		t.Fatalf("after a post to runs the poll listed: %v", calls)
	}
	wantMarked(t, m, "runs")
	// The room changed a moment ago is listed each poll until its stamp is old enough
	// to trust (a coarse file system can stamp two posts alike).
	spy.take()
	m = poll(t, m)
	if calls = spy.take(); !listed(calls, "runs") {
		t.Fatalf("a room changed just now was not listed: %v", calls)
	}
	wantMarked(t, m, "runs")
}

// The marks come from listings alone: a poll lists the other rooms (names only), and
// a read a key asks for lists nothing else. Since 2026-11-27 (glance-plain) a chained
// poll reads a post that arrived in another room once, to see whether it mentions the
// reader, and never again.
func TestMarksReadNoBodyOfAnotherRoom(t *testing.T) {
	f := synthetic(t)
	spy := newSpy(f)
	m := shown(t, spy)
	spy.take()
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	m = poll(t, m)
	calls := spy.take()
	var rest []string
	once := 0
	for _, c := range calls {
		if c == "Body runs 20260802-090000-ci-bot" {
			once++
			continue
		}
		rest = append(rest, c)
	}
	if err := allowedRead(rest, "dev"); err != nil || once > 1 {
		t.Fatalf("%v; the new post in runs read %d times", err, once)
	}
	if got := strings.Join(calls, "|"); !strings.Contains(got, "List lobby") || !strings.Contains(got, "List runs") {
		t.Fatalf("the poll did not list the other rooms: %v", calls)
	}
	m = poll(t, m)
	if err := allowedRead(spy.take(), "dev"); err != nil {
		t.Fatalf("a post already checked was read again: %v", err)
	}
	m, cmd := feed(m, key(tea.KeyDown))
	spy.take()
	answer(t, m, cmd)
	for _, c := range spy.take() {
		if strings.HasPrefix(c, "List ") && c != "List lobby" {
			t.Fatalf("a read from a key listed %q", c)
		}
	}
}
