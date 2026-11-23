package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// seeing is m with participants rows brought by a poll as the walk's would, and the
// first of them selected in the sidebar.
func seeing(m Model, rows []person) Model {
	m.root = "/synthetic-root"
	rooms := slices.Clone(m.all)
	for i := range rooms { // kept by the opened folder, as the walk lists them
		rooms[i].Keeper, rooms[i].PostName = m.src.Folder(), rooms[i].Name
	}
	next, _ := m.Update(pollMsg{src: m.src, asked: m.room, room: m.room, rooms: rooms, posts: m.posts, bodies: m.bodies, name: m.name, people: rows})
	return next.(Model).pickPerson(0)
}

// somePeople is n synthetic participants, the first the opened folder, with each kind
// of watch and status.
func somePeople(n int) []person {
	var out []person
	for i := 0; i < n; i++ {
		p := person{folder: fmt.Sprintf("/synthetic/p%02d", i), you: i == 0, Presence: moot.Presence{Name: fmt.Sprintf("p%02d", i)}}
		switch i % 4 {
		case 1:
			p.Watch, p.Status, p.age = moot.Watching, "building the index, then a long tail of words to cut at the edge", "12m"
		case 2:
			p.Watch, p.Status, p.age = moot.WatchPaused, "heartbeat timing fix", "1h"
		case 3:
			p.Status = "no stamp on this one"
		}
		out = append(out, p)
	}
	return out
}

// TestParticipantsFromTheWalk reads a synthetic tree: the opened folder and two
// siblings, named by their files and not by $MOOT_NAME, sorted by name, listed in the
// sidebar under a divider below the rooms, with the selected one's strip in the right
// pane. Presence is read only while the divider is in the sidebar's window or a
// participant is selected.
func TestParticipantsFromTheWalk(t *testing.T) {
	f := newFixture(t, "atlas")
	t.Setenv("MOOT_NAME", "from-the-environment")
	f.room("dev")
	stamp := time.Now().UTC().Add(-90 * time.Minute).Format("20060102-150405")
	f.write(".moot/status", stamp+" building the participants view\n")
	f.write("../beta/.moot/rooms/dev/posts/20260801-090000-x.md", "beta dev")
	f.write("../beta/.moot/watching", "pid="+strconv.Itoa(os.Getpid())+"\n")
	f.write("../beta/.moot/status", "reviewing, no stamp\n")
	f.mkdir("../gamma/.moot/rooms")
	f.write("../gamma/.moot/name", "gee\n")
	m := shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4))

	var got []string
	for _, p := range m.people {
		got = append(got, fmt.Sprintf("%s|%s|%s|%s", p.label(), mark(p.Watch), p.Status, p.age))
	}
	want := []string{"atlas (you)|○|building the participants view|1h", "beta|●|reviewing, no stamp|", "gee|○||"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var side []string
	for _, line := range lines(ansi.Strip(m.View()))[1:10] {
		side = append(side, strings.TrimSpace(strings.TrimRight(ansi.Truncate(line, m.layout().side.w, ""), "━─")))
	}
	if want := "━━ Rooms 2,atlas,❯ dev,beta,dev,── Participants 3,○ atlas (you),● beta,○ gee"; strings.Join(side, ",") != want {
		t.Fatalf("sidebar:\n%s\nwant %s", strings.Join(side, "\n"), want)
	}

	// ↓↓ crosses the divider: the strip replaces the posts, the header counts people.
	m = settle(m, key(tea.KeyDown))
	m, cmd := feed(m, key(tea.KeyDown))
	if !m.onPerson || m.person != f.folder || cmd != nil {
		t.Fatalf("↓ from dev: on a person %v, %q, cmd %v", m.onPerson, m.person, cmd != nil)
	}
	v := ansi.Strip(m.View())
	for _, s := range []string{"3 participants · 1 watching", "○ atlas (you) · said 1h ago", "building the participants view", "❯ ○ atlas (you)"} {
		if !strings.Contains(v, s) {
			t.Fatalf("view lacks %q:\n%s", s, v)
		}
	}
	if strings.Contains(v, "1 post") || strings.Contains(v, "❯ dev") {
		t.Fatalf("the room still shows as selected:\n%s", v)
	}
	names := func(m Model) string {
		var out []string
		for _, c := range m.commands() {
			out = append(out, c.name)
		}
		return strings.Join(out, ",")
	}
	if got := names(m); got != "Refresh,Quit" {
		t.Fatalf("commands on (you) = %s", got)
	}
	m, _ = feed(m, key(tea.KeyDown))
	if got := names(m); got != "DM beta,Refresh,Quit" {
		t.Fatalf("commands on beta = %s", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "● beta · ") || !strings.Contains(v, "reviewing, no stamp") {
		t.Fatalf("beta's strip:\n%s", v)
	}
	// Back up to the room: its posts at once, from the poll that went on reading it.
	m, cmd = feed(m, key(tea.KeyUp), key(tea.KeyUp))
	if v := ansi.Strip(m.View()); m.onPerson || cmd != nil || !strings.Contains(v, "❯ dev") || !strings.Contains(v, "beta dev") {
		t.Fatalf("↑↑: on a person %v, cmd %v", m.onPerson, cmd != nil)
	}
	if strings.HasPrefix(names(m), "DM") {
		t.Fatal("DM is offered on a room")
	}
	// A click selects a participant as it does a room, and reads nothing.
	for y, line := range lines(ansi.Strip(m.View())) {
		if strings.Contains(line, "○ gee") {
			clicked, cmd := feed(m, tea.MouseMsg{X: 3, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			if p, _ := clicked.selectedPerson(); !clicked.onPerson || p.Name != "gee" || cmd != nil {
				t.Fatalf("a click on gee selected %q (on a person %v, cmd %v)", p.Name, clicked.onPerson, cmd != nil)
			}
		}
	}

	// With the divider out of the sidebar's window and the selection on a room, the
	// poll reads no presence; a selected participant has it read again.
	for i := 0; i < 8; i++ {
		f.room(fmt.Sprintf("room-%d", i))
	}
	short := resized(t, shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4)), 100, 10)
	if msg := short.poll(short.room, short.bodies, short.latest, true, false)().(pollMsg); msg.people != nil {
		t.Fatalf("a poll with the participants out of sight read presence: %+v", msg.people)
	}
	if msg := short.pickPerson(1).poll(short.room, nil, nil, false, false)().(pollMsg); len(msg.people) != 3 {
		t.Fatalf("a poll with a participant selected read %d", len(msg.people))
	}
}

// With no walk the sidebar has no participants section, and nothing reads presence.
func TestNoWalkNoParticipants(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	for _, r := range m.sidebar() {
		if r.divider || r.person >= 0 {
			t.Fatalf("a no-walk sidebar has %+v", r)
		}
	}
	if strings.Contains(m.View(), "── Participants") || m.people != nil {
		t.Fatalf("people %v:\n%s", m.people, m.View())
	}
}

// TestArrowsReachEveryParticipant: with more participants than rows, ↓ from the first
// room crosses the divider and reaches the last participant, and ↑ comes back to the
// first room, the selected row on screen each step (Bob, 2026-11-22: "i should be able
// to arrow through all participants").
func TestArrowsReachEveryParticipant(t *testing.T) {
	rows := somePeople(30)
	m := seeing(resized(t, shown(t, newSpy(synthetic(t))), 80, 20), rows)
	next, _ := m.jump(0)
	m = next.(Model)
	if m.sideRows() >= len(m.sidebar()) {
		t.Fatalf("%d rows shown: the list must be taller than its pane", m.sideRows())
	}
	var want []string // what the selected row reads, in list order
	for _, r := range m.sidebar() {
		switch {
		case r.room >= 0:
			want = append(want, "❯ "+m.rooms[r.room].Name)
		case r.person >= 0:
			want = append(want, "❯ "+mark(rows[r.person].Watch)+" "+rows[r.person].label())
		}
	}
	visible := func(i int) {
		t.Helper()
		for _, line := range lines(ansi.Strip(m.View())) {
			if strings.Contains(line, want[i]) {
				return
			}
		}
		t.Fatalf("step %d (%s) not shown selected:\n%s", i, want[i], ansi.Strip(m.View()))
	}
	step := func(k tea.KeyType, i int) {
		t.Helper()
		var cmd tea.Cmd
		m, cmd = feed(m, key(k))
		if cmd != nil { // a newly selected room is read
			m = answer(t, m, cmd)
		}
		visible(i)
	}
	visible(0)
	for i := 1; i < len(want); i++ {
		step(tea.KeyDown, i)
	}
	step(tea.KeyDown, len(want)-1)
	if !m.onPerson || m.person != rows[len(rows)-1].folder {
		t.Fatalf("the last ↓ selected %q", m.person)
	}
	for i := len(want) - 2; i >= 0; i-- {
		step(tea.KeyUp, i)
	}
}

// dmTo selects the participant called name.
func (m Model) dmTo(name string) Model {
	for i, p := range m.people {
		if p.Name == name {
			m = m.pickPerson(i)
		}
	}
	return m
}

// TestDMRunsDmThenThePost: DM <name>'s Enter runs moot.sh dm on the participant's
// folder, posts to the room its line names, and selects that room; the
// pinned script starts the DM the first time and has it the second. A refusal (no
// moot.sh at gamma) posts nothing and keeps the text.
func TestDMRunsDmThenThePost(t *testing.T) {
	f := newFixture(t, "atlas")
	f.installScript()
	f.room("dev")
	f.mkdir("../beta/.moot/rooms")
	f.installScript("..", "beta")
	f.mkdir("../gamma/.moot/rooms")
	spy := newSpy(f)
	var said []string
	spy.setRunner(func(args []string) (moot.RunResult, error) {
		res, err := spy.Moot.Run(context.Background(), args)
		if args[0] == "dm" {
			said = append(said, res.Lines...)
		}
		return res, err
	})
	spy.setPoster(func(room, text string) (moot.PostResult, error) {
		return spy.Moot.Post(context.Background(), room, text)
	})
	m := shown(t, spy, WithRoot(filepath.Dir(f.folder), 4))
	beta := filepath.Join(filepath.Dir(f.folder), "beta")
	spy.take()
	for i, verb := range []string{"started", "have"} {
		m = settle(m.dmTo("beta"), append(append(typed("dm"), typeText(fmt.Sprintf("hello %d", i))...), key(tea.KeyEnter))...)
		r, _ := m.roomKind()
		if m.onPerson || m.bar != nil || r.Name != "dm-atlas-beta" || r.Kind.Standing != moot.Kept || len(m.posts) != i+1 {
			t.Fatalf("after DM %d: on a person %v, room %+v, %d posts, notice %q", i, m.onPerson, r, len(m.posts), m.notice)
		}
		if said[i] != verb+" rooms/dm-atlas-beta" || !strings.HasPrefix(m.notice, "posted to dm-atlas-beta as atlas") {
			t.Fatalf("DM %d: dm said %q, notice %q", i, said[i], m.notice)
		}
		var calls []string
		for _, c := range spy.take() {
			if strings.HasPrefix(c, "Run") || strings.HasPrefix(c, "Post") {
				calls = append(calls, c)
			}
		}
		if want := fmt.Sprintf("Run dm %s;Post dm-atlas-beta \"hello %d\"", beta, i); strings.Join(calls, ";") != want {
			t.Fatalf("DM %d ran %q, want %q", i, calls, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(beta, ".moot/rooms/dm-atlas-beta")); err != nil {
		t.Errorf("beta was not linked to the DM: %v", err)
	}

	m = settle(m.dmTo("gamma"), append(append(typed("dm"), typeText("anyone there?")...), key(tea.KeyEnter))...)
	for _, c := range spy.take() {
		if strings.HasPrefix(c, "Post") {
			t.Errorf("a refused dm posted: %s", c)
		}
	}
	if m.bar == nil || m.bar.busy || m.bar.text != "anyone there?" || !m.failure ||
		!strings.HasPrefix(m.notice, "Could not post to @gamma: ") || !strings.Contains(m.notice, "install moot there first") {
		t.Fatalf("after a refusal: bar %+v, notice %q", m.bar, m.notice)
	}
	if _, err := os.Stat(filepath.Join(f.folder, ".moot/rooms/dm-atlas-gamma")); !os.IsNotExist(err) {
		t.Errorf("a refused dm left a room: %v", err)
	}
}

// A time-out in moot.sh dm posted nothing, so the bar stays with its text, and the
// status line names dm rather than saying the post may have landed.
func TestADmTimeoutKeepsTheBar(t *testing.T) {
	f := newFixture(t, "atlas")
	f.room("dev")
	f.mkdir("../beta/.moot/rooms")
	spy := newSpy(f)
	spy.setRunner(func(args []string) (moot.RunResult, error) { return moot.RunResult{}, moot.ErrTimeout })
	m := shown(t, spy, WithRoot(filepath.Dir(f.folder), 4))
	m = settle(m.dmTo("beta"), append(append(typed("dm"), typeText("still here")...), key(tea.KeyEnter))...)
	if m.bar == nil || m.bar.busy || m.bar.text != "still here" || !m.failure ||
		!strings.Contains(m.notice, "moot.sh dm did not finish") || strings.Contains(m.notice, "may have landed") {
		t.Fatalf("after a dm time-out: bar %+v, notice %q", m.bar, m.notice)
	}
}

// peopleOf sorts by name, in byte order (capitals first), whatever order the walk
// listed the folders in; the opened folder keeps its (you) mark wherever it lands.
func TestPeopleAreSortedByNameNotWalkOrder(t *testing.T) {
	f := newFixture(t, "opened")
	f.write(".moot/name", "mid\n")
	var folders []string
	folders = append(folders, f.folder)
	for _, name := range []string{"zulu", "alpha", "Zed"} {
		f.write("../"+name+"/.moot/name", name+"\n")
		folders = append(folders, filepath.Join(filepath.Dir(f.folder), name))
	}
	var got []string
	for _, p := range peopleOf(folders, time.Now()) {
		got = append(got, p.label())
	}
	if want := "Zed,alpha,mid (you),zulu"; strings.Join(got, ",") != want {
		t.Fatalf("people: %s, want %s", strings.Join(got, ","), want)
	}
}
