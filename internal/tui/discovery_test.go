package tui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func recursiveFixture(t *testing.T) (*fixture, *spySource, Model) {
	t.Helper()
	f := newFixture(t, "opened")
	f.room("local")
	f.post("local", "same-id", "opened body")
	for _, keeper := range []string{"alpha", "beta"} {
		f.write("../"+keeper+"/.moot/rooms/dev/posts/same-id.md", keeper+" body")
	}
	target := filepath.Join(filepath.Dir(f.folder), "alpha/.moot/rooms/dev")
	if err := os.Symlink(target, filepath.Join(f.folder, ".moot/rooms/alias")); err != nil {
		t.Fatal(err)
	}
	spy := newSpy(f)
	m := shown(t, spy, WithRoot(filepath.Dir(f.folder), 4))
	return f, spy, m
}

func TestRecursiveTreeReadsAndPostsByLocalLinkName(t *testing.T) {
	_, spy, m := recursiveFixture(t)
	if len(m.rooms) != 3 || len(m.sidebar()) != 6+1+len(m.people) {
		t.Fatalf("rooms %+v rows %+v", m.rooms, m.sidebar())
	}
	if m.roomName() != "local" || !strings.Contains(ansi.Strip(m.View()), "opened body") {
		t.Fatal(m.View())
	}
	// Headers never take arrow-key selection; same post ID must not reuse a body
	// from the room left behind.
	m, cmd := feed(m, key(tea.KeyDown))
	m = answer(t, m, cmd)
	if m.roomName() != "dev" || !strings.Contains(ansi.Strip(m.View()), "alpha body") || strings.Contains(m.View(), "opened body") {
		t.Fatal(m.View())
	}
	firstKey := m.room
	spy.take()
	m = openBarOn(t, m)
	if m.bar.room != "alias" {
		t.Fatalf("posting name %q", m.bar.room)
	}
	m, _ = feed(m, typeText("hello")...)
	m, cmd = feed(m, key(tea.KeyEnter))
	msgs := flatten(cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages %v", msgs)
	}
	if _, ok := msgs[0].(postMsg); !ok {
		t.Fatalf("Enter returned %T", msgs[0])
	}
	calls := spy.take()
	if len(calls) != 1 || calls[0] != `Post alias "hello"` {
		t.Fatalf("calls %v", calls)
	}
	m, cmd = feed(m, msgs...)
	if !m.follow || cmd == nil {
		t.Fatal("successful linked post did not read selected room")
	}
	m = answer(t, m, cmd)
	if m.room != firstKey || !m.read {
		t.Fatal("linked post changed selection")
	}
	m, cmd = feed(m, key(tea.KeyDown))
	m = answer(t, m, cmd)
	if m.room == firstKey || !strings.Contains(m.View(), "beta body") {
		t.Fatal(m.View())
	}
	spy.take()
	// Read-only refusal is reached through the real palette, with no entry bar.
	m, cmd = feed(m, append(typeText(">post"), key(tea.KeyEnter))...)
	if m.bar != nil || !strings.Contains(m.notice, "read-only") {
		t.Fatalf("bar %+v notice %q", m.bar, m.notice)
	}
	for _, msg := range flatten(cmd) {
		if _, ok := msg.(postMsg); ok {
			t.Fatal("read-only Post reached poster")
		}
	}
	if calls := spy.take(); len(calls) != 0 {
		t.Fatalf("refusal called source: %v", calls)
	}
	// Typing and Enter after a refusal also cannot post.
	m, cmd = feed(m, append(typeText("accidental sentence"), key(tea.KeyEnter))...)
	for _, msg := range flatten(cmd) {
		if _, ok := msg.(postMsg); ok {
			t.Fatal("refused bar still posted")
		}
	}
	if calls := spy.take(); len(calls) != 0 {
		t.Fatalf("refusal left an action open: %v", calls)
	}
}

func TestRecursiveUnreadAndRefreshKeepRoomIdentity(t *testing.T) {
	f, _, m := recursiveFixture(t)
	originalRows := m.rooms
	f.write("../beta/.moot/rooms/dev/posts/new-id.md", "new beta post")
	// Fourteen normal rounds must see new posts, but not new rooms.
	f.write("../aardvark/.moot/rooms/new/posts/one.md", "new room")
	for range 14 {
		m = poll(t, m)
	}
	if len(m.rooms) != 3 {
		t.Fatal("discovered before fifteenth round")
	}
	if &m.rooms[0] != &originalRows[0] {
		t.Fatal("unchanged room set rebuilt")
	}
	beta := m.rooms[2].Key()
	if !m.unread[beta] || m.unread[m.rooms[1].Key()] {
		t.Fatalf("unread %+v", m.unread)
	}
	next, cmd := m.jump(2)
	m = next.(Model)
	m = answer(t, m, cmd)
	selected := m.room
	m = poll(t, m)
	if len(m.rooms) != 4 || m.room != selected || m.sel != 3 {
		t.Fatalf("refresh rooms %+v sel %d room %q", m.rooms, m.sel, m.room)
	}
	if m.unread[selected] {
		t.Fatal("selected room marked unread")
	}
	f.write("../gamma/.moot/rooms/new/posts/one.md", "manual room")
	m, cmd = feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	if cmd == nil {
		t.Fatal("refresh scheduled no walk")
	}
	if len(m.rooms) != 4 {
		t.Fatal("Update walked synchronously")
	}
	m = answer(t, m, cmd)
	if len(m.rooms) != 5 || m.room != selected {
		t.Fatalf("manual refresh: %+v", m.rooms)
	}
}

func TestRecursiveMouseAndSidebarWindow(t *testing.T) {
	_, _, m := recursiveFixture(t)
	l := m.layout()
	// First keeper header is inert; alpha's room sits at projection row 3.
	before := m.room
	m, cmd := feed(m, tea.MouseMsg{X: l.side.x + 1, Y: l.side.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd != nil || m.room != before {
		t.Fatal("selected keeper header")
	}
	m, cmd = feed(m, tea.MouseMsg{X: l.side.x + 1, Y: l.side.y + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = answer(t, m, cmd)
	if m.rooms[m.sel].PostName != "alias" {
		t.Fatalf("selected %+v", m.rooms[m.sel])
	}
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 8})
	next, cmd := m.jump(2)
	m = next.(Model)
	m = answer(t, m, cmd)
	at := selectedRow(m.sidebar(), m)
	if at < m.sideTop || at >= m.sideTop+m.sideRows() {
		t.Fatalf("selected room %d outside sidebar %d + %d", at, m.sideTop, m.sideRows())
	}
}

func TestRecursiveReadOnlyItalicsAndPlainFallback(t *testing.T) {
	_, _, m := recursiveFixture(t)
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	m.sel = 2 // selected read-only also keeps italics
	row := m.roomRow(2, 30)
	if !strings.Contains(row, "\x1b[") || !strings.Contains(row, "3;") && !strings.Contains(row, ";3m") && !strings.Contains(row, "[3m") {
		t.Fatalf("no italic sequence: %q", row)
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	plain := m.roomRow(2, 30)
	if ansi.Strip(plain) != plain || !strings.Contains(plain, "dev") {
		t.Fatalf("plain fallback: %q", plain)
	}
}

func TestRecursiveHeardRoomOutsideRootAndRealScript(t *testing.T) {
	f := newFixture(t, "opened")
	f.installScript()
	f.write("../outside/.moot/rooms/shared/posts/old.md", "outside body")
	f.mkdir("../scan")
	if err := os.Symlink(filepath.Join(filepath.Dir(f.folder), "outside/.moot/rooms/shared"), filepath.Join(f.folder, ".moot/rooms/local-name")); err != nil {
		t.Fatal(err)
	}
	m := shown(t, f.open(), WithRoot(filepath.Join(filepath.Dir(f.folder), "scan"), 0))
	if len(m.rooms) != 1 || m.rooms[0].PostName != "local-name" || !strings.Contains(m.View(), "outside body") {
		t.Fatal(m.View())
	}
	m = openBarOn(t, m)
	m, _ = feed(m, typeText("posted from opened")...)
	m, cmd := feed(m, key(tea.KeyEnter))
	m, cmd = feed(m, cmd())
	if m.failure {
		t.Fatal(m.notice)
	}
	m = answer(t, m, cmd)
	if !strings.Contains(m.View(), "posted from opened") {
		t.Fatal(m.View())
	}
	reader, name := m.discovery.Reader(m.room)
	posts, err := reader.List(name)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range posts {
		if p.Sender == "opened" {
			found = true
		}
	}
	if !found {
		t.Fatalf("not posted as opened: %+v", posts)
	}
}

func TestRecursiveRefreshRejectsOldCatalogAndKeepsNewSelection(t *testing.T) {
	f, _, m := recursiveFixture(t)
	oldRead := m.readSelected()()
	// Schedule Refresh before the new directory exists: walking must happen only
	// when its command runs, not while Update handles the palette.
	m, refresh := feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	f.write("../aardvark/.moot/rooms/new/posts/one.md", "new room")
	m, read := feed(m, key(tea.KeyDown))
	m = answer(t, m, read)
	selected := m.room
	m = answer(t, m, refresh)
	if len(m.rooms) != 4 || m.room != selected || !strings.Contains(m.View(), "alpha body") {
		t.Fatal(m.View())
	}
	// An old selected-room read can answer late without rolling back the tree.
	old := oldRead.(pollMsg)
	old.asked = m.room // exercise the catalog check independently of selection check
	m, _ = feed(m, old)
	if len(m.rooms) != 4 || m.room != selected {
		t.Fatal("stale read restored an old tree")
	}
}

func TestRecursiveRemovedSelectionAndChangedHearing(t *testing.T) {
	f, _, m := recursiveFixture(t)
	m, cmd := feed(m, key(tea.KeyDown))
	m = answer(t, m, cmd)
	selected := m.room
	alias := filepath.Join(f.folder, ".moot/rooms/alias")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	m, cmd = feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	m = answer(t, m, cmd)
	if m.room != selected || m.postName(m.room) != "" {
		t.Fatal("hearing change lost selection or kept posting access")
	}
	m, cmd = feed(m, append(typeText(">post"), key(tea.KeyEnter))...)
	if m.bar != nil || cmd != nil || !strings.Contains(m.notice, "read-only") {
		t.Fatal("former heard room remained postable")
	}
	roomPath := filepath.Join(filepath.Dir(f.folder), "alpha/.moot/rooms/dev")
	// Rename into a hidden name to remove the selected room without deleting data.
	if err := os.Rename(roomPath, filepath.Join(filepath.Dir(roomPath), ".removed")); err != nil {
		t.Fatal(err)
	}
	m, cmd = feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	msg := cmd() // only discovery and reading, no timer
	next, cmd := feed(m, msg)
	m = next
	for _, msg := range flatten(cmd) {
		m, _ = feed(m, msg)
	}
	if m.room == selected || m.roomName() != "dev" || !strings.Contains(m.View(), "beta body") {
		t.Fatal(m.View())
	}
}

func TestRecursiveWalkAnswersCannotOvertakeNewerAppliedWalks(t *testing.T) {
	f, _, m := recursiveFixture(t)
	// A periodic walk overlaps a later manual refresh. Deliver their completed
	// answers in reverse order, without depending on goroutine timing.
	m.pollRound = 14
	m, older := feed(m, tickMsg{src: m.src})
	oldAnswer := older().(pollMsg)
	f.write("../aardvark/.moot/rooms/new/posts/one.md", "new room")
	m, newer := feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	newAnswer := newer().(pollMsg)
	if !oldAnswer.walk || oldAnswer.walkSequence >= newAnswer.walkSequence {
		t.Fatal("walk requests were not sequenced")
	}
	m, _ = feed(m, newAnswer)
	selected, discovery := m.room, m.discovery
	if len(m.rooms) != 4 {
		t.Fatal("newer walk did not discover new room")
	}
	m, next := feed(m, oldAnswer)
	if next == nil {
		t.Fatal("ignored periodic answer stopped the poll chain")
	}
	if len(m.rooms) != 4 || m.room != selected || m.discovery != discovery || m.appliedWalk != newAnswer.walkSequence {
		t.Fatal("older answer replaced newer discovery")
	}
	oldAnswer.discovery = nil
	oldAnswer.roomsErr = errors.New("outdated scan failure")
	m, _ = feed(m, oldAnswer)
	if m.loadErr != "" {
		t.Fatalf("older failure replaced newer success: %q", m.loadErr)
	}
	// Even an unchanged successful walk advances the applied sequence while
	// retaining its discovery pointer.
	m, refresh := feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	unchanged := refresh().(pollMsg)
	m, _ = feed(m, unchanged)
	if m.discovery != discovery || m.appliedWalk != unchanged.walkSequence || unchanged.walkSequence <= newAnswer.walkSequence {
		t.Fatal("unchanged walk did not advance sequence")
	}
}

// A walk that finds the same rooms but a different list of participants still replaces
// the discovery: Participants lists whoever the latest walk found.
func TestAWalkWithNewParticipantsReplacesTheDiscovery(t *testing.T) {
	f, _, m := recursiveFixture(t)
	before := m.discovery
	rooms := len(m.rooms)
	f.mkdir("../newcomer/.moot/rooms") // a folder with a moot, and no room
	m, refresh := feed(m, append(typeText(">refresh"), key(tea.KeyEnter))...)
	m = answer(t, m, refresh)
	newcomer := filepath.Join(filepath.Dir(f.folder), "newcomer")
	if m.discovery == before || len(m.rooms) != rooms || !slices.Contains(m.discovery.Participants, newcomer) {
		t.Fatalf("discovery replaced %v, rooms %d (was %d), participants %v", m.discovery != before, len(m.rooms), rooms, m.discovery.Participants)
	}
}
