package tui

// Default size for the single frame printed to redirected output, and for a
// window that has not reported its size yet.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// The sidebar's width bounds in the side-by-side layout: a quarter of the window,
// never under minSideWidth (a short room name still reads) nor over maxSideWidth.
// A width set by a resize (shift+←/→ or a drag on the divider) holds for the session
// within sideBounds instead.
const (
	minSideWidth = 18
	maxSideWidth = 32
	// leastSide is the narrowest a resize makes the sidebar; the widest is half the window.
	leastSide = 12
	// splitWidth is the narrowest window that shows both panes at once.
	splitWidth = 64
)

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type layoutMode int

const (
	layoutSplit  layoutMode = iota // rooms left, posts right
	layoutSingle                   // one pane at a time; tab swaps
	layoutTiny                     // too small for panes
)

// layout is the screen divided up. Both panes' first line is their title.
type layout struct {
	mode           layoutMode
	w, h           int
	header         bool // one header line on top
	statusRow      bool // a status line above the key hints
	body           rect
	side, posts    rect // zero when hidden
	showSide, show bool
}

// computeLayout follows Easyhog's (internal/tui/layout.go at 01336f8), without
// the stacked mode: side by side when the window allows, one pane at a time (tab
// swaps) when it does not, and a plain notice when there is no room for anything.
// side is the sidebar width a resize set, 0 for the default.
func computeLayout(w, h int, focusPosts bool, side int) layout {
	l := layout{w: w, h: h}
	if w < 20 || h < 3 {
		l.mode = layoutTiny
		return l
	}
	switch {
	case h >= 10:
		l.header, l.statusRow = true, true
		l.body = rect{0, 1, w, h - 3}
	case h >= 6:
		l.header = true
		l.body = rect{0, 1, w, h - 2}
	default:
		l.body = rect{0, 0, w, h - 1}
	}
	b := l.body
	if w >= splitWidth {
		l.mode = layoutSplit
		sw := min(maxSideWidth, max(minSideWidth, w/4))
		if side > 0 {
			sw = sideBounds(side, w)
		}
		l.side = rect{b.x, b.y, sw, b.h}
		l.posts = rect{b.x + sw + 1, b.y, w - sw - 1, b.h}
		l.showSide, l.show = true, true
		return l
	}
	l.mode = layoutSingle
	if focusPosts {
		l.posts, l.show = b, true
	} else {
		l.side, l.showSide = b, true
	}
	return l
}

// sideBounds clamps a resized sidebar width for a window w cells wide.
func sideBounds(side, w int) int { return max(leastSide, min(side, w/2)) }
