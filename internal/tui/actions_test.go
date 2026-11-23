package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// settle feeds msgs to the model and then every message the commands it returns
// produce, until none is left: a script, then the read that follows it.
func settle(m Model, msgs ...tea.Msg) Model {
	m, cmd := feed(m, msgs...)
	for i := 0; cmd != nil; i++ {
		if i > 10 {
			panic("settle: the commands never ran out")
		}
		m, cmd = feed(m, flatten(cmd)...)
	}
	return m
}

func typed(name string) []tea.Msg { return append(typeText(":"+name), key(tea.KeyEnter)) }

// ran says a command produced the message of a room action or a post.
func ran(cmd tea.Cmd) bool {
	for _, msg := range flatten(cmd) {
		switch msg.(type) {
		case actionMsg, postMsg:
			return true
		}
	}
	return false
}

// docs/design.md, "Room actions": which of Archive, Join and Leave the palette lists for each kind.
func TestRoomActionsAreListedByKind(t *testing.T) {
	kept, heard, away := moot.Standing(moot.Kept), moot.Standing(moot.Heard), moot.Standing(moot.Elsewhere)
	rows := []struct {
		name string
		kind moot.Kind
		want string
	}{
		{"kept, live", moot.Kind{Standing: kept}, "Archive room"},
		{"heard, live", moot.Kind{Standing: heard}, "Leave room"},
		{"not heard, live", moot.Kind{Standing: away}, "Join room"},
		{"archived, kept", moot.Kind{Standing: kept, Archived: true}, ""},
		{"archived, heard", moot.Kind{Standing: heard, Archived: true}, "Leave room"},
		{"archived, not heard", moot.Kind{Standing: away, Archived: true}, ""},
		{"DM I keep", moot.Kind{Standing: kept, DM: true}, ""},
		{"DM I hear", moot.Kind{Standing: heard, DM: true}, "Leave room"},
		{"DM between two others", moot.Kind{Standing: away, DM: true}, ""},
	}
	m := shown(t, newSpy(synthetic(t)))
	for _, r := range rows {
		m.rooms, m.sel = []moot.Room{{Name: "x", Kind: r.kind}}, 0
		var got []string
		for _, c := range m.commands() {
			if c.action == actionArchive || c.action == actionJoin || c.action == actionLeave {
				got = append(got, c.name)
			}
		}
		if strings.Join(got, ",") != r.want {
			t.Errorf("%s lists %q, want %q", r.name, got, r.want)
		}
	}
}

// Only a typed Enter on the bar's own row runs a verb: not a paste, not an Esc fused
// with the next key, not any other key. Tested by what the returned command produces.
func TestActionBarsRunOnlyOnATypedEnter(t *testing.T) {
	spy := newSpy(synthetic(t))
	m := shown(t, spy) // dev is selected and kept here
	spy.take()
	notTyped := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\r"), Paste: true},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y\n"), Paste: true},
		tea.KeyMsg{Type: tea.KeyEnter, Alt: true},
		key2("y"), key(tea.KeySpace), key(tea.KeyTab), key(tea.KeyEsc),
	}
	// DM <name> opens a post's bar in Participants, on a participant who is not (you).
	dm := seeing(m, somePeople(2))
	dm.person = dm.people[1].folder
	starts := map[string]Model{"archive": m, "create": m, "dm": dm}
	for _, name := range []string{"archive", "create", "dm"} {
		for _, msg := range notTyped {
			bar, cmd := feed(starts[name], typed(name)...)
			if cmd != nil || bar.bar == nil {
				t.Fatalf(":%s did not open a bar quietly", name)
			}
			if name != "archive" {
				// Text to act on: a blank bar would refuse the fused Enter anyway.
				bar, _ = feed(bar, typeText("run-notes")...)
			}
			bar, cmd = feed(bar, msg)
			if ran(cmd) {
				t.Errorf("%s bar: %#v ran a verb", name, msg)
			}
			if bar.acting || len(spy.take()) != 0 {
				t.Errorf("%s bar: %#v reached the script", name, msg)
			}
		}
	}
	// A confirmation takes no text; a name bar takes it, and blank text confirms nothing.
	bar, _ := feed(m, append(typed("archive"), typeText("abc")...)...)
	if bar.bar.text != "" {
		t.Errorf("a confirmation took the text %q", bar.bar.text)
	}
	bar, cmd := feed(m, append(typed("create"), key(tea.KeyEnter))...)
	if ran(cmd) || bar.bar == nil {
		t.Error("Enter on a blank name ran")
	}
	// A name that moot.sh would read as an option stays in the bar, unrun.
	bar, cmd = feed(m, append(append(typed("create"), typeText("-h")...), key(tea.KeyEnter))...)
	if ran(cmd) || bar.bar == nil || bar.bar.text != "-h" || !bar.failure || !strings.Contains(bar.notice, "can't start with -") || len(spy.take()) != 0 {
		t.Errorf("a name starting with - ran or lost the bar: bar %+v, notice %q", bar.bar, bar.notice)
	}
	// The typed Enters.
	_, cmd = feed(m, append(typed("archive"), key(tea.KeyEnter))...)
	if !ran(cmd) || strings.Join(spy.take(), ";") != "Run archive dev" {
		t.Error("a typed Enter did not archive dev")
	}
	_, cmd = feed(m, append(append(typed("create"), typeText(" run-notes ")...), key(tea.KeyEnter))...)
	if !ran(cmd) || strings.Join(spy.take(), ";") != "Run create run-notes" {
		t.Error("a typed Enter did not create run-notes")
	}
}

// Results: the script's line on success, its message on a refusal. While one runs nothing else acts, and ctrl+c quits.
func TestActionResultsAndOneAtATime(t *testing.T) {
	m := shown(t, newSpy(synthetic(t)))
	spy := m.src.(*spySource)
	answers := []struct {
		run  func([]string) (moot.RunResult, error)
		want string
		fail bool
	}{
		{func([]string) (moot.RunResult, error) {
			return moot.RunResult{Lines: []string{"archived rooms/dev.archived"}, Notices: []string{"repaired a link"}}, nil
		}, "archived rooms/dev.archived · repaired a link", false},
		{func([]string) (moot.RunResult, error) {
			return moot.RunResult{}, &moot.ScriptError{Message: "only its keeper archives it", ExitCode: 1}
		}, "Could not archive dev: only its keeper archives it", true},
		{func([]string) (moot.RunResult, error) { return moot.RunResult{}, moot.ErrTimeout },
			"moot.sh did not finish archiving dev; Refresh shows where it stands", true},
	}
	for _, a := range answers {
		spy.setRunner(a.run)
		busy, cmd := feed(m, append(typed("archive"), key(tea.KeyEnter))...)
		if busy.notice != "archiving dev…" || !busy.acting {
			t.Fatalf("while it runs: notice %q", busy.notice)
		}
		for _, c := range busy.commands() {
			if c.action != actionQuit {
				t.Errorf("%s listed while a verb runs", c.name)
			}
		}
		if _, c := busy.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !quits(c) {
			t.Error("ctrl+c did not quit at once during an action")
		}
		done, _ := feed(busy, flatten(cmd)...)
		if !strings.HasPrefix(done.notice, a.want) || done.failure != a.fail || done.acting {
			t.Errorf("notice %q (failure %v), want %q", done.notice, done.failure, a.want)
		}
	}
}

// Selection after an action, each verb through the pinned script: Create selects the
// new room; Archive moves to the neighbour (or follows the room while archived rooms
// show); Leave on a flat folder's link moves to the neighbour.
func TestSelectionAfterCreateArchiveAndLeave(t *testing.T) {
	f := newFixture(t, "atlas")
	f.installScript()
	for _, r := range []string{"a", "b", "c"} {
		f.room(r)
	}
	f.write("../carol/.moot/rooms/shared/posts/.gitkeep", "")
	if err := os.Symlink(filepath.Join(filepath.Dir(f.folder), "carol/.moot/rooms/shared"), filepath.Join(f.folder, ".moot/rooms/shared")); err != nil {
		t.Fatal(err)
	}
	m := shown(t, f.open())
	pick := func(m Model, name string) Model {
		next, cmd := m.jump(m.indexByName(t, name))
		return answer(t, next.(Model), cmd)
	}
	step := func(m Model, name string, confirm bool) Model {
		keys := typed(name)
		if confirm {
			keys = append(keys, key(tea.KeyEnter))
		}
		return settle(m, keys...)
	}

	m = step(m, "create", false)
	m = settle(m, append(typeText("new"), key(tea.KeyEnter))...)
	if m.room != "new" || !m.read || !strings.HasPrefix(m.notice, "created rooms/new") {
		t.Errorf("after create: room %q read %v notice %q", m.room, m.read, m.notice)
	}
	m = step(pick(m, "b"), "archive", true)
	if m.room != "c" || labelsOf(m) != "a,c,new,shared" {
		t.Errorf("after archive: room %q in %s, want c, the neighbour", m.room, labelsOf(m))
	}
	m, _ = toggle(t, m, "s")
	m = step(pick(m, "a"), "archive", true)
	if r, _ := m.roomKind(); m.room != "a.archived" || !r.Kind.Archived {
		t.Errorf("after archive with archived rooms shown: room %q, want a.archived", m.room)
	}
	m = step(pick(m, "shared"), "leave", true)
	if m.room == "shared" || strings.Contains(labelsOf(m), "shared") || !strings.HasPrefix(m.notice, "left rooms/shared") {
		t.Errorf("after leave: room %q in %s, notice %q", m.room, labelsOf(m), m.notice)
	}
}

// With --root, Join turns a room found below it into one this folder hears, and Leave
// puts it back: the row stays where it is, selected, and changes kind.
func TestJoinAndLeaveUnderRootKeepTheRow(t *testing.T) {
	f := newFixture(t, "atlas")
	f.installScript()
	f.room("local")
	f.write("../carol/.moot/rooms/shared/posts/.gitkeep", "")
	m := shown(t, f.open(), WithRoot(filepath.Dir(f.folder), 4))
	next, cmd := m.jump(m.indexByName(t, "shared"))
	m = answer(t, next.(Model), cmd)
	key0 := m.room
	if r, _ := m.roomKind(); r.Kind.Standing != moot.Elsewhere {
		t.Fatalf("shared stands %v before joining", r.Kind.Standing)
	}
	m = settle(m, typed("join")...)
	if r, _ := m.roomKind(); m.room != key0 || r.Kind.Standing != moot.Heard || !strings.HasPrefix(m.notice, "joined rooms/shared") {
		t.Errorf("after join: room %q kind %+v notice %q", m.room, r.Kind, m.notice)
	}
	if _, err := os.Lstat(filepath.Join(f.folder, ".moot/rooms/shared")); err != nil {
		t.Errorf("join made no link: %v", err)
	}
	m = settle(m, append(typed("leave"), key(tea.KeyEnter))...)
	if r, _ := m.roomKind(); m.room != key0 || r.Kind.Standing != moot.Elsewhere || !strings.HasPrefix(m.notice, "left rooms/shared") {
		t.Errorf("after leave: room %q kind %+v notice %q", m.room, r.Kind, m.notice)
	}
}

// At open, moot.sh rooms mends a link its keeper's archive left dangling, and what it
// said reaches the status line; Refresh runs it again.
func TestLinkRepairAtOpenReachesTheStatusLine(t *testing.T) {
	f := newFixture(t, "atlas")
	f.installScript()
	f.write("../carol/.moot/rooms/shared.archived/posts/.gitkeep", "")
	dir := filepath.Dir(f.folder)
	if err := os.Symlink(filepath.Join(dir, "carol/.moot/rooms/shared"), filepath.Join(f.folder, ".moot/rooms/shared")); err != nil {
		t.Fatal(err)
	}
	m := shown(t, f.open())
	if !strings.Contains(m.notice, "shared.archived") {
		t.Errorf("status %q does not say the link was mended", m.notice)
	}
	if _, err := os.Lstat(filepath.Join(f.folder, ".moot/rooms/shared.archived")); err != nil {
		t.Errorf("the link was not renamed on disk: %v", err)
	}
}

// Link repair (moot.sh rooms) runs on Refresh, and after every room action, before the
// rooms are listed again; the idle poll does not run it. Counted through the spy's Run
// calls, and a repair line reaches the status line.
func TestRefreshAndEveryActionRunLinkRepair(t *testing.T) {
	rooms := func(calls []string) int {
		n := 0
		for _, c := range calls {
			if c == "Run rooms" {
				n++
			}
		}
		return n
	}
	spy := newSpy(synthetic(t))
	m := shown(t, spy)
	spy.take()
	spy.setRunner(func(args []string) (moot.RunResult, error) {
		if args[0] == "rooms" {
			return moot.RunResult{Notices: []string{"moot: repaired a link"}}, nil
		}
		return moot.RunResult{Lines: []string{"done"}}, nil
	})

	m = settle(m, typed("refresh")...)
	if n := rooms(spy.take()); n != 1 || !strings.Contains(m.notice, "repaired a link") {
		t.Errorf("Refresh ran link repair %d times, notice %q", n, m.notice)
	}

	m = settle(m, append(typed("archive"), key(tea.KeyEnter))...)
	calls := spy.take()
	if a := indexOf(calls, "Run archive dev"); rooms(calls) != 1 || a < 0 || a > indexOf(calls, "Run rooms") {
		t.Errorf("an action should run link repair once, after the verb: %v", calls)
	}
	if !strings.Contains(m.notice, "done") || !strings.Contains(m.notice, "repaired a link") {
		t.Errorf("the action's line and the repair's both belong on the status line: %q", m.notice)
	}

	// A plain poll mends nothing.
	poll(t, m)
	if n := rooms(spy.take()); n != 0 {
		t.Errorf("the idle poll ran link repair %d times", n)
	}
}

func indexOf(calls []string, want string) int {
	for i, c := range calls {
		if c == want {
			return i
		}
	}
	return -1
}
