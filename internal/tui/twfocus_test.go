package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/twfocus"
)

// The focus is the posts pane, anchored at the bottom: its last row is the last
// row of the newest post's area, and its left edge is right of the sidebar's rule.
func TestFocusRectIsThePosts(t *testing.T) {
	f := newFixture(t, "t")
	f.room("r")
	f.post("r", "20260801-090000-ada", "the newest words\n— ada\n")
	for _, size := range [][2]int{{100, 30}, {178, 53}, {120, 12}} {
		m := resized(t, shown(t, f.open()), size[0], size[1])
		r := m.focusRect()
		if !r.Bottom || r.W < 1 || r.X+r.W != size[0] || r.Y+r.H > size[1] {
			t.Fatalf("%v: focus %+v", size, r)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		inside := ""
		for y := r.Y; y < r.Y+r.H; y++ {
			inside += ansi.Cut(rows[y], r.X, r.X+r.W) + "\n"
		}
		if !strings.Contains(inside, "the newest words") || strings.Contains(inside, "Participants") {
			t.Errorf("%v: the focus holds\n%s", size, inside)
		}
	}
}

// In one-pane-at-a-time mode the posts are named only while they are on screen.
func TestFocusRectFollowsTheSinglePane(t *testing.T) {
	f := newFixture(t, "t")
	f.room("r")
	m := resized(t, shown(t, f.open()), 50, 30)
	m.focus = panePosts
	if r, b := m.focusRect(), m.layout().body; r != (twfocus.Rect{X: b.x, Y: b.y, W: b.w, H: b.h, Bottom: true}) {
		t.Errorf("posts showing: %+v, want the body %+v", r, b)
	}
	m.focus = paneRooms
	if r := m.focusRect(); r.W != 0 {
		t.Errorf("sidebar showing: %+v, want none", r)
	}
}
