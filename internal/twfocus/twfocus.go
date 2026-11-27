// Package twfocus tells a tool that previews this program's tmux pane in a
// small space which part of the screen matters, so the preview shows that part
// instead of the screen's left edge and bottom rows.
//
// The contract: the pane option @tw-focus on this program's own pane holds
// "x,y,w,h", a rectangle in the pane's cells from its top-left corner, or
// "x,y,w,h,1" when its newest rows are at the bottom (a transcript). A reader uses
// it only while the pane is on the alternate screen, shows the rectangle from its
// top-left corner (or its last rows that hold anything, for ",1"), and ignores a
// value that does not parse or does not fit the pane. An empty value unsets the option. Outside tmux (no $TMUX_PANE)
// nothing happens.
//
// The same file is kept in EasyLoom, EasyMoot and Easyhog.
package twfocus

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// Rect is a rectangle of the pane's cells. Bottom says its newest rows are at
// the bottom, so a short preview should show its last rows. The zero Rect names
// nothing.
type Rect struct {
	X, Y, W, H int
	Bottom     bool
}

// String is the option's value: "x,y,w,h" (",1" added for Bottom), or "" for an
// empty rectangle.
func (r Rect) String() string {
	if r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 {
		return ""
	}
	if r.Bottom {
		return fmt.Sprintf("%d,%d,%d,%d,1", r.X, r.Y, r.W, r.H)
	}
	return fmt.Sprintf("%d,%d,%d,%d", r.X, r.Y, r.W, r.H)
}

// Pane reports the focus of one tmux pane. Its zero value, or one with no ID,
// reports nothing, which is what tests and non-tmux sessions want.
type Pane struct {
	ID   string // $TMUX_PANE, e.g. "%12"
	last string // the value last sent, so an unchanged layout costs nothing
	sent bool
	// Run replaces the tmux call in tests.
	Run func(args ...string) error
}

// Report returns a command that sets the option to r, or nil when there is no
// pane or r is what was last sent.
func (p *Pane) Report(r Rect) tea.Cmd {
	if p == nil || p.ID == "" {
		return nil
	}
	v := r.String()
	if p.sent && v == p.last {
		return nil
	}
	p.last, p.sent = v, true
	args := []string{"set-option", "-p", "-t", p.ID, "@tw-focus", v}
	if v == "" {
		args = []string{"set-option", "-pu", "-t", p.ID, "@tw-focus"}
	}
	run := p.Run
	if run == nil {
		run = func(args ...string) error { return exec.Command("tmux", args...).Run() }
	}
	return func() tea.Msg {
		_ = run(args...) // best effort: a reader falls back to its normal view
		return nil
	}
}
