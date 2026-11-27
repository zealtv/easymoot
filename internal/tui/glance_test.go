package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/twglance"
	"github.com/zealtv/easymoot/internal/twpane"
)

func TestMentionsReadsAsMootDoes(t *testing.T) {
	for text, want := range map[string]bool{
		"@atlas look at this":                true,
		"thanks, @atlas.":                    true,
		"@all: the build is red":             true,
		"@atlasx is someone else":            false,
		"the host @atlas.com, not a name":    false,
		"see `@atlas` in the README":         false,
		"```\n@atlas in a fence\n```":        false,
		"~~~\n@atlas\n~~~\nand @atlas after": true,
		"no mention here":                    false,
	} {
		if got := mentions(text, "atlas"); got != want {
			t.Errorf("mentions(%q) = %v, want %v", text, got, want)
		}
	}
}

// selectRoom moves the sidebar onto the named room by the keys that do it, and takes
// the read it asks for.
func selectRoom(t *testing.T, m Model, room string) Model {
	t.Helper()
	for i := 0; i < 10 && m.room != room; i++ {
		var cmd tea.Cmd
		m, cmd = feed(m, key(tea.KeyDown))
		if cmd != nil {
			m = answer(t, m, cmd)
		}
	}
	if m.room != room {
		t.Fatalf("could not select %q", room)
	}
	return m
}

func TestNextLineCountsUnreadAndTheWaitNamesAMention(t *testing.T) {
	f := synthetic(t) // the folder is atlas, and dev is on show
	f.room("tests")   // after runs in the sidebar, so reaching runs leaves it unread
	m := shown(t, newSpy(f))
	if g := m.glance(); g != (glance{}) {
		t.Fatalf("nothing is unread at launch: %+v", g)
	}
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	m = poll(t, m)
	if g := m.glance(); g.next != "1 new" || g.wait {
		t.Fatalf("one post in runs: %+v", g)
	}
	f.post("runs", "20260802-091000-grace", "@atlas the nightly needs you\n")
	f.post("tests", "20260802-092000-ada", "hello\n")
	m = poll(t, m)
	g := m.glance()
	if g.next != "3 new" {
		t.Fatalf("next = %q", g.next)
	}
	if !g.wait || g.waitText != "@atlas ← grace · runs" || g.waitAt == 0 {
		t.Fatalf("a mention of atlas should wait: %+v", g)
	}
	// The wait began when the mention was found, and a later poll keeps that time.
	at := g.waitAt
	if m = poll(t, m); m.glance().waitAt != at {
		t.Fatal("a later poll moved the wait's time")
	}
	// Reading runs clears the wait and its count; tests stays new.
	m = selectRoom(t, m, "runs")
	m = poll(t, m)
	if g := m.glance(); g.next != "1 new" || g.wait {
		t.Fatalf("after reading runs: %+v", g)
	}
	m = selectRoom(t, m, "tests")
	m = poll(t, m)
	if g := m.glance(); g != (glance{}) {
		t.Fatalf("everything read: %+v", g)
	}
}

// Only a mention of the opened folder's own name waits, not one of another
// participant, and not the reader's own post; any post from the other side of a DM
// does, as moot.sh counts it.
func TestOnlyAMentionOfTheReaderWaits(t *testing.T) {
	f := synthetic(t)
	f.room("dm-atlas-beta")
	m := shown(t, newSpy(f))
	f.post("runs", "20260802-090000-grace", "@ada please look\n")
	f.post("runs", "20260802-091000-atlas", "@atlas a note to self\n")
	m = poll(t, m)
	if g := m.glance(); g.wait || g.next != "2 new" {
		t.Fatalf("no mention of atlas by another: %+v", g)
	}
	f.post("dm-atlas-beta", "20260802-092000-beta", "a word, when you have a minute\n")
	m = poll(t, m)
	if g := m.glance(); !g.wait || g.waitText != "@atlas ← beta · @beta" {
		t.Fatalf("a DM from beta should wait: %+v", g)
	}
}

// The wait names a run room without its date, and the wait names the mention
// first, with +N for the others.
func TestTheMapsLinesShortenARunRoom(t *testing.T) {
	f := synthetic(t)
	f.room("run-20260802-nightly")
	f.room("run-")
	m := shown(t, newSpy(f))
	f.post("run-", "20260802-090000-ada", "@atlas and here\n")
	f.post("run-20260802-nightly", "20260802-091000-grace", "@atlas the nightly needs you\n")
	m = poll(t, m)
	g := m.glance()
	if g.next != "2 new" {
		t.Fatalf("next = %q", g.next)
	}
	if g.waitText != "@atlas ← grace · nightly +1" {
		t.Fatalf("waitText = %q", g.waitText)
	}
	for in, want := range map[string]string{"run-20260802-nightly": "nightly", "run-2026080-x": "run-2026080-x", "runs": "runs", "run-20260802-": "run-20260802-", "dev-run-20260802-x": "dev-run-20260802-x"} {
		if got := short(in); got != want {
			t.Errorf("short(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlanceValues(t *testing.T) {
	if v := (glance{}).values(); v[twpane.Next] != "" || v[twpane.Status] != "" || v[twpane.StatusText] != "" || v[twpane.StatusAt] != "" {
		t.Fatalf("an empty glance should unset everything: %v", v)
	}
	v := glance{next: "1 new", wait: true, waitText: "@atlas ← grace · runs", waitAt: 42}.values()
	if v[twpane.Next] != "1 new" || v[twpane.Status] != "wait" ||
		v[twpane.StatusText] != "@atlas ← grace · runs" || v[twpane.StatusAt] != "42" {
		t.Fatalf("values = %v", v)
	}
}

// Update reports the glance after every message, as it does @tw-focus.
func TestUpdateReportsTheGlance(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	var calls []string
	m.tw = twpane.Pane{ID: "%9", Run: func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil }}
	f.post("runs", "20260802-090000-ci-bot", "run 414 passed\n")
	// With a pane, the tick's command is a batch: the poll and the first write,
	// which unsets what an earlier run left. (The poll's answer brings the next
	// tick, which waits, so its command is not run here.)
	m, cmd := feed(m, tickMsg{src: m.src})
	for _, msg := range flatten(cmd) {
		if p, ok := msg.(pollMsg); ok {
			next, _ := m.Update(p)
			m = next.(Model)
		}
	}
	if len(calls) == 0 || !strings.Contains(calls[0], "-pu -t %9 @tw-next") {
		t.Fatalf("tmux calls: %v", calls)
	}
	// The poll's answer asked for the new line already, so asking again is no change.
	if m.tw.Sync(glance{next: "1 new"}.values()) != nil {
		t.Fatal("Update did not ask for @tw-next after the poll")
	}
}

// A room tipsOf didn't list this poll has not changed: it keeps its hits and is not
// read again. A body that can't be read is not cached, so the next listing retries it.
func TestCheckMentionsReadsEachArrivalOnce(t *testing.T) {
	f := synthetic(t)
	f.post("runs", "20260802-091000-grace", "@atlas the nightly needs you\n")
	spy := newSpy(f)
	rooms, err := spy.Rooms()
	if err != nil {
		t.Fatal(err)
	}
	posts, err := spy.List("runs")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]roomTip{"runs": {count: 1, newest: "20260802-081000-ci-bot"}}
	tips := map[string]roomTip{"runs": tipOf(posts)}
	listed := map[string][]moot.Post{"runs": posts}
	now := time.Unix(1000, 0)
	k := hitKey("runs", "20260802-091000-grace")

	failing := bodyFails{spy}
	if hits := checkMentions(failing, rooms, "dev", tips, seen, listed, nil, "atlas", now); len(hits) != 0 {
		t.Fatalf("an unread body was cached: %+v", hits)
	}
	spy.take()
	hits := checkMentions(spy, rooms, "dev", tips, seen, listed, nil, "atlas", now)
	if h := hits[k]; !h.counts || h.found != 1000 {
		t.Fatalf("first check: %+v", hits)
	}
	if calls := strings.Join(spy.take(), "|"); calls != "Body runs 20260802-091000-grace" {
		t.Fatalf("the first check should read the new post only: %s", calls)
	}
	// Listed again, or not listed at all: nothing is read, and the hit stays.
	for _, l := range []map[string][]moot.Post{listed, {}} {
		again := checkMentions(spy, rooms, "dev", tips, seen, l, hits, "atlas", time.Unix(2000, 0))
		if calls := spy.take(); len(calls) != 0 {
			t.Fatalf("a checked post was read again: %v", calls)
		}
		if again[k] != hits[k] {
			t.Fatalf("the hit changed: %+v", again[k])
		}
	}
}

type bodyFails struct{ *spySource }

func (bodyFails) Body(room, id string) (moot.Body, error) {
	return moot.Body{}, errors.New("mid-write")
}

// With a participant selected no room is on show, so a mention in the room behind it
// waits too, as its posts count as new there.
func TestAMentionWaitsInTheRoomBehindASelectedParticipant(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	m.onPerson = true // the selection is on a participant; m.room is still dev
	f.post("dev", "20260802-090000-grace", "@atlas a question\n")
	m = poll(t, m)
	if g := m.glance(); !g.wait || g.next != "1 new" {
		t.Fatalf("a mention behind a participant: %+v", g)
	}
}

// The wait's time is when its newest mention was first found, not when it became the
// newest: reading a later one leaves the earlier one's time.
func TestTheWaitKeepsEachMentionsOwnTime(t *testing.T) {
	m := Model{
		name:   "atlas",
		unread: map[string]bool{"runs": true, "tests": true},
		seen:   map[string]roomTip{"runs": {}, "tests": {}},
		hits: map[string]mentionHit{
			"a": {room: "runs", id: "20260802-090000-grace", sender: "grace", counts: true, found: 100},
			"b": {room: "tests", id: "20260802-091000-ada", sender: "ada", counts: true, found: 200},
		},
	}
	if g := m.glance(); g.waitAt != 200 {
		t.Fatalf("waitAt = %d, want the newest mention's 200", g.waitAt)
	}
	m.unread = map[string]bool{"runs": true} // tests was read
	if g := m.glance(); g.waitAt != 100 {
		t.Fatalf("waitAt = %d, want runs' own 100", g.waitAt)
	}
}

func TestPaneIDOnlyInsideTmux(t *testing.T) {
	for env, want := range map[[2]string]string{
		{"/tmp/tmux-1/default,1,0", "%3"}: "%3",
		{"", "%3"}:                        "", // a stale $TMUX_PANE outside tmux
		{"/tmp/tmux-1/default,1,0", ""}:   "",
	} {
		got := paneID(func(k string) string {
			if k == "TMUX" {
				return env[0]
			}
			return env[1]
		})
		if got != want {
			t.Errorf("TMUX=%q TMUX_PANE=%q: pane %q, want %q", env[0], env[1], got, want)
		}
	}
}

// The badge line counts unread posts, mentions and listeners; the list names each
// listener, current ones first, and leaves out a paused watch and one with no watch.
func TestGlanceBadgesAndListeners(t *testing.T) {
	f := synthetic(t) // the folder is atlas
	f.room("tests")
	m := shown(t, newSpy(f))
	text := func(es []twglance.Entry) string {
		var out []string
		for _, e := range es {
			out = append(out, e.Role+":"+e.Text)
		}
		return strings.Join(out, ";")
	}
	if got := text(m.badges()); got != ":moot" {
		t.Fatalf("nothing unread, no participants: %q", got)
	}
	if len(m.listRows()) != 0 {
		t.Fatalf("no participants, but rows %v", m.listRows())
	}
	f.post("runs", "20260802-091000-grace", "@atlas the nightly needs you\n")
	f.post("tests", "20260802-092000-ada", "hello\n")
	m = poll(t, m)
	now := time.Now()
	people := peopleAt(now, []person{
		{folder: "/s/zed", Presence: moot.Presence{Name: "zed", Watch: moot.Watching, Status: "old news; days ago", Stamp: now.Add(-13 * time.Hour)}},
		{folder: "/s/ada", Presence: moot.Presence{Name: "ada", Watch: moot.Watching, Status: "run glance: working", Stamp: now.Add(-4 * time.Minute)}},
		{folder: "/s/bea", Presence: moot.Presence{Name: "bea", Watch: moot.Watching, Status: "\u200b", Stamp: now.Add(-13 * time.Hour)}}, // cleans to nothing
		{folder: "/s/cy", Presence: moot.Presence{Name: "cy", Watch: moot.WatchPaused, Status: "paused", Stamp: now}},
		{folder: "/s/dee", Presence: moot.Presence{Name: "dee", Status: "not watching", Stamp: now}},
	})
	m.people = people // as a poll with the walk would bring them
	if got, want := text(m.badges()), ":moot;:•2;hold:@1;ok:●1;quiet:●?2"; got != want {
		t.Fatalf("badges = %q, want %q", got, want)
	}
	if got, want := text(m.listRows()), "ok:ada › run glance: working;quiet:bea;quiet:zed › old news, days ago"; got != want {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	// On the wire one entry a listener, the ";" in zed's status a ",", so the map's
	// 16 rows hold 16 listeners.
	if got, want := twglance.MidValue(m.listRows()), "ok:ada › run glance: working;quiet:bea;quiet:zed › old news, days ago"; got != want {
		t.Fatalf("@tw-glance-mid = %q, want %q", got, want)
	}
}

// peopleAt is rows as peopleOf finishes them at now: aged, judged current, by name.
func peopleAt(now time.Time, rows []person) []person {
	for i := range rows {
		if p := &rows[i]; !p.Stamp.IsZero() {
			d := now.Sub(p.Stamp)
			p.age, p.current = ageOf(d), d >= -time.Minute && d < currentFor
		}
	}
	slices.SortStableFunc(rows, func(a, b person) int { return strings.Compare(a.Name, b.Name) })
	return rows
}

// On the map the poll reads presence even with the participants out of the sidebar's
// window, since the glance lists the listeners; peopleOf judges a status current under
// 12 hours.
func TestOnTheMapThePollReadsPresence(t *testing.T) {
	f := newFixture(t, "atlas")
	f.room("dev")
	for i := 0; i < 8; i++ {
		f.room(fmt.Sprintf("room-%d", i))
	}
	old := time.Now().UTC().Add(-13 * time.Hour).Format("20060102-150405")
	f.write(".moot/status", old+" yesterday's work\n")
	f.write("../beta/.moot/rooms/dev/posts/20260801-090000-x.md", "beta dev")
	f.write("../beta/.moot/watching", "pid="+strconv.Itoa(os.Getpid())+"\n")
	f.write("../beta/.moot/status", time.Now().UTC().Add(-time.Minute).Format("20060102-150405")+" reviewing\n")
	f.mkdir("../gamma/.moot/rooms")
	f.write("../gamma/.moot/watching", "pid="+strconv.Itoa(os.Getpid())+"\n")
	f.write("../gamma/.moot/status", time.Now().UTC().Add(2*time.Hour).Format("20060102-150405")+" from the future\n")
	short := resized(t, shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4)), 100, 10)
	if msg := short.poll(short.room, short.bodies, short.latest, true, false)().(pollMsg); msg.people != nil {
		t.Fatal("off the map, a poll with the participants out of sight read presence")
	}
	short.twg = twglance.Pane{ID: "%9"}
	msg := short.poll(short.room, short.bodies, short.latest, true, false)().(pollMsg)
	var got []string
	for _, p := range msg.people {
		got = append(got, fmt.Sprintf("%s %v %v", p.Name, p.listening(), p.current))
	}
	if want := "atlas false false,beta true true,gamma true false"; strings.Join(got, ",") != want {
		t.Fatalf("people = %q, want %q", strings.Join(got, ","), want)
	}
}

// Update writes the glance to the pane after a message, both options in one call, with
// moot's ";" made safe; with nothing changed it writes nothing more.
func TestUpdateWritesTheBadgesAndTheList(t *testing.T) {
	f := synthetic(t)
	m := shown(t, newSpy(f))
	var calls [][]string
	m.twg = twglance.Pane{ID: "%9", Run: func(args ...string) error { calls = append(calls, args); return nil }}
	m.people = peopleAt(time.Now(), []person{{folder: "/s/ada", Presence: moot.Presence{Name: "ada", Watch: moot.Watching, Status: "a; b", Stamp: time.Now()}}})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	flatten(cmd) // runs each command, the write among them
	if len(calls) != 1 {
		t.Fatalf("tmux calls: %v", calls)
	}
	got := strings.Join(calls[0], " ")
	for _, want := range []string{"set-option -p -t %9 @tw-glance-far :moot;ok:●1", "set-option -p -t %9 @tw-glance-mid ok:ada › a, b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("call %q lacks %q", got, want)
		}
	}
	nm := next.(Model)
	if nm.twg.Sync(nm.glanceParts()) != nil {
		t.Fatal("an unchanged glance asked for another write")
	}
}
