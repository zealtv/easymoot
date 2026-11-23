package moot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestReadPresence covers the presence files one at a time over synthetic folders:
// the name chain without $MOOT_NAME, a status with and without its stamp, and a
// live, a stale and a paused watch.
func TestReadPresence(t *testing.T) {
	t.Setenv("MOOT_NAME", "from-the-environment")
	// A pid that has exited: a stale watching file.
	done := exec.Command("true")
	if err := done.Run(); err != nil {
		t.Fatal(err)
	}
	stale := done.Process.Pid
	live := os.Getpid()
	watching := func(pid int) string {
		return "pid=" + strconv.Itoa(pid) + "\nparent=1\nname=x\nstarted=20261122-090000\n"
	}
	stamp := time.Date(2026, 11, 22, 9, 30, 12, 0, time.UTC)
	cases := []struct {
		name  string
		files map[string]string
		want  Presence
	}{
		{"bare", nil, Presence{Name: "folder"}},
		{"named in .moot/name", map[string]string{"name": "  ada \n"}, Presence{Name: "ada"}},
		{"unsafe name falls back to the folder", map[string]string{"name": "a/b\n"}, Presence{Name: "folder"}},
		{"stamped status", map[string]string{"status": "20261122-093012 reviewing the archive\nmore text\n"},
			Presence{Name: "folder", Status: "reviewing the archive", Stamp: stamp}},
		{"status without a stamp", map[string]string{"status": "just text, no stamp\n"},
			Presence{Name: "folder", Status: "just text, no stamp"}},
		{"stamp-shaped but no date", map[string]string{"status": "20261399-093012 odd\n"},
			Presence{Name: "folder", Status: "20261399-093012 odd"}},
		{"empty status", map[string]string{"status": "\n"}, Presence{Name: "folder"}},
		{"live watch", map[string]string{"watching": watching(live)}, Presence{Name: "folder", Watch: Watching}},
		{"paused watch", map[string]string{"watching": watching(live), "watch.paused": ""}, Presence{Name: "folder", Watch: WatchPaused}},
		{"stale watch", map[string]string{"watching": watching(stale), "watch.paused": ""}, Presence{Name: "folder"}},
		{"no pid", map[string]string{"watching": "pid=\n"}, Presence{Name: "folder"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			folder := filepath.Join(t.TempDir(), "folder")
			if err := os.MkdirAll(filepath.Join(folder, ".moot", "rooms"), 0o755); err != nil {
				t.Fatal(err)
			}
			for name, text := range c.files {
				if err := os.WriteFile(filepath.Join(folder, ".moot", name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := ReadPresence(folder); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}
