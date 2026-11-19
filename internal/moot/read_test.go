package moot

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// shapes builds a room holding every shape moot.sh's listing rules cover.
func shapesFixture(t *testing.T) *fixture {
	f := newFixture(t, "proj")
	f.room("r")
	p := func(name, content string) { f.write(".moot/rooms/r/posts/"+name, content) }
	// Same-second posts from three senders, written out of order.
	p("20260829-100000-carol.md", "from carol\n")
	p("20260829-100000-alice.md", "hello @bob\nsecond line\n\n— alice\n")
	p("20260829-100000-bob.md", "from bob\n")
	// An old -2 suffix: id order, not filename order, puts x-alice-2 after x-alice.
	p("20260829-100001-alice.md", "first\n")
	p("20260829-100001-alice-2.md", "second\n")
	// A dotted sender.
	p("20260829-100002-ci.bot.md", "dotted\n")
	// A post directory with post.md and attachments, one without post.md.
	p("20260830-000000-dirpost/post.md", "in a directory\n")
	p("20260830-000000-dirpost/b.txt", "b")
	p("20260830-000000-dirpost/a.txt", "a")
	p("20260830-000000-dirpost/.hidden", "h")
	p("20260830-000001-nopost/a.png", "png")
	// A file and a directory sharing an id: the directory wins.
	p("20260830-000002-both.md", "the file\n")
	p("20260830-000002-both/post.md", "the directory\n")
	// Strays: not posts, or listed under their own names.
	p("README.md", "stray readme\n")
	p("notes.txt", "not a post\n")
	p(".20260101-000000-hidden.md", "hidden landing\n")
	p(".20260101-000000-x.1234.landing", "mid-compose\n")
	// A stamp that is no real date, and ids that are almost stamps.
	p("20261345-999999-baddate.md", "no such day\n")
	p("2026082-100000-short.md", "seven digits\n")
	p("20260829-100000-.md", "no sender\n") // id "20260829-100000-", not a stamp
	return f
}

type block struct {
	heading string
	body    string
}

// readBlocks parses `moot.sh read` output: each post is a "── heading ──" line
// followed by its text.
func readBlocks(t *testing.T, out string) []block {
	t.Helper()
	var blocks []block
	for _, line := range strings.Split(out, "\n") {
		if h, ok := strings.CutPrefix(line, "── "); ok && strings.HasSuffix(h, " ──") {
			blocks = append(blocks, block{heading: strings.TrimSuffix(h, " ──")})
			continue
		}
		if len(blocks) > 0 {
			blocks[len(blocks)-1].body += line + "\n"
		}
	}
	return blocks
}

func TestListingParityWithPinnedScript(t *testing.T) {
	setMootName(t, nil)
	f := shapesFixture(t)
	out, _, err := f.script("", "read", "--room", "r")
	if err != nil {
		t.Fatal(err)
	}
	want := readBlocks(t, out)

	m := f.open()
	posts, err := m.List("r")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != len(want) {
		t.Fatalf("Go lists %d posts, moot.sh read shows %d\nGo: %v\nread:\n%s", len(posts), len(want), ids(posts), out)
	}
	for i, p := range posts {
		b := want[i]
		var heading string
		switch {
		case !p.Stamp:
			heading = p.ID
			if p.Sender != "" || !p.Time.IsZero() {
				t.Errorf("%s: non-stamp has sender %q time %v", p.ID, p.Sender, p.Time)
			}
		case p.Time.IsZero():
			heading = p.WrittenTime() + " UTC · " + p.Sender
		default:
			heading = p.Time.Local().Format("2006-01-02 15:04:05 -0700") + " · " + p.Sender
		}
		if heading != b.heading {
			t.Errorf("post %d (%s): Go heading %q, moot.sh heading %q", i, p.ID, heading, b.heading)
		}

		body, err := m.Body("r", p.ID)
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		var wantBody string
		switch {
		case body.Missing:
			wantBody = fmt.Sprintf("(no post.md in posts/%s)", p.ID)
		default:
			wantBody = strings.TrimRight(body.Text, "\n")
		}
		got := strings.TrimSpace(b.body)
		if len(body.Attached) > 0 {
			var attachedLine string
			got, attachedLine, _ = strings.Cut(got, "\n\nattached: ")
			if attachedLine != strings.Join(body.Attached, " ") {
				t.Errorf("%s: attached %q, moot.sh says %q", p.ID, body.Attached, attachedLine)
			}
		} else if strings.Contains(got, "attached: ") {
			t.Errorf("%s: moot.sh shows attachments Go does not: %q", p.ID, got)
		}
		if got != strings.TrimSpace(wantBody) {
			t.Errorf("%s: body %q, moot.sh shows %q", p.ID, wantBody, got)
		}
	}

	// The shapes land where the rules say.
	byID := map[string]Post{}
	for _, p := range posts {
		byID[p.ID] = p
	}
	check := func(id string, f func(Post) bool, what string) {
		t.Helper()
		p, ok := byID[id]
		if !ok || !f(p) {
			t.Errorf("%s: want %s, got %+v (listed: %v)", id, what, p, ok)
		}
	}
	check("20260830-000000-dirpost", func(p Post) bool { return p.Dir }, "a post directory")
	check("20260830-000002-both", func(p Post) bool { return p.Dir }, "the directory winning over the file")
	check("README", func(p Post) bool { return !p.Stamp }, "a non-stamp")
	check("20260829-100001-alice-2", func(p Post) bool { return p.Sender == "alice-2" }, "sender alice-2")
	check("20260829-100002-ci.bot", func(p Post) bool { return p.Sender == "ci.bot" }, "a dotted sender")
	for _, id := range []string{"notes", "notes.txt", ".20260101-000000-hidden"} {
		if _, ok := byID[id]; ok {
			t.Errorf("%s listed, but is not a post", id)
		}
	}
	if got := ids(posts); !sort.StringsAreSorted(got) {
		t.Errorf("ids not in bytewise order: %v", got)
	}
}

func ids(posts []Post) []string {
	var out []string
	for _, p := range posts {
		out = append(out, p.ID)
	}
	return out
}

func TestListingIsBytewiseOldestFirst(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	for _, n := range []string{"b.md", "B-1.md", "a-2.md", "a.md", "_x.md", "z.md", "Z-1.md"} {
		f.write(".moot/rooms/r/posts/"+n, "x")
	}
	posts, err := f.open().List("r")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"B-1", "Z-1", "_x", "a", "a-2", "b", "z"}
	if got := ids(posts); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRoomsParity(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "proj")
	store := filepath.Join(f.root, "store")
	for _, r := range []string{"dev", "runs", "gone"} {
		if err := os.MkdirAll(filepath.Join(store, r, "posts"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rooms := filepath.Join(f.dir, "rooms")
	link := func(name, target string) {
		if err := os.Symlink(target, filepath.Join(rooms, name)); err != nil {
			t.Fatal(err)
		}
	}
	link("runs", filepath.Join(store, "runs"))
	link("dev", filepath.Join(store, "dev"))
	link("dangling", filepath.Join(store, "nowhere"))
	link(".hiddenlink", filepath.Join(store, "dev"))
	f.room("private")
	f.room(".hiddendir")
	f.write(".moot/rooms/afile", "not a room")
	f.room("Zed")

	out, _, err := f.script("", "rooms")
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.open().Rooms()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got {
		names = append(names, r.Name)
	}
	// moot.sh rooms prints extra columns; compare its room names, in its order.
	var want []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			want = append(want, strings.TrimPrefix(fields[0], "*"))
		}
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Go rooms %v, moot.sh rooms %v\n%s", names, want, out)
	}
	if exp := []string{"Zed", "dev", "private", "runs"}; !reflect.DeepEqual(names, exp) {
		t.Errorf("rooms %v, want %v", names, exp)
	}
	for _, r := range got {
		switch r.Name {
		case "dev", "runs":
			if r.Link != filepath.Join(store, r.Name) {
				t.Errorf("%s link %q", r.Name, r.Link)
			}
		default:
			if r.Link != "" {
				t.Errorf("%s: link %q, want none", r.Name, r.Link)
			}
		}
	}
}

func TestOpen(t *testing.T) {
	f := newFixture(t, "proj")
	for _, path := range []string{f.folder, f.dir, f.folder + "/", f.folder + "/sub/.."} {
		m, err := Open(path)
		if err != nil {
			t.Errorf("Open(%q): %v", path, err)
			continue
		}
		if m.Folder() != f.folder || m.Dir() != f.dir {
			t.Errorf("Open(%q): folder %q dir %q", path, m.Folder(), m.Dir())
		}
	}
	if _, err := Open(f.root); err == nil {
		t.Error("a folder with no .moot opened")
	}
	// A relative path is made absolute.
	t.Chdir(f.root)
	if m, err := Open("proj"); err != nil || m.Folder() != f.folder {
		t.Errorf("relative: %v, %v", m, err)
	}
}

func TestOpenKeepsSymlinkedFolder(t *testing.T) {
	f := newFixture(t, "real-name")
	link := filepath.Join(f.root, "link-name")
	if err := os.Symlink(f.folder, link); err != nil {
		t.Fatal(err)
	}
	m, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if m.Folder() != link {
		t.Errorf("folder %q, want the link %q kept", m.Folder(), link)
	}
}

func TestRoomsAndListMissing(t *testing.T) {
	f := newFixture(t, "proj")
	if err := os.Remove(f.dir + "/rooms"); err != nil {
		t.Fatal(err)
	}
	m := f.open()
	if rooms, err := m.Rooms(); err != nil || len(rooms) != 0 {
		t.Errorf("Rooms with no rooms/: %v, %v", rooms, err)
	}
	if posts, err := m.List("nope"); err != nil || len(posts) != 0 {
		t.Errorf("List of a missing room: %v, %v", posts, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b"} {
		if _, err := m.List(bad); err == nil {
			t.Errorf("List(%q) succeeded", bad)
		}
		if _, err := m.Body(bad, "x"); err == nil {
			t.Errorf("Body(%q) succeeded", bad)
		}
	}
}

func TestBodyCacheAndLimit(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	path := f.path(".moot/rooms/r/posts/20260829-100000-a.md")
	f.write(".moot/rooms/r/posts/20260829-100000-a.md", "one\n")
	m := f.open()
	id := "20260829-100000-a"
	b, err := m.Body("r", id)
	if err != nil || b.Text != "one\n" {
		t.Fatalf("%v %v", b, err)
	}
	// Same size and mtime: served from the cache even though the bytes differ.
	info, _ := os.Stat(path)
	if err := os.WriteFile(path, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.Body("r", id); b.Text != "one\n" {
		t.Errorf("same size and mtime reread: %q", b.Text)
	}
	// A changed size is read afresh.
	if err := os.WriteFile(path, []byte("three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := m.Body("r", id); b.Text != "three\n" {
		t.Errorf("changed file not reread: %q", b.Text)
	}
	// A body over 1 MiB is cut.
	big := strings.Repeat("x", MaxBody+100)
	f.write(".moot/rooms/r/posts/20260829-100001-big.md", big)
	b, err = m.Body("r", "20260829-100001-big")
	if err != nil || !b.Truncated || len(b.Text) != MaxBody {
		t.Errorf("big body: truncated %v len %d err %v", b.Truncated, len(b.Text), err)
	}
	// A post that is gone is an error.
	if _, err := m.Body("r", "20260829-100002-gone"); err == nil {
		t.Error("missing post read")
	}
}
