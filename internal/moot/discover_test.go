package moot

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverKeepersHearingAndWalkBoundaries(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	room := func(keeper, name string) { mkdir(filepath.Join(keeper, ".moot", "rooms", name, "posts")) }
	link := func(target, path string) {
		t.Helper()
		if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
	room("opened", "local")
	room("scan", "at-root")
	room("scan/alpha", "dev")
	room("scan/beta", "dev")
	room("outside", "remote")
	room("unheard", "symlink-only")
	room("scan/deep/child", "too-deep")
	for _, p := range []string{"node_modules", "vendor", ".cache", ".hidden", ".git"} {
		room("scan/"+p, "pruned")
	}
	room("scan/worktree", "copy")
	if err := os.WriteFile(filepath.Join(root, "scan/worktree/.git"), []byte("gitdir: synthetic\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mkdir("scan/listener/.moot/rooms")
	link("scan/alpha/.moot/rooms/dev", "scan/listener/.moot/rooms/dev")
	link("scan/alpha/.moot/rooms/dev", "opened/.moot/rooms/alias")
	link("outside/.moot/rooms/remote", "opened/.moot/rooms/remote")
	link("missing", "opened/.moot/rooms/dangling")
	link("scan", "scan/loop")
	link("unheard", "scan/folder-link")
	d, err := Discover(filepath.Join(root, "opened"), filepath.Join(root, "scan"), 1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range d.Rooms {
		rel, _ := filepath.Rel(root, r.Keeper)
		got = append(got, rel+"/"+r.Name+":"+r.PostName)
	}
	want := []string{"opened/local:local", "outside/remote:remote", "scan/at-root:", "scan/alpha/dev:alias", "scan/beta/dev:"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rooms %v, want %v", got, want)
	}
	// Participants: every folder walked holding .moot/rooms/, the opened one first
	// though it lies outside root; the worktree, pruned and too-deep folders are not.
	got = nil
	for _, p := range d.Participants {
		rel, _ := filepath.Rel(root, p)
		got = append(got, rel)
	}
	if want := []string{"opened", "scan", "scan/alpha", "scan/beta", "scan/listener"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("participants %v, want %v", got, want)
	}
	for _, r := range d.Rooms {
		reader, name := d.Reader(r.Key())
		if _, err := reader.List(name); err != nil {
			t.Fatal(err)
		}
	}
	// Zero means root only; hearing remains independent of the scan depth.
	d, err = Discover(filepath.Join(root, "opened"), filepath.Join(root, "scan"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rooms) != 4 {
		t.Fatalf("depth zero: %+v", d.Rooms)
	}
	if _, err = Discover(filepath.Join(root, "opened"), filepath.Join(root, "scan"), -1); err == nil {
		t.Fatal("negative depth accepted")
	}
	if _, err = Discover(filepath.Join(root, "opened"), filepath.Join(root, "scan/folder-link"), 4); err == nil {
		t.Fatal("followed symlink root")
	}
}

func TestDiscoverSkipsLinkedProtocolDirectoriesAndUnreadableFolders(t *testing.T) {
	root := t.TempDir()
	room := filepath.Join(root, "keeper/.moot/rooms/dev/posts")
	if err := os.MkdirAll(room, 0755); err != nil {
		t.Fatal(err)
	}
	opened := filepath.Join(root, "opened")
	if err := os.MkdirAll(filepath.Join(opened, ".moot/rooms"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"linked", "unreadable"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "keeper/.moot"), filepath.Join(root, "linked/.moot")); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(filepath.Join(blocked, "copy/.moot/rooms/hidden/posts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0755) })
	d, err := Discover(opened, root, 4)
	if err != nil {
		t.Fatal(err)
	}
	// Root can read mode-000 folders; the skip assertion applies to ordinary users.
	if os.Geteuid() != 0 && len(d.Rooms) != 1 {
		t.Fatalf("rooms %+v", d.Rooms)
	}
	for _, r := range d.Rooms {
		if filepath.Base(r.Keeper) == "linked" {
			t.Fatal("followed linked .moot")
		}
	}
}

func TestDiscoverMatchesKeeperAndRoomIdentityAcrossPathCase(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	for _, p := range []string{"opened/.moot/rooms/local/posts", "alpha/.moot/rooms/dev/posts", "beta/.moot/rooms/dev/posts"} {
		if err := os.MkdirAll(filepath.Join(work, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	opened := filepath.Join(root, "WORK/OPENED")
	info, err := os.Lstat(opened)
	if os.IsNotExist(err) {
		t.Skip("filesystem is case-sensitive")
	}
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Lstat(filepath.Join(work, "opened"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(info, original) {
		t.Fatal("case variants did not identify the same synthetic directory")
	}
	// The hearing target also varies keeper and room-name case from the walk.
	target := filepath.Join(root, "WORK/ALPHA/.moot/rooms/DEV")
	if err := os.Symlink(target, filepath.Join(opened, ".moot/rooms/alias")); err != nil {
		t.Fatal(err)
	}
	d, err := Discover(opened, work, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rooms) != 3 {
		t.Fatalf("case variants duplicated rooms: %+v", d.Rooms)
	}
	if d.Rooms[0].Name != "local" || d.Rooms[0].PostName != "local" {
		t.Fatalf("opened keeper lost priority or posting: %+v", d.Rooms)
	}
	count := map[string]int{}
	for _, r := range d.Rooms {
		keeper := strings.ToLower(filepath.Base(r.Keeper))
		count[keeper]++
		switch keeper {
		case "opened":
			if r.PostName != "local" {
				t.Fatalf("own room read-only: %+v", r)
			}
		case "alpha":
			if r.PostName != "alias" {
				t.Fatalf("heard room read-only: %+v", r)
			}
		case "beta":
			if r.PostName != "" {
				t.Fatalf("unheard room postable: %+v", r)
			}
		default:
			t.Fatalf("unexpected keeper: %+v", r)
		}
		reader, name := d.Reader(r.Key())
		if _, err := reader.List(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, keeper := range []string{"opened", "alpha", "beta"} {
		if count[keeper] != 1 {
			t.Fatalf("keeper counts: %v", count)
		}
	}
}
