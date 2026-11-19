//go:build unix

package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/tui"
)

// TestMVPAcceptance is one session over a synthetic home, from a folder with a
// .moot/ to Quit. The steps run in order and share the world; a step that fails
// stops the rest, since later steps build on earlier ones.
//
// The claims come from docs/design.md and docs/architecture.md as they stood on
// 2026-11-19, and from the pinned moot.sh (upstream c25b5bb). A newer upstream
// that disagrees means the pin moves deliberately, with this test.
func TestMVPAcceptance(top *testing.T) {
	gate(top)
	t := top
	bin := buildBinary(t) // before HOME points at the synthetic home
	w := newWorld(t)
	before := w.snapshot()

	var d *driver
	step := func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, func(t *testing.T) {
			if d != nil {
				d.t = t // the driver reports to the step that is running
			}
			fn(t)
		}) {
			t.Fatalf("stopping: step %q failed, and the steps after it depend on it", name)
		}
	}
	opts := []tui.Option{tui.WithLocation(time.UTC), tui.WithPollInterval(25 * time.Millisecond)}
	open := func(folder string) tui.Model {
		src, err := moot.Open(w.path(folder))
		if err != nil {
			top.Fatal(err)
		}
		return tui.New(src, opts...)
	}

	step("start in atlas: its rooms and the first room's posts", func(t *testing.T) {
		d = newDriver(top, open("repos/atlas"))
		d.t = t
		d.untilScreen("atlas's first room", "as atlas", "dev 14 posts", "Update 13 on the index rebuild.")
		for _, room := range []string{"dev", "lobby", "runs"} {
			if !d.has(room) {
				t.Errorf("the sidebar lacks %q\n%s", room, d.screen())
			}
		}
		if !d.has("3 rooms") || !d.has("~/repos/atlas") {
			t.Errorf("the header lacks the room count or the folder\n%s", d.screen())
		}
		// A heading: the sender at the left, the time at the right, seconds dropped.
		sender := d.has("── ada ─") || d.has("── grace ─") || d.has("── linus ─")
		if !sender || !d.has(" 2026-08-02 09:00 ──") {
			t.Errorf("no post heading with the sender left and the time right\n%s", d.screen())
		}
	})

	step("read another room and come back", func(t *testing.T) {
		d.key(tea.KeyDown)
		d.untilScreen("the lobby", "lobby 1 post", "hello lobby")
		if d.has("Update 13") {
			t.Errorf("the dev room's posts stayed on screen under lobby\n%s", d.screen())
		}
		d.key(tea.KeyUp)
		d.untilScreen("dev again", "dev 14 posts", "Update 13 on the index rebuild.")
	})

	step("a post from outside arrives without a keypress", func(t *testing.T) {
		w.outsidePost("repos/atlas", "dev", "grace", "Build is red again on the second pass.\n— grace\n")
		d.untilScreen("the outside post", "dev 15 posts", "Build is red again on the second pass.", "── grace ─")
		if d.has("scrolled up") {
			t.Errorf("a view at the bottom stopped following\n%s", d.screen())
		}
	})

	step("scroll up: the view holds while a post arrives below", func(t *testing.T) {
		d.key(tea.KeyTab) // focus the posts
		d.key(tea.KeyPgUp)
		d.key(tea.KeyPgUp)
		d.untilScreen("the held view", "scrolled up, end follows")
		held := withoutTitleRow(d.screen())
		if d.has("Build is red again") {
			t.Fatalf("two pages up still shows the newest post\n%s", d.screen())
		}
		w.outsidePost("repos/atlas", "dev", "linus", "Second outside post, arriving while you read.\n— linus\n")
		d.untilScreen("the poll to read it", "dev 16 posts", "scrolled up, end follows")
		if got := withoutTitleRow(d.screen()); got != held {
			t.Errorf("the held view moved when a post arrived\nbefore:\n%s\nafter:\n%s", held, got)
		}
		d.key(tea.KeyEnd)
		d.untilScreen("end follows again", "Second outside post, arriving while you read.")
		if d.has("scrolled up") {
			t.Errorf("end did not return to following\n%s", d.screen())
		}
		d.key(tea.KeyTab) // back to the rooms
	})

	// Reading, scrolling and polling so far wrote nothing: the only new files in
	// the home are the two posts that arrived from outside.
	step("reading wrote nothing", func(t *testing.T) {
		added := newPaths(before, w.snapshot())
		if len(added) != 2 {
			t.Errorf("new paths after reading: %v, want the two outside posts", added)
		}
		for _, p := range added {
			if !strings.HasPrefix(p, filepath.Join(w.store, "dev", "posts")+string(filepath.Separator)) {
				t.Errorf("new path outside the shared dev room: %s", p)
			}
		}
	})

	step("the palette offers Post and Quit; the bar drops with Esc", func(t *testing.T) {
		d.typeText(":")
		d.untilScreen("the palette", "Post", "Quit")
		if d.has("Switch") {
			t.Errorf("the palette still offers a switch command\n%s", d.screen())
		}
		d.typeText("post")
		d.key(tea.KeyEnter)
		d.untilScreen("the bar", "post to dev as atlas", "enter post", "esc cancel")
		d.typeText("this one is never sent")
		d.untilScreen("the typed text", "this one is never sent")
		d.key(tea.KeyEsc)
		d.untilScreen("the bar closed", "read-only")
		if d.has("post to dev as") || d.has("never sent") {
			t.Errorf("Esc left the bar or its text\n%s", d.screen())
		}
		if got := len(newPaths(before, w.snapshot())); got != 2 {
			t.Errorf("dropping the bar wrote something: %d new paths, want 2", got)
		}
	})

	var barPost string
	step("post through the bar, under the folder's name", func(t *testing.T) {
		d.palette("post")
		d.untilScreen("the bar", "post to dev as atlas")
		const text = "Thanks both. Fix is in; rebuilding now."
		d.typeText(text)
		d.key(tea.KeyEnter)
		d.untilScreen("the post to land", "posted to dev as atlas", "dev 17 posts", text, "── atlas ─")
		if d.has("post to dev as atlas") && d.has("esc cancel") {
			t.Errorf("the bar is still open after the post\n%s", d.screen())
		}
		// The file went into the shared store, as the script's own post would.
		posts, err := filepath.Glob(filepath.Join(w.store, "dev", "posts", "*-atlas.md"))
		if err != nil || len(posts) != 1 {
			t.Fatalf("posts by atlas in the shared room: %v, %v", posts, err)
		}
		barPost = posts[0]
		got, err := os.ReadFile(barPost)
		if err != nil || string(got) != text+"\n" {
			t.Errorf("the post's bytes are %q (%v), want %q", got, err, text+"\n")
		}
	})

	step("open harbor: it shows the post atlas made, under harbor's name", func(t *testing.T) {
		h := newDriver(top, open("repos/harbor"))
		h.t = t
		h.untilScreen("harbor", "as harbor", "2 rooms", "~/repos/harbor", "dev 17 posts")
		for _, want := range []string{"Build is red again on the second pass.", "Thanks both. Fix is in; rebuilding now."} {
			if !h.has(want) {
				t.Errorf("harbor's dev room lacks %q (a room linked into two repos shows once in each)\n%s", want, h.screen())
			}
		}
		if h.has("lobby") {
			t.Errorf("atlas's private lobby shows under harbor\n%s", h.screen())
		}
		// The bar there names harbor.
		h.palette("post")
		h.untilScreen("harbor's bar", "post to dev as harbor")
		h.key(tea.KeyEsc)
	})

	step("Quit from the palette ends the session", func(t *testing.T) {
		d.palette("quit")
		d.until("the quit message", func() bool { return d.quit })
	})

	step("ctrl+c quits", func(t *testing.T) {
		p := newDriver(top, open("repos/atlas"))
		p.t = t
		p.untilScreen("the shell", "as atlas")
		p.key(tea.KeyCtrlC)
		p.until("ctrl+c to quit", func() bool { return p.quit })
	})

	step("the binary: with and without a path", func(t *testing.T) { binarySteps(t, w, bin) })

	// The end: nothing under any seen/ directory changed, and the only new paths
	// in the home are the three posts of the session.
	after := w.snapshot()
	if changes := diff(seenOnly(before), seenOnly(after)); len(changes) > 0 {
		t.Errorf("a seen marker changed:\n%s", strings.Join(changes, "\n"))
	}
	if len(seenOnly(before)) < 8 {
		t.Errorf("the fixture holds only %d seen entries; the check proves little", len(seenOnly(before)))
	}
	added := newPaths(before, after)
	if len(added) != 3 {
		t.Errorf("new paths: %v, want the three posts", added)
	}
	for _, p := range added {
		if filepath.Dir(p) != filepath.Join(w.store, "dev", "posts") {
			t.Errorf("new path outside the shared dev room: %s", p)
		}
	}
	if barPost != "" && !contains(added, barPost) {
		t.Errorf("the bar's post %s is not among the new paths", barPost)
	}
	// Everything that was there before is still there, byte for byte.
	for p, was := range before {
		if now, ok := after[p]; !ok || now != was {
			t.Errorf("%s changed: was %q, now %q", p, was, now)
		}
	}
}

// withoutTitleRow is the screen less its second line, the pane titles, whose post
// count changes when a post arrives.
func withoutTitleRow(screen string) string {
	lines := strings.Split(screen, "\n")
	if len(lines) > 1 {
		lines = append(lines[:1:1], lines[2:]...)
	}
	return strings.Join(lines, "\n")
}

// newPaths lists the paths in after that before lacks, sorted by diff.
func newPaths(before, after map[string]string) []string {
	var out []string
	for _, c := range diff(before, after) {
		if p, ok := strings.CutPrefix(c, "new: "); ok {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
