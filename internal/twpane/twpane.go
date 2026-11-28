// Package twpane keeps a few tmux user options on this program's own pane, for
// tireless-watcher's map to read: @tw-idle (what is ready to do) and, while the
// program is acting, @tw_status, @tw_status_text and @tw_status_at. Their
// meanings are the map's (its docs/interface.md); this package only writes them.
//
// Every option goes on the program's own pane (set-option -p), never its
// window, so it says nothing about the panes beside it. Outside tmux (no
// $TMUX_PANE) nothing happens. An empty value unsets the option. Only changes
// are sent, all of them in one tmux call, and Clear unsets every option this
// package owns, for the way out, after which nothing more is written.
package twpane

import (
	"maps"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The options this package owns, in the order a call sets them.
const (
	Idle       = "@tw-idle"
	Status     = "@tw_status"
	StatusText = "@tw_status_text"
	StatusAt   = "@tw_status_at"
)

var owned = []string{Idle, Status, StatusText, StatusAt}

// MaxCells is how much of a text option the map keeps (docs/interface.md:
// "120 cells kept"). Longer values are cut here, with an ellipsis, rather
// than by the reader.
const MaxCells = 120

// Values is the options' wanted values by name. A missing or empty value
// means unset.
type Values map[string]string

// Pane writes the options of one tmux pane. Its zero value, or one with no ID,
// writes nothing, which is what tests and non-tmux sessions want.
//
// A Pane is copied with the model that holds it, so what it shares lives
// behind a pointer: the values wanted and the values tmux has.
type Pane struct {
	ID string // $TMUX_PANE, e.g. "%12"
	// Run replaces the tmux call in tests.
	Run func(args ...string) error

	s *state
}

// New returns a Pane for the pane id whose copies all share one state, so
// Clear on any of them stops the writes of every other.
func New(id string) Pane { return Pane{ID: id, s: &state{}} }

// state is what a Pane's copies share. Bubble Tea runs each command in a
// goroutine of its own and never waits for them, so two writes can race, and
// one can outlive the program. Every write therefore takes the lock, applies
// the newest wanted values rather than the ones it was made with, and does
// nothing once Clear has run.
type state struct {
	mu     sync.Mutex
	want   map[string]string // the newest values asked for, already cut
	have   map[string]string // what tmux holds; nil until the first write succeeds
	closed bool
}

// Sync returns a command that brings the pane's options to v, or nil when
// there is no pane or nothing changed since the last Sync. Options this
// package does not own are ignored. The first write states every option,
// unsetting the empty ones, so a value an earlier run left behind (it crashed,
// or was killed) does not linger on the map.
func (p *Pane) Sync(v Values) tea.Cmd {
	if p == nil || p.ID == "" {
		return nil
	}
	if p.s == nil {
		p.s = &state{}
	}
	want := map[string]string{}
	for _, name := range owned {
		want[name] = Cut(v[name])
	}
	s := p.s
	s.mu.Lock()
	unchanged := s.want != nil && maps.Equal(s.want, want)
	if !unchanged {
		s.want = want
	}
	s.mu.Unlock()
	if unchanged {
		return nil
	}
	return func() tea.Msg {
		p.write(s)
		return nil
	}
}

// write sends what differs between the newest wanted values and what tmux
// holds, in one call. A failed call leaves have as it was, so the next write
// tries again.
func (p *Pane) write(s *state) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	var args []string
	for _, name := range owned {
		want := s.want[name]
		if had, ok := s.have[name]; ok && had == want {
			continue
		}
		args = p.appendSet(args, name, want)
	}
	if len(args) == 0 {
		return
	}
	if p.runner()(args...) != nil { // best effort: the map shows what it shows without them
		return
	}
	s.have = maps.Clone(s.want)
}

// Clear unsets every option this package owns, now, and stops every write
// still to come. Call it on the way out.
func (p *Pane) Clear() error {
	if p == nil || p.ID == "" {
		return nil
	}
	if p.s == nil {
		p.s = &state{}
	}
	s := p.s
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var args []string
	for _, name := range owned {
		args = p.appendSet(args, name, "")
	}
	return p.runner()(args...)
}

// appendSet adds one set-option (or its unset) to a tmux command list,
// separated from the one before by tmux's own ";".
func (p *Pane) appendSet(args []string, name, value string) []string {
	if len(args) > 0 {
		args = append(args, ";")
	}
	if value == "" {
		return append(args, "set-option", "-pu", "-t", p.ID, name)
	}
	// tmux takes an argument ending in ";" as the end of a command, even
	// inside a list it was handed whole: "a;" would become "a", and ";"
	// alone would stop the list. Escaped, it is kept as written.
	if strings.HasSuffix(value, ";") {
		value = strings.TrimSuffix(value, ";") + `\;`
	}
	return append(args, "set-option", "-p", "-t", p.ID, name, value)
}

func (p *Pane) runner() func(args ...string) error {
	if p.Run != nil {
		return p.Run
	}
	return func(args ...string) error { return exec.Command("tmux", args...).Run() }
}

// Cut is s on one line and at most MaxCells wide, ending "…" when cut. A
// newline, tab or other control character becomes a space: an option is one
// line on the map.
func Cut(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c < ' ' || c == 0x7f {
			r[i] = ' '
		}
	}
	s = string(r)
	if ansi.StringWidth(s) <= MaxCells {
		return s
	}
	return ansi.Truncate(s, MaxCells, "…")
}
