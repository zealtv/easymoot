package picker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeHome is a synthetic home directory under t.TempDir(). No test here scans the
// real ~ or reads a real moot.
func fakeHome(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "home")
}

func mkdir(t *testing.T, home string, rel ...string) {
	t.Helper()
	for _, r := range rel {
		if err := os.MkdirAll(filepath.Join(home, r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// makeMoot makes <rel>/.moot with the named rooms.
func makeMoot(t *testing.T, home, rel string, rooms ...string) {
	t.Helper()
	mkdir(t, home, filepath.Join(rel, ".moot", "rooms"))
	for _, r := range rooms {
		mkdir(t, home, filepath.Join(rel, ".moot", "rooms", r, "posts"))
	}
}

func scan(t *testing.T, home string) []Entry {
	t.Helper()
	got, err := Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func folders(es []Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Folder)
	}
	return out
}

func TestScanFindsFoldersWithAMootWithNamesAndRoomCounts(t *testing.T) {
	home := fakeHome(t)
	makeMoot(t, home, "repos/alpha", "dev", "runs")
	makeMoot(t, home, "repos/beta")
	mkdir(t, home, "repos/plain/src")
	got := scan(t, home)
	want := []Entry{
		{Folder: filepath.Join(home, "repos/alpha"), Name: "alpha", Rooms: 2},
		{Folder: filepath.Join(home, "repos/beta"), Name: "beta", Rooms: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// A match is a directory named .moot: a plain file of that name is no moot.
func TestScanWantsADirectoryNamedMoot(t *testing.T) {
	home := fakeHome(t)
	mkdir(t, home, "repos/file")
	if err := os.WriteFile(filepath.Join(home, "repos/file/.moot"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := scan(t, home); got != nil {
		t.Fatalf("a .moot file counted: %+v", got)
	}
}

// The home directory's own moot (what ~/.moot is) is found with no special case,
// and comes first however its path sorts against the rest.
func TestScanListsTheHomeFolderFirstThenPathOrder(t *testing.T) {
	home := fakeHome(t)
	for _, r := range []string{"repos/zed", "aaa", "repos/mid", "repos/alpha"} {
		makeMoot(t, home, r, "dev")
	}
	makeMoot(t, home, ".", "dev") // ~/.moot
	got := folders(scan(t, home))
	want := []string{
		home,
		filepath.Join(home, "aaa"),
		filepath.Join(home, "repos/alpha"),
		filepath.Join(home, "repos/mid"),
		filepath.Join(home, "repos/zed"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if es := scan(t, home); es[0].Name != filepath.Base(home) {
		t.Fatalf("the home entry is named %q, want the folder's own name", es[0].Name)
	}
}

func TestScanFollowsNoDirectorySymlink(t *testing.T) {
	home := fakeHome(t)
	outside := filepath.Join(filepath.Dir(home), "outside")
	makeMoot(t, outside, "elsewhere", "dev")
	makeMoot(t, home, "repos/real", "dev")
	// A link to a folder that has a moot, a link to a directory holding one, and a
	// link that loops back up: none of them is walked or listed.
	for link, target := range map[string]string{
		"repos/linked":        filepath.Join(outside, "elsewhere"),
		"repos/linked-parent": outside,
		"repos/loop":          home,
	} {
		if err := os.Symlink(target, filepath.Join(home, link)); err != nil {
			t.Fatal(err)
		}
	}
	got := folders(scan(t, home))
	if want := []string{filepath.Join(home, "repos/real")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The root itself may be a link (a home reached through one): its target is walked.
func TestScanWalksTheRootThroughALink(t *testing.T) {
	home := fakeHome(t)
	makeMoot(t, home, "repos/real", "dev")
	link := filepath.Join(filepath.Dir(home), "homelink")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	got := folders(scan(t, link))
	if want := []string{filepath.Join(link, "repos/real")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScanSkipsPrunedDirectories(t *testing.T) {
	home := fakeHome(t)
	makeMoot(t, home, "repos/keep", "dev")
	makeMoot(t, home, "repos/keep/node_modules/pkg", "dev")
	makeMoot(t, home, "repos/keep/vendor/pkg", "dev")
	makeMoot(t, home, "repos/keep/.git/modules/m", "dev")
	makeMoot(t, home, "Library/Application Support/app", "dev")  // ~/Library, pruned
	makeMoot(t, home, "repos/Library/inner", "dev")              // not under ~: found
	makeMoot(t, home, "repos/keep/.moot/rooms/dev/inner", "dev") // inside a .moot: not walked
	got := folders(scan(t, home))
	want := []string{
		filepath.Join(home, "repos/Library/inner"),
		filepath.Join(home, "repos/keep"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestScanStopsAtTheDepthBound(t *testing.T) {
	home := fakeHome(t)
	makeMoot(t, home, "a/b/c/d", "dev")   // four levels down: found
	makeMoot(t, home, "a/b/c/d/e", "dev") // five: not
	got := folders(scan(t, home))
	if want := []string{filepath.Join(home, "a/b/c/d")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScanOfNothingIsAnEmptyListAndNoError(t *testing.T) {
	home := fakeHome(t)
	mkdir(t, home, "repos/plain")
	got, err := Scan(context.Background(), home)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v; want nothing and no error", got, err)
	}
}

func TestScanPassesOverUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("permissions do not bind this user")
	}
	home := fakeHome(t)
	makeMoot(t, home, "repos/open", "dev")
	mkdir(t, home, "locked/inner")
	if err := os.Chmod(filepath.Join(home, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, "locked"), 0o755) })
	got := folders(scan(t, home))
	if want := []string{filepath.Join(home, "repos/open")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScanErrors(t *testing.T) {
	home := fakeHome(t)
	if _, err := Scan(context.Background(), home); err == nil {
		t.Error("a home that does not exist scanned without error")
	}
	mkdir(t, home, ".")
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(context.Background(), file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("a file scanned: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	makeMoot(t, home, "repos/x", "dev")
	if _, err := Scan(ctx, home); err == nil {
		t.Error("a cancelled scan returned no error")
	}
}

// Scanning lists directory names and counts rooms; it leaves every file as it was,
// seen markers included.
func TestScanChangesNothing(t *testing.T) {
	home := fakeHome(t)
	makeMoot(t, home, "repos/x", "dev")
	mkdir(t, home, "repos/x/.moot/seen/dev")
	marker := filepath.Join(home, "repos/x/.moot/seen/dev/ada")
	if err := os.WriteFile(marker, []byte("20260101-000000-ada\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := func() map[string]string {
		out := map[string]string{}
		_ = filepath.Walk(home, func(p string, i os.FileInfo, err error) error {
			if err == nil {
				out[p] = i.ModTime().String() + i.Mode().String()
			}
			return nil
		})
		return out
	}
	before := snap()
	scan(t, home)
	if after := snap(); !reflect.DeepEqual(before, after) {
		t.Fatal("a scan changed the tree")
	}
}

func TestShort(t *testing.T) {
	for _, c := range []struct{ home, path, want string }{
		{"/h/bob", "/h/bob", "~"},
		{"/h/bob", "/h/bob/repos/x", "~/repos/x"},
		{"/h/bob", "/h/bobby/x", "/h/bobby/x"},
		{"/h/bob", "/srv/x", "/srv/x"},
		{"", "/srv/x", "/srv/x"},
	} {
		if got := Short(c.home, c.path); got != c.want {
			t.Errorf("Short(%q, %q) = %q, want %q", c.home, c.path, got, c.want)
		}
	}
}
