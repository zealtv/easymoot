//go:build unix

package acceptance

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pinned is the upstream moot.sh the whole session runs, the same copy
// internal/moot tests against.
const pinned = "../moot/testdata/upstream-c25b5bb/moot.sh"

// gate skips the test unless EASYMOOT_ACCEPTANCE=1.
func gate(t *testing.T) {
	t.Helper()
	if os.Getenv("EASYMOOT_ACCEPTANCE") != "1" {
		t.Skip("set EASYMOOT_ACCEPTANCE=1 (or run `make acceptance`) to run the acceptance session")
	}
}

// world is the synthetic home: ~/.moot, three repos that share rooms through a
// store, one repo with a moot and no rooms, and one with a private room.
type world struct {
	t     *testing.T
	root  string
	home  string // HOME for the whole session; its base name, "ada", is the name of ~/.moot
	store string // the shared store the repos' rooms link into
}

// oldSeen is the modification time every seen marker starts with, so a rewrite
// of any of them shows.
var oldSeen = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newWorld(t *testing.T) *world {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, root: root, home: filepath.Join(root, "ada")}
	w.store = filepath.Join(w.home, "repos", ".moots")
	t.Setenv("HOME", w.home)
	// An empty MOOT_NAME is skipped by moot's name chain, so the developer's own
	// environment never names a post.
	t.Setenv("MOOT_NAME", "")
	t.Setenv("NO_COLOR", "1")

	// The shared rooms, as ~/repos/.moots/ holds the real ones.
	w.mkdir("repos/.moots/dev/posts")
	w.mkdir("repos/.moots/runs/posts")
	// dev is longer than any pane: fourteen posts of three lines, so it scrolls.
	senders := []string{"ada", "grace", "linus"}
	for i := 0; i < 14; i++ {
		who := senders[i%3]
		w.write(fmt.Sprintf("repos/.moots/dev/posts/202608%02d-09%04d-%s.md", 1+i/7, i, who),
			fmt.Sprintf("Update %02d on the index rebuild.\nSecond line of update %02d.\n— %s\n", i, i, who))
	}
	w.write("repos/.moots/runs/posts/20260802-081000-ci-bot.md", "run 412 passed\nrun 413 passed\n")

	w.moot("", []string{"lobby"}, nil)
	w.moot("repos/atlas", []string{"lobby"}, []string{"dev", "runs"})
	w.moot("repos/harbor", nil, []string{"dev", "runs"})
	w.moot("repos/ledger", nil, nil)
	w.moot("work/notes", []string{"lobby"}, nil)
	w.write("work/notes/.moot/rooms/lobby/posts/20260801-100000-grace.md", "notes room: nothing to see\n")
	w.write("repos/atlas/.moot/rooms/lobby/posts/20260801-100000-grace.md", "hello lobby\n")

	// seen markers: one per participant and room, in every moot that has rooms,
	// all with an old modification time.
	for _, folder := range []string{"", "repos/atlas", "repos/harbor", "work/notes"} {
		rooms, _ := os.ReadDir(filepath.Join(w.home, folder, ".moot", "rooms"))
		for _, r := range rooms {
			for _, who := range []string{"grace", "ada"} {
				p := filepath.Join(folder, ".moot", "seen", r.Name(), who)
				w.write(p, "20260801-100000-grace\n")
				if err := os.Chtimes(filepath.Join(w.home, p), oldSeen, oldSeen); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return w
}

// moot makes <folder>/.moot with a copy of the pinned script, the private rooms
// named, and links for the shared rooms into the store. An empty folder is home.
func (w *world) moot(folder string, private, shared []string) {
	w.t.Helper()
	script, err := os.ReadFile(pinned)
	if err != nil {
		w.t.Fatal(err)
	}
	dir := filepath.Join(folder, ".moot")
	w.mkdir(filepath.Join(dir, "rooms"))
	if err := os.WriteFile(filepath.Join(w.home, dir, "moot.sh"), script, 0o755); err != nil {
		w.t.Fatal(err)
	}
	for _, r := range private {
		w.mkdir(filepath.Join(dir, "rooms", r, "posts"))
	}
	for _, r := range shared {
		if err := os.Symlink(filepath.Join(w.store, r), filepath.Join(w.home, dir, "rooms", r)); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *world) path(rel string) string { return filepath.Join(w.home, rel) }

func (w *world) mkdir(rel string) {
	w.t.Helper()
	if err := os.MkdirAll(w.path(rel), 0o755); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) write(rel, text string) {
	w.t.Helper()
	w.mkdir(filepath.Dir(rel))
	if err := os.WriteFile(w.path(rel), []byte(text), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// outsidePost writes a post the way another agent would: by running the folder's
// own pinned script, under its own name, from outside Easymoot.
func (w *world) outsidePost(folder, room, as, text string) {
	w.t.Helper()
	cmd := exec.Command(w.path(filepath.Join(folder, ".moot", "moot.sh")), "post", "--room", room, "-")
	cmd.Dir = w.path(folder)
	cmd.Env = append(os.Environ(), "MOOT_NAME="+as)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		w.t.Fatalf("outside post to %s/%s as %s: %v\n%s", folder, room, as, err, out)
	}
}

// snapshot describes every entry under the home, symlinks not followed: kind and
// mode for directories, mode, size, modification time and a content hash for
// files, the target for links. A directory's own time is left out, since a new
// post legitimately touches its parent.
func (w *world) snapshot() map[string]string {
	w.t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(w.home, func(path string, d fs.DirEntry, err error) error {
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
		w.t.Fatal(err)
	}
	// The shared store and the posts linked into it are inside the home, so one
	// walk covers them; links are not followed, so a linked room's files are seen
	// once, under the store.
	return snap
}

// seenOnly keeps the entries under a seen/ directory.
func seenOnly(snap map[string]string) map[string]string {
	out := map[string]string{}
	for p, v := range snap {
		if strings.Contains(p, string(filepath.Separator)+"seen"+string(filepath.Separator)) ||
			strings.HasSuffix(p, string(filepath.Separator)+"seen") {
			out[p] = v
		}
	}
	return out
}

// diff lists what differs between two snapshots, sorted.
func diff(a, b map[string]string) []string {
	var out []string
	for p, v := range a {
		if nv, ok := b[p]; !ok {
			out = append(out, "gone: "+p)
		} else if nv != v {
			out = append(out, fmt.Sprintf("changed: %s\n  was %s\n  now %s", p, v, nv))
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			out = append(out, "new: "+p)
		}
	}
	sort.Strings(out)
	return out
}

// driver runs the Bubble Tea model as the program would, without a terminal: it
// feeds messages to Update on the test's goroutine, and runs each command it
// returns in a goroutine of its own, whose answer comes back as a message. The
// poll's ticks are real, a few milliseconds apart, so a post that arrives from
// outside shows up by itself.
type driver struct {
	t    *testing.T
	m    tea.Model
	msgs chan tea.Msg
	done chan struct{}
	quit bool
}

// newDriver reports to t until a step sets another; t is also what ends the
// driver, so it has to be the test the whole session runs under, not one step.
func newDriver(t *testing.T, m tea.Model) *driver {
	d := &driver{t: t, m: m, msgs: make(chan tea.Msg, 4096), done: make(chan struct{})}
	t.Cleanup(func() { close(d.done) })
	d.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	d.run(m.Init())
	return d
}

func (d *driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		select {
		case d.msgs <- msg:
		case <-d.done:
		}
	}()
}

// deliver hands one message to the model, or expands a batch.
func (d *driver) deliver(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	case tea.QuitMsg:
		d.quit = true
	default:
		next, cmd := d.m.Update(msg)
		d.m = next
		d.run(cmd)
	}
}

// send gives the model a message at once (a key, a size).
func (d *driver) send(msg tea.Msg) { d.deliver(msg) }

// until delivers answers until cond holds, and fails the test with the screen if
// it does not within timeout.
func (d *driver) until(what string, cond func() bool) {
	d.t.Helper()
	deadline := time.After(30 * time.Second)
	for !cond() {
		select {
		case msg := <-d.msgs:
			d.deliver(msg)
		case <-deadline:
			d.t.Fatalf("timed out waiting for %s; the screen:\n%s", what, d.screen())
		}
	}
}

func (d *driver) screen() string { return d.m.View() }

// untilScreen waits for the screen to contain every one of parts.
func (d *driver) untilScreen(what string, parts ...string) {
	d.t.Helper()
	d.until(what, func() bool {
		s := d.screen()
		for _, p := range parts {
			if !strings.Contains(s, p) {
				return false
			}
		}
		return true
	})
}

func (d *driver) typeText(s string) {
	for _, r := range s {
		if r == ' ' {
			d.send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func (d *driver) key(k tea.KeyType) { d.send(tea.KeyMsg{Type: k}) }

// palette runs a palette command the way a person would: a colon, the name, Enter.
func (d *driver) palette(name string) {
	d.typeText(":" + name)
	d.key(tea.KeyEnter)
}

// lineWith returns the first screen line containing part, or "".
func (d *driver) lineWith(part string) string {
	for _, l := range strings.Split(d.screen(), "\n") {
		if strings.Contains(l, part) {
			return l
		}
	}
	return ""
}

// pickerCursor is the picker's selected row: the line with the marker that also
// names a room count, which a room row in the sidebar behind it does not.
func (d *driver) pickerCursor() string {
	for _, l := range strings.Split(d.screen(), "\n") {
		if strings.Contains(l, "❯") && (strings.Contains(l, " room") || strings.Contains(l, "no rooms")) {
			return l
		}
	}
	return ""
}

func (d *driver) has(parts ...string) bool {
	s := d.screen()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
