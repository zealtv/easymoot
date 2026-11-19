package moot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// snapshot describes every entry under roots (symlinks not followed): kind, mode,
// size, modification time and content hash for files, the target for links.
// Directories carry no mtime, since a new post legitimately touches its parent.
func snapshot(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				target, _ := os.Readlink(path)
				snap[path] = "link -> " + target
			case info.IsDir():
				snap[path] = fmt.Sprintf("dir %v", info.Mode())
			default:
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				snap[path] = fmt.Sprintf("file %v %d %d %x", info.Mode(), info.Size(), info.ModTime().UnixNano(), sha256.Sum256(data))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snap
}

// seenFixture builds a moot with seen markers: a shared room (a link into a store
// outside .moot) and a private room, each holding posts.
func seenFixture(t *testing.T) (f *fixture, store string) {
	f = newFixture(t, "proj")
	store = filepath.Join(f.root, "store")
	if err := os.MkdirAll(filepath.Join(store, "dev", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(store, "dev"), f.dir+"/rooms/dev"); err != nil {
		t.Fatal(err)
	}
	f.room("local")
	for _, room := range []string{"dev", "local"} {
		f.write(".moot/rooms/"+room+"/posts/20260829-100000-alice.md", "hello from alice\n")
		f.write(".moot/rooms/"+room+"/posts/20260829-100001-bob.md", "hello from bob\n")
		f.write(".moot/rooms/"+room+"/posts/20260830-000000-dir/post.md", "dir post\n")
		f.write(".moot/seen/"+room+"/alice", "20260829-100000-alice\n")
		f.write(".moot/seen/"+room+"/proj", "20260829-100001-bob\n")
	}
	// Old marker times, so a rewrite of any of them shows.
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, room := range []string{"dev", "local"} {
		for _, who := range []string{"alice", "proj"} {
			if err := os.Chtimes(f.dir+"/seen/"+room+"/"+who, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f, store
}

// A session of open, rooms, list, read, name and post, twice over (a poll after
// the post), changes nothing under .moot/ (or in a linked room's store) except the
// one new post file.
func TestSessionLeavesSeenMarkersAndEverythingElseAlone(t *testing.T) {
	for _, room := range []string{"local", "dev"} {
		t.Run("post to "+room, func(t *testing.T) {
			setMootName(t, nil)
			f, store := seenFixture(t)
			before := snapshot(t, f.dir, store)

			m, err := Open(f.folder)
			if err != nil {
				t.Fatal(err)
			}
			session := func() {
				rooms, err := m.Rooms()
				if err != nil || len(rooms) != 2 {
					t.Fatalf("rooms %v, %v", rooms, err)
				}
				for _, r := range rooms {
					posts, err := m.List(r.Name)
					if err != nil {
						t.Fatal(err)
					}
					for _, p := range posts {
						if _, err := m.Body(r.Name, p.ID); err != nil {
							t.Fatal(err)
						}
					}
				}
				if _, err := m.Name(); err != nil {
					t.Fatal(err)
				}
			}
			session()
			if got := snapshot(t, f.dir, store); !sameSnapshot(got, before) {
				t.Fatalf("reading changed something:\n%s", diffSnapshots(before, got))
			}

			res, err := m.Post(context.Background(), room, "a new post\n")
			if err != nil {
				t.Fatal(err)
			}
			session()

			after := snapshot(t, f.dir, store)
			added := ""
			for path := range after {
				if _, ok := before[path]; !ok {
					if added != "" {
						t.Errorf("more than one new path: %s and %s", added, path)
					}
					added = path
				}
			}
			wantAdded := filepath.Join(store, "dev", "posts", res.ID+".md")
			if room == "local" {
				wantAdded = filepath.Join(f.dir, "rooms", "local", "posts", res.ID+".md")
			}
			if added != wantAdded {
				t.Errorf("new path %q, want %q", added, wantAdded)
			}
			for path, was := range before {
				if now, ok := after[path]; !ok || now != was {
					t.Errorf("%s changed: was %q, now %q (present %v)", path, was, now, ok)
				}
			}
			for path := range after {
				if strings.Contains(path, ".landing") {
					t.Errorf("left-over landing file %s", path)
				}
			}
		})
	}
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func diffSnapshots(a, b map[string]string) string {
	var sb strings.Builder
	for k, v := range a {
		if b[k] != v {
			fmt.Fprintf(&sb, "%s: %q -> %q\n", k, v, b[k])
		}
	}
	for k, v := range b {
		if _, ok := a[k]; !ok {
			fmt.Fprintf(&sb, "%s: new %q\n", k, v)
		}
	}
	return sb.String()
}
