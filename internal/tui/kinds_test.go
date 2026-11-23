package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// kindsMoot is a synthetic moot of the opened folder "atlas" with live rooms, two
// DMs (the carol one newer), and an archived room, all made by hand: archived rooms
// and DMs are told apart by name alone, so no script is needed.
func kindsMoot(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t, "atlas")
	for _, r := range []string{"dev", "lobby", "old.archived", "dm-atlas-bob", "dm-atlas-carol"} {
		f.room(r)
	}
	f.post("dev", "20260801-090000-ada", "dev post\n")
	f.post("lobby", "20260801-091000-ada", "lobby post\n")
	f.post("old.archived", "20260701-090000-ada", "the last word here\n")
	f.post("dm-atlas-bob", "20260801-100000-bob", "hi from bob\n")
	f.post("dm-atlas-carol", "20260801-110000-carol", "hi from carol\n")
	return f
}

func labelsOf(m Model) string {
	var out []string
	for _, r := range m.rooms {
		out = append(out, m.label(r))
	}
	return strings.Join(out, ",")
}

// sidebarRows is the sidebar as one line: labels, with headers in brackets.
func sidebarRows(m Model) string {
	var rows []string
	for _, r := range m.sidebar() {
		switch {
		case r.room >= 0:
			rows = append(rows, m.label(m.rooms[r.room]))
		case r.person >= 0:
			rows = append(rows, m.people[r.person].label())
		case r.divider:
			rows = append(rows, "|")
		default:
			rows = append(rows, "["+r.header+"]")
		}
	}
	return strings.Join(rows, " ")
}

func headerTexts(m Model) []string {
	var out []string
	for _, r := range m.sidebar() {
		if r.header != "" {
			out = append(out, r.header)
		}
	}
	return out
}

// toggle runs the palette's Show/Hide archived by the keys that reach it.
func toggle(t *testing.T, m Model, typed string) (Model, tea.Cmd) {
	t.Helper()
	m, cmd := feed(m, append(typeText(":"+typed), key(tea.KeyEnter))...)
	if m.palette != nil {
		t.Fatalf("typing %q left the palette open", typed)
	}
	return m, cmd
}

func TestSidebarGroupsFlat(t *testing.T) {
	spy := newSpy(kindsMoot(t))
	m := shown(t, spy)
	if got := labelsOf(m); got != "dev,lobby,@carol,@bob" {
		t.Fatalf("labels = %s: the live rooms, then direct, newest post first; archived hidden", got)
	}
	if got := sidebarRows(m); got != "dev lobby [direct] @carol @bob" {
		t.Fatalf("rows = %s", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "4 rooms · 1 archived") || strings.Contains(v, "old") {
		t.Fatalf("the header counts the hidden room, and nothing else shows it:\n%s", v)
	}
}

func TestDirectOrdersByRecencyAsPostsArrive(t *testing.T) {
	f := kindsMoot(t)
	m := shown(t, newSpy(f))
	f.post("dm-atlas-bob", "20260802-080000-bob", "newer from bob\n")
	m = poll(t, m)
	if got := labelsOf(m); got != "dev,lobby,@bob,@carol" {
		t.Fatalf("labels = %s", got)
	}
	if !m.unread[m.rooms[2].Key()] {
		t.Fatalf("the DM that got a post is unread: %v", m.unread)
	}
}

func TestDMLabelUsesTheFoldersOwnNameNotTheEnvironment(t *testing.T) {
	f := kindsMoot(t)
	t.Setenv("MOOT_NAME", "session-override")
	m := shown(t, newSpy(f))
	if got := labelsOf(m); got != "dev,lobby,@carol,@bob" {
		t.Fatalf("labels = %s; the override must not change which side is this folder", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "as session-override") {
		t.Fatal("the header still shows the name a post goes out under")
	}
	f.write(".moot/name", "atlas-prime\n")
	f.room("dm-atlas-prime-dana")
	m = poll(t, m)
	if !strings.Contains(labelsOf(m), "@dana") {
		t.Fatalf("labels = %s: .moot/name should name this folder", labelsOf(m))
	}
}

// A moot with only archived rooms opens with nothing selected; Show archived selects
// the first room and reads it at once.
func TestShowArchivedOnAMootOfOnlyArchivedRoomsSelectsOne(t *testing.T) {
	f := newFixture(t, "atlas")
	f.room("old.archived")
	f.post("old.archived", "20260701-090000-ada", "the last word here\n")
	m := shown(t, newSpy(f))
	if m.room != "" || m.sel >= 0 {
		t.Fatalf("before: room %q sel %d", m.room, m.sel)
	}
	m, cmd := toggle(t, m, "s")
	if m.room != "old.archived" || m.sel != 0 || cmd == nil {
		t.Fatalf("after Show archived: room %q sel %d cmd %v", m.room, m.sel, cmd)
	}
	m = answer(t, m, cmd)
	if !strings.Contains(ansi.Strip(m.View()), "the last word here") {
		t.Fatal("the room's posts were not read")
	}
}

func TestDMLabels(t *testing.T) {
	cases := []struct {
		room, own, keeper string
		standing          moot.Standing
		want              string
	}{
		{"dm-alice-me", "me", "", moot.Kept, "@alice"},
		{"dm-me-zed", "me", "", moot.Heard, "@zed"},
		{"dm-kit-two-me", "me", "", moot.Heard, "@kit-two"},    // the other side holds a hyphen
		{"dm-me-alpha-dev", "me", "", moot.Kept, "@alpha-dev"}, // so does this one
		{"dm-alpha-dev-gamma-dev", "me", "/r/alpha-dev", moot.Elsewhere, "alpha-dev ↔ gamma-dev"},
		{"dm-kit-two-alpha", "me", "/r/alpha", moot.Elsewhere, "alpha ↔ kit-two"},
		{"dm-kit-two-me", "me", "/r/kit-two", moot.Elsewhere, "@kit-two"}, // this folder on one side
		{"dm-kit-two-me.archived", "me", "", moot.Heard, "@kit-two"},
		{"dm-who-knows-what", "me", "/r/third", moot.Elsewhere, "who-knows-what"}, // neither side: as written
		// Hyphens in the keeper's and the opened folder's names: split at the keeper first.
		{"dm-easymoot-dev-zed", "easymoot", "/r/zed", moot.Elsewhere, "zed ↔ easymoot-dev"},
		{"dm-easymoot-zed", "easymoot", "/r/zed", moot.Elsewhere, "@zed"},
		{"dm-a-b", "", "", moot.Kept, "a-b"}, // no name from the files
	}
	for _, c := range cases {
		r := moot.Room{Name: c.room, Keeper: c.keeper, Kind: moot.KindOf(c.room, c.standing)}
		name := strings.TrimSuffix(c.room, moot.ArchivedSuffix)
		if got := dmLabel(r, name, c.own); got != c.want {
			t.Errorf("%s as %q: %q, want %q", c.room, c.own, got, c.want)
		}
	}
}

func TestHiddenArchivedRoomsAreNotPolledNorMarkedUnread(t *testing.T) {
	f := kindsMoot(t)
	spy := newSpy(f)
	m := shown(t, spy)
	spy.take()
	f.post("old.archived", "20260802-090000-ada", "a late post in an archived room\n")
	for range 3 {
		m = poll(t, m)
	}
	for _, c := range spy.take() {
		if strings.Contains(c, "old.archived") {
			t.Fatalf("a hidden archived room was read: %q", c)
		}
	}
	if len(m.unread) != 0 {
		t.Fatalf("unread %v", m.unread)
	}
	// Shown, it is polled, and still never marked, however much arrives.
	m, _ = toggle(t, m, "s")
	m = poll(t, m)
	f.post("old.archived", "20260803-090000-ada", "and another\n")
	m = poll(t, m)
	m = poll(t, m)
	polled := false
	for _, c := range spy.take() {
		polled = polled || strings.Contains(c, "old.archived")
	}
	if !polled {
		t.Fatal("a shown archived room was not polled")
	}
	if m.unread["old.archived"] {
		t.Fatalf("an archived room was marked unread: %v", m.unread)
	}
}

func TestShowAndHideArchivedKeepTheSelection(t *testing.T) {
	m := shown(t, newSpy(kindsMoot(t)))
	// Select lobby, a room above the archived group.
	next, cmd := m.jump(1)
	m = answer(t, next.(Model), cmd)
	if m.room != "lobby" {
		t.Fatalf("room %q", m.room)
	}
	m, cmd = toggle(t, m, "show")
	if cmd != nil {
		t.Fatal("showing archived rooms read something")
	}
	if got := labelsOf(m); got != "dev,lobby,@carol,@bob,old" {
		t.Fatalf("labels = %s", got)
	}
	if m.room != "lobby" || m.rooms[m.sel].Name != "lobby" || m.sel != 1 {
		t.Fatalf("selection jumped: room %q sel %d", m.room, m.sel)
	}
	if got := strings.Join(headerTexts(m), ","); got != "direct,archived" {
		t.Fatalf("headers %s", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "4 rooms + 1 archived") {
		t.Fatalf("header:\n%s", v)
	}
	m, cmd = toggle(t, m, "hide")
	if cmd != nil || m.room != "lobby" || m.sel != 1 || labelsOf(m) != "dev,lobby,@carol,@bob" {
		t.Fatalf("hide: room %q sel %d labels %s cmd %v", m.room, m.sel, labelsOf(m), cmd)
	}
	// Open the archived room, then hide: its neighbour is selected and read.
	m, _ = toggle(t, m, "s")
	next, cmd = m.jump(4)
	m = answer(t, next.(Model), cmd)
	if m.room != "old.archived" || !strings.Contains(ansi.Strip(m.View()), "the last word here") {
		t.Fatalf("archived room not open: %q", m.room)
	}
	m, cmd = toggle(t, m, "h")
	if m.room == "old.archived" || m.sel < 0 || cmd == nil {
		t.Fatalf("hiding the open archived room: room %q sel %d cmd %v", m.room, m.sel, cmd)
	}
	if want := "@bob"; m.label(m.rooms[m.sel]) != want {
		t.Fatalf("neighbour %q, want %q", m.label(m.rooms[m.sel]), want)
	}
	m = answer(t, m, cmd)
	if !strings.Contains(ansi.Strip(m.View()), "hi from bob") {
		t.Fatal("the neighbour's posts were not read")
	}
}

func TestPaletteListsArchivedAndPinnedCommandsOnlyWhereTheyApply(t *testing.T) {
	names := func(m Model) string {
		var out []string
		for _, c := range m.commands() {
			if !c.strict { // the commands that only look; the room actions have their own test
				out = append(out, c.name)
			}
		}
		return strings.Join(out, ",")
	}
	// No archived room, no pinned post.
	if got := names(shown(t, newSpy(synthetic(t)))); got != "" {
		t.Fatalf("commands = %s", got)
	}
	f := kindsMoot(t)
	m := shown(t, newSpy(f))
	if got := names(m); got != "Show archived" {
		t.Fatalf("commands = %s", got)
	}
	m, _ = toggle(t, m, "s")
	if got := names(m); got != "Hide archived" {
		t.Fatalf("commands = %s", got)
	}
	// A pinned post in the open room adds Pinned post, and only while that room is open.
	f.post("dev", "20260801-080000-pinned", "# Customs\n")
	m = poll(t, m)
	if got := names(m); got != "Pinned post,Hide archived" {
		t.Fatalf("commands = %s", got)
	}
	next, cmd := m.jump(1)
	m = answer(t, next.(Model), cmd)
	if got := names(m); got != "Hide archived" {
		t.Fatalf("commands in a room with no pinned post = %s", got)
	}
	// The way a command is typed: s, h and pi find them; p still finds Post.
	m, _ = toggle(t, m, "hide")
	if m.showArchived {
		t.Fatal("hide did not hide")
	}
	p, _ := feed(m, typeText(":p")...)
	if p.palette.chosen != actionPost {
		t.Fatalf(":p chose %q", p.palette.chosen)
	}
}

func TestPostRefusesInAnArchivedRoomBeforeTheBarOpens(t *testing.T) {
	spy := newSpy(kindsMoot(t))
	m := shown(t, spy)
	m, _ = toggle(t, m, "s")
	next, cmd := m.jump(4)
	m = answer(t, next.(Model), cmd)
	spy.take()
	m, cmd = feed(m, append(typeText(":post"), key(tea.KeyEnter))...)
	if m.bar != nil || cmd != nil {
		t.Fatalf("the bar opened in an archived room: %v %v", m.bar, cmd)
	}
	want := "Can't post: old is archived; posting is closed"
	if m.notice != want || !m.failure {
		t.Fatalf("notice %q, want %q", m.notice, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), " ! "+want) {
		t.Fatalf("status line:\n%s", ansi.Strip(m.View()))
	}
	// Typing into the pane afterwards reaches nothing either.
	m, cmd = feed(m, append(typeText("a stray sentence"), key(tea.KeyEnter))...)
	if cmd != nil {
		t.Fatal("a key scheduled a command")
	}
	for _, c := range spy.take() {
		if strings.HasPrefix(c, "Post ") {
			t.Fatalf("something posted: %q", c)
		}
	}
}

func TestArchivedRoomSaysSoInTheTitleAndTheStatusLine(t *testing.T) {
	f := newFixture(t, "opened")
	f.room("live")
	f.room("kept.archived")
	f.post("kept.archived", "20260601-120000-pinned", "# Final customs\n")
	f.write("../other/.moot/rooms/theirs.archived/posts/20260801-090000-x.md", "theirs")
	f.write("../other/.moot/rooms/live-there/posts/20260801-090000-x.md", "live there")
	m := shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4))
	m, _ = toggle(t, m, "s")
	var kept, theirs int
	for i, r := range m.rooms {
		switch r.Name {
		case "kept.archived":
			kept = i
		case "theirs.archived":
			theirs = i
		}
	}
	next, cmd := m.jump(kept)
	k := answer(t, next.(Model), cmd)
	v := ansi.Strip(k.View())
	if !strings.Contains(v, "kept archived · 1 post · pinned 2026-06-01") {
		t.Fatalf("pane title:\n%s", v)
	}
	if !strings.Contains(v, "archived · reopen: mv kept.archived kept in .moot/rooms") {
		t.Fatalf("status line gives no way back:\n%s", v)
	}
	// In 50 columns the mv stays whole (the "in .moot/rooms" tail is what gives way).
	narrow := ""
	for _, l := range lines(ansi.Strip(resized(t, k, 50, 20).View())) {
		if strings.Contains(l, "reopen:") {
			narrow = strings.TrimSpace(l)
		}
	}
	if narrow != "archived · reopen: mv kept.archived kept" {
		t.Fatalf("the mv at 50 columns: %q", narrow)
	}
	next, cmd = k.jump(theirs)
	o := answer(t, next.(Model), cmd)
	v = ansi.Strip(o.View())
	if !strings.Contains(v, "theirs archived · 1 post") || !strings.Contains(v, "archived · posting is closed · kept by other") {
		t.Fatalf("a room kept elsewhere:\n%s", v)
	}
	if strings.Contains(v, "mv ") {
		t.Fatalf("offered a mv for a room the folder does not keep:\n%s", v)
	}
	// A live room says neither.
	next, cmd = o.jump(0)
	l := answer(t, next.(Model), cmd)
	if v := ansi.Strip(l.View()); strings.Contains(v, "posting is closed") || !strings.Contains(v, "keys only look · : opens commands") {
		t.Fatalf("a live room's status:\n%s", v)
	}
}

func TestRootedGroupsKeepersDirectAndArchived(t *testing.T) {
	f := newFixture(t, "opened")
	f.room("local")
	f.room("dm-opened-zed")
	f.room("old.archived")
	f.write("../aardvark/.moot/rooms/dev/posts/20260801-090000-x.md", "aardvark")
	f.write("../aardvark/.moot/rooms/dm-aardvark-beta/posts/20260801-090000-x.md", "dm")
	f.write("../beta/.moot/rooms/dev/posts/20260801-090000-x.md", "beta")
	f.write("../beta/.moot/rooms/gone.archived/posts/20260801-090000-x.md", "beta gone")
	f.post("dm-opened-zed", "20260801-120000-zed", "zed\n")
	m := shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4))
	m, _ = toggle(t, m, "s")
	got := sidebarRows(m)
	want := "[opened] local [direct] @zed aardvark ↔ beta [aardvark] dev [beta] dev [archived] old gone | aardvark beta opened (you)"
	if got != want {
		t.Fatalf("rows\n got %s\nwant %s", got, want)
	}
	// The other pair's DM is read-only: italic, never postable.
	if m.rooms[m.indexByName(t, "dm-aardvark-beta")].PostName != "" {
		t.Fatal("another pair's DM is postable")
	}
}

func (m Model) indexByName(t *testing.T, name string) int {
	t.Helper()
	for i, r := range m.rooms {
		if r.Name == name {
			return i
		}
	}
	t.Fatalf("no room %s in %v", name, labelsOf(m))
	return -1
}

func TestPinnedPostTitleAndScroll(t *testing.T) {
	f := newFixture(t, "atlas")
	f.room("dev")
	f.post("dev", "20260701-120000-pinned", "# Old customs\n")
	f.post("dev", "20260720-120000-pinned", "# Current customs\n\nSign your posts.\n")
	for i := 0; i < 30; i++ {
		f.post("dev", fmt.Sprintf("20260801-09%04d-ada", i), fmt.Sprintf("post %02d\nsecond line\n", i))
	}
	f.room("quiet")
	m := shown(t, newSpy(f))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "dev 32 posts · pinned 2026-07-20") {
		t.Fatalf("title does not mark the newest pinned post:\n%s", v)
	}
	if !m.follow || strings.Contains(v, "Current customs") {
		t.Fatal("the room should open at its latest post")
	}
	m, cmd := feed(m, append(typeText(":pi"), key(tea.KeyEnter))...)
	if cmd != nil {
		t.Fatal("scrolling to the pinned post scheduled a read")
	}
	v = ansi.Strip(m.View())
	if m.follow || !strings.Contains(v, "Current customs") || strings.Contains(v, "Old customs") {
		t.Fatalf("not scrolled to the newest pinned post:\n%s", v)
	}
	if !strings.Contains(v, "scrolled up, end follows") || !strings.Contains(v, "pinned 2026-07-20 · scrolled up") {
		t.Fatalf("title:\n%s", v)
	}
	if s := statusRow(m); !strings.Contains(s, "the pinned post of dev, 2026-07-20 · end returns to the latest") {
		t.Fatalf("the command gave no word on the status line: %q", s)
	}
	// Held as any scroll is: a new post below changes nothing; end returns.
	top := m.top
	f.post("dev", "20260803-090000-ada", "late\n")
	m = poll(t, m)
	if m.top != top || m.follow {
		t.Fatalf("view moved: top %d -> %d, follow %v", top, m.top, m.follow)
	}
	m, _ = feed(m, key(tea.KeyEnd))
	if !m.follow || !strings.Contains(ansi.Strip(m.View()), "late") {
		t.Fatal("end did not return to the latest")
	}
	// A room with none lists no command and shows no mark.
	next, cmd := m.jump(1)
	q := answer(t, next.(Model), cmd)
	if strings.Contains(ansi.Strip(q.View()), "pinned 2026") { // not the earlier notice's words
		t.Fatal("a room with no pinned post shows a pinned mark")
	}
}

// Narrow layouts: Pinned post brings the posts pane up when only one pane shows.
func TestPinnedPostInTheSinglePaneLayoutShowsThePosts(t *testing.T) {
	f := newFixture(t, "atlas")
	f.room("dev")
	f.post("dev", "20260701-120000-pinned", "# Customs\n")
	for i := 0; i < 30; i++ {
		f.post("dev", fmt.Sprintf("20260801-09%04d-ada", i), fmt.Sprintf("post %02d\n", i))
	}
	m := resized(t, shown(t, newSpy(f)), 60, 20)
	m, _ = feed(m, append(typeText(":pi"), key(tea.KeyEnter))...)
	if m.focus != panePosts || !strings.Contains(ansi.Strip(m.View()), "Customs") {
		t.Fatalf("focus %v:\n%s", m.focus, ansi.Strip(m.View()))
	}
}

// TestKindsFrames writes the kinds screens at 80 and 60 columns under NO_COLOR to
// EASYMOOT_CAPTURE_DIR when that names a directory; TestViewFitsEverySize checks that
// they fit.
func TestKindsFrames(t *testing.T) {
	out := os.Getenv("EASYMOOT_CAPTURE_DIR")
	if out == "" {
		t.Skip("EASYMOOT_CAPTURE_DIR not set")
	}
	t.Setenv("NO_COLOR", "1")
	f := newFixture(t, "atlas")
	for _, r := range []string{"dev", "lobby", "old-run.archived", "dm-atlas-bob", "dm-atlas-carol"} {
		f.room(r)
	}
	f.post("dev", "20260720-120000-pinned", "# Customs\n\nSign your posts.\n")
	f.post("dev", "20260801-090000-ada", "Starting on the index rebuild.\n— ada\n")
	f.post("lobby", "20260801-091000-ada", "lobby post\n")
	f.post("old-run.archived", "20260701-090000-ada", "Run closed. This room is archived: read it, don't post in it.\n")
	f.post("dm-atlas-bob", "20260801-100000-bob", "Wait for me. I want to see the frames first.\n")
	f.post("dm-atlas-carol", "20260801-110000-carol", "Phase 1 is done.\n")
	f.write("../beta/.moot/rooms/dev/posts/20260801-090000-x.md", "beta dev")
	f.write("../beta/.moot/rooms/dm-atlas-beta/posts/20260801-090000-x.md", "kept by beta")
	f.write("../beta/.moot/rooms/dm-beta-gamma/posts/20260801-090000-x.md", "another pair")
	f.write("../gamma/.moot/rooms/dev/posts/20260801-090000-x.md", "gamma dev")
	t.Setenv("HOME", filepath.Dir(f.folder))
	flat := shown(t, newSpy(f))
	rooted := shown(t, newSpy(f), WithRoot(filepath.Dir(f.folder), 4))
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		w, h := size[0], size[1]
		hidden := resized(t, flat, w, h)
		on, _ := toggle(t, hidden, "s")
		next, cmd := on.jump(on.indexByName(t, "old-run.archived"))
		open := answer(t, next.(Model), cmd)
		open.focus = panePosts
		pal, _ := feed(open, key2(":"))
		r, _ := toggle(t, resized(t, rooted, w, h), "s")
		pin, _ := feed(hidden, append(typeText(":pi"), key(tea.KeyEnter))...)
		for name, m := range map[string]Model{"hidden": hidden, "shown": on, "archived-open": open, "palette": pal, "rooted-shown": r, "pinned": pin} {
			file := fmt.Sprintf("kinds-%s-%dx%d.txt", name, w, h)
			if err := os.WriteFile(filepath.Join(out, file), []byte(m.View()+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
