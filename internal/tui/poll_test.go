package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// advance runs the command a message left and feeds back what it produces, once.
func advance(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	msgs := flatten(cmd)
	var last tea.Cmd
	for _, msg := range msgs {
		m, last = feed(m, msg)
	}
	return m, last
}

// A post written after the first read shows up within one poll: the poll interval
// passes (a tick), the tick starts a read, and the read's answer shows the post.
// Only the new post's body is read; the old ones come from the model.
func TestANewPostAppearsWithinOnePoll(t *testing.T) {
	f := synthetic(t)
	spy := newSpy(f)
	m := New(spy, utc, WithPollInterval(time.Millisecond))
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, next := advance(t, m, m.Init()) // the first read; it chains the first tick
	if next == nil || !strings.Contains(ansi.Strip(m.View()), "Nightly is green.") {
		t.Fatal("the first read did not show the room, or did not start the poll")
	}
	spy.take()

	f.post("dev", "20260802-090000-grace", "Fresh post that arrived later.\n")
	if strings.Contains(ansi.Strip(m.View()), "Fresh post") {
		t.Fatal("the post showed before any read")
	}
	m, next = advance(t, m, next) // the tick: starts the read
	if next == nil {
		t.Fatal("the tick started no read")
	}
	m, next = advance(t, m, next) // the read's answer
	if !strings.Contains(ansi.Strip(m.View()), "Fresh post that arrived later.") {
		t.Fatalf("the new post is not shown after one poll:\n%s", m.View())
	}
	if next == nil {
		t.Fatal("the poll chain stopped")
	}
	calls := spy.take()
	if err := allowedRead(calls, "dev"); err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, c := range calls {
		if strings.HasPrefix(c, "Body ") {
			bodies = append(bodies, c)
		}
	}
	if want := []string{"Body dev 20260802-090000-grace"}; fmt.Sprint(bodies) != fmt.Sprint(want) {
		t.Fatalf("bodies read = %v, want only the new post's %v", bodies, want)
	}
}

// The poll loop is one chain: a read a key asked for is not chained, and one that
// arrives after the room changed is dropped but still passes the chain on.
func TestReadsFromKeysDoNotChainAndStaleAnswersAreDropped(t *testing.T) {
	f := longMoot(t)
	m := shown(t, f.open(), WithPollInterval(time.Millisecond))
	devPosts := len(m.posts)

	// Select lobby: the read it asks for is not part of the chain.
	m, askLobby := feed2(m, key(tea.KeyDown))
	if m.room != "lobby" {
		t.Fatal("did not select lobby")
	}
	lobby := askLobby().(pollMsg)
	// Before it answers, go back to dev.
	m, askDev := feed2(m, key(tea.KeyUp))
	if m.room != "dev" {
		t.Fatal("did not go back to dev")
	}
	// The lobby answer arrives late: nothing of it is shown.
	m, cmd := feed(m, lobby)
	if cmd != nil {
		t.Fatal("an unchained answer started a poll")
	}
	if m.room != "dev" || len(m.posts) != 0 || m.read {
		t.Fatalf("a stale answer was applied: room %q posts %d", m.room, len(m.posts))
	}
	// The dev answer fills the pane.
	m, cmd = feed(m, askDev())
	if cmd != nil || len(m.posts) != devPosts {
		t.Fatalf("dev answer: posts %d, cmd %v", len(m.posts), cmd)
	}
	// A chained answer that is stale still re-arms the tick.
	stale := lobby
	stale.chain = true
	if _, cmd := feed(m, stale); cmd == nil {
		t.Fatal("a stale chained answer broke the poll loop")
	} else if _, ok := cmd().(tickMsg); !ok {
		t.Fatalf("a stale chained answer scheduled %T, want a tick", cmd())
	}
	// An answer for another folder is dropped whole, and starts nothing.
	other := newSpy(synthetic(t))
	foreign := lobby
	foreign.src, foreign.asked, foreign.chain = other, "dev", true
	if next, cmd := feed(m, foreign); cmd != nil || len(next.posts) != devPosts {
		t.Fatal("an answer for another folder was used")
	}
	if _, cmd := feed(m, tickMsg{src: other}); cmd != nil {
		t.Fatal("a tick for another folder started a read")
	}
}

// feed2 sends one message and returns the model with the command it left.
func feed2(m Model, msg tea.Msg) (Model, tea.Cmd) { return feed(m, msg) }

// The selected room vanishes (a link removed while Bob watched): the neighbour is
// selected and read, and nothing is left showing the dead room.
func TestARoomThatVanishesIsReplacedByItsNeighbour(t *testing.T) {
	f := synthetic(t) // dev, lobby, runs
	m := shown(t, f.open())
	m, cmd := feed(m, key(tea.KeyDown)) // lobby
	m = answer(t, m, cmd)
	if m.room != "lobby" {
		t.Fatalf("room %q", m.room)
	}
	if err := os.RemoveAll(filepath.Join(f.folder, ".moot/rooms/lobby")); err != nil {
		t.Fatal(err)
	}
	m, next := advance(t, m, tickPoll(m))
	if m.room != "runs" || m.sel != 1 {
		t.Fatalf("room %q sel %d, want runs 1", m.room, m.sel)
	}
	if next == nil {
		t.Fatal("the neighbour was not read")
	}
	m, _ = advance(t, m, next)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "run 412 passed") || strings.Contains(v, "lobby") {
		t.Fatalf("after the room vanished:\n%s", v)
	}
	// The last room goes: no room selected, no posts.
	for _, r := range []string{"dev", "runs"} {
		os.RemoveAll(filepath.Join(f.folder, ".moot/rooms", r))
	}
	m, _ = advance(t, m, tickPoll(m))
	if m.room != "" || len(m.posts) != 0 || !strings.Contains(ansi.Strip(m.View()), "No rooms.") {
		t.Fatalf("with every room gone: room %q", m.room)
	}
}

// Review note L10: a scrolled-up view is held by post id, so posts that arrive
// above it (one that sorts early) or below it move nothing.
func TestAScrolledUpViewIsHeldByPostID(t *testing.T) {
	f := longMoot(t)
	m := shown(t, f.open())
	m, _ = feed(m, key(tea.KeyPgUp), key(tea.KeyPgUp))
	if m.follow || m.anchorID == "" {
		t.Fatal("not scrolled up")
	}
	topLine, anchor, off := m.lines[m.top], m.anchorID, m.anchorOff
	rows := lines(ansi.Strip(m.View()))

	// A late post that sorts before everything, and a new newest post.
	f.post("dev", "20260801-080000-grace", "late arrival, sorts first\nwith another line\nand a third\n")
	f.post("dev", "20260802-120000-linus", "newest post\n")
	m, _ = advance(t, m, tickPoll(m))
	if len(m.posts) != 42 {
		t.Fatalf("posts = %d", len(m.posts))
	}
	if m.follow || m.anchorID != anchor || m.anchorOff != off {
		t.Fatalf("the hold moved: %q+%d -> %q+%d follow %v", anchor, off, m.anchorID, m.anchorOff, m.follow)
	}
	if m.lines[m.top] != topLine {
		t.Fatalf("the top line changed: %q -> %q", ansi.Strip(topLine), ansi.Strip(m.lines[m.top]))
	}
	if got := lines(ansi.Strip(m.View())); fmt.Sprint(got[2:len(got)-2]) != fmt.Sprint(rows[2:len(rows)-2]) {
		t.Fatal("the posts pane shows other lines than before")
	}

	// At the bottom the pane keeps following.
	m, _ = feed(m, key(tea.KeyEnd))
	f.post("dev", "20260802-130000-ada", "even newer\n")
	m, _ = advance(t, m, tickPoll(m))
	if !m.follow || m.top != m.maxTop() || !strings.Contains(ansi.Strip(m.View()), "even newer") {
		t.Fatalf("not following: top %d of %d", m.top, m.maxTop())
	}
}

// A resize re-wraps the transcript and the held post stays in view.
func TestAResizeKeepsTheHeldPost(t *testing.T) {
	m := shown(t, newSpy(longMoot(t)))
	m, _ = feed(m, key(tea.KeyHome), key(tea.KeyPgDown))
	anchor := m.anchorID
	m, _ = feed(m, tea.WindowSizeMsg{Width: 70, Height: 20})
	if m.anchorID != anchor || m.follow {
		t.Fatalf("resize moved the hold: %q -> %q", anchor, m.anchorID)
	}
	if i := m.anchorIndex(); i < 0 || m.top < m.starts[i] || m.top >= m.starts[i]+m.blockLen(i) {
		t.Fatalf("top %d is outside the held post", m.top)
	}
}

func TestRedirectedRunCancellation(t *testing.T) {
	var out strings.Builder
	ctx, cancel := contextCancelled()
	defer cancel()
	m := New(newSpy(synthetic(t)), utc)
	if err := Run(ctx, m, nil, &out, false); err == nil || out.Len() != 0 {
		t.Fatalf("cancelled run: %v, %q", err, out.String())
	}
}
