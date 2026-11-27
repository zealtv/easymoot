// Package twglance publishes a program's glance for tireless-watcher's map:
// what the map draws of the program inside its own pane's box, in two pane
// options. @tw-glance-far is the badge line (a label and a few marked
// counts); @tw-glance-mid is the list (one row per thing). Their meanings, and
// how the map draws them, are the map's (its docs/interface.md, "Glance");
// this package only encodes and writes them.
//
// Each option holds entries separated by ";", each "role:text". The colon is
// always written, so text holding a colon keeps its head. The text is cleaned
// here, not by the reader: escape sequences are taken out, control characters
// and every kind of white space become spaces, invisible format characters
// (zero-widths, bidi controls) are dropped, ";" becomes ",", white space is
// squeezed, and each entry is cut to the cells the map keeps,
// ending "…". An entry with no text is left out. The badge line keeps its
// first MaxBadges entries. The list keeps every row that fits the 8 KiB the
// map reads, so the map can count the rows it cannot show ("+N").
//
// Both options go on the program's own pane (set-option -p), never its
// window. Outside tmux (no pane ID) nothing happens. An empty value unsets the
// option. Only changes are sent, both in one tmux call, and Clear unsets both,
// for the way out, after which nothing more is written.
//
// The same file is used unchanged by EasyLoom, Easymoot and Easyhog, so the
// three write alike. Change it in EasyLoom first.
package twglance

import (
	"os/exec"
	"strings"
	"sync"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The two options.
const (
	Far = "@tw-glance-far"
	Mid = "@tw-glance-mid"
)

// The roles the map knows. An empty role draws plain and dim: a label.
const (
	Label = ""
	OK    = "ok"    // as meant: ready, ticked here, current
	Busy  = "busy"  // in hand now
	Hold  = "hold"  // paused, parked, blocked
	Bad   = "bad"   // failed, broken, needs repair
	Other = "other" // someone else's: outside, tended
	Quiet = "quiet" // idle, not ready, unknown, old
)

// What the map keeps (docs/interface.md, "Limits").
const (
	MaxBadges  = 8
	BadgeCells = 16
	RowCells   = 60
	MaxBytes   = 8 << 10
)

// Entry is one badge or one row. Its text may start with a mark (a run of up
// to two symbol cells, such as "◐"), which takes the role's colour.
type Entry struct {
	Role string
	Text string
}

// Glance is what a program wants the map to draw of it. A nil or empty part
// unsets its option.
type Glance struct {
	Badges []Entry
	Rows   []Entry
}

// FarValue is the badge line's option value.
func FarValue(badges []Entry) string {
	return encode(badges, MaxBadges, BadgeCells)
}

// MidValue is the list's option value.
func MidValue(rows []Entry) string {
	return encode(rows, -1, RowCells)
}

// encode joins up to max entries (all, when max < 0), each cleaned and cut to
// cells, stopping before the value would pass MaxBytes.
func encode(entries []Entry, max, cells int) string {
	var b strings.Builder
	n := 0
	for _, e := range entries {
		if max >= 0 && n == max {
			break
		}
		text := cut(Clean(e.Text), cells)
		if text == "" {
			continue
		}
		entry := clean(e.Role, ":") + ":" + text
		size := len(entry)
		if n > 0 {
			size++
		}
		if b.Len()+size > MaxBytes {
			break
		}
		if n > 0 {
			b.WriteByte(';')
		}
		b.WriteString(entry)
		n++
	}
	return b.String()
}

// Clean is s as the map would draw it: one line, escape sequences taken out,
// controls and white space as spaces, format characters (zero-widths, bidi
// controls) dropped, ";" (which would end the entry) as ",", runs of white
// space squeezed to one space, and none at either end.
func Clean(s string) string { return clean(s, "") }

// clean is Clean that also drops every rune in drop (a role loses its colons).
func clean(s, drop string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case strings.ContainsRune(drop, r):
			continue
		case r == ';':
			r = ','
		case unicode.IsSpace(r) || unicode.IsControl(r):
			space = true
			continue
		case unicode.Is(unicode.Cf, r):
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// cut is s at most cells wide, ending "…" when cut.
func cut(s string, cells int) string {
	if ansi.StringWidth(s) <= cells {
		return s
	}
	return ansi.Truncate(s, cells, "…")
}

// Pane writes the glance of one tmux pane. Its zero value, or one with no ID,
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
	want   *[2]string // far, mid: the newest values asked for
	have   *[2]string // what tmux holds; nil until the first write succeeds
	closed bool
}

var names = [2]string{Far, Mid}

// Sync returns a command that brings the pane's glance to g, or nil when
// there is no pane or tmux already holds g. A write that failed is tried
// again by the next Sync, changed or not. The first write
// states both options, unsetting an empty one, so a glance an earlier run
// left behind (it crashed, or was killed) does not linger on the map.
func (p *Pane) Sync(g Glance) tea.Cmd {
	if p == nil || p.ID == "" {
		return nil
	}
	if p.s == nil {
		p.s = &state{}
	}
	want := [2]string{FarValue(g.Badges), MidValue(g.Rows)}
	s := p.s
	s.mu.Lock()
	unchanged := s.have != nil && *s.have == want
	s.want = &want
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
	if s.closed || s.want == nil {
		return
	}
	var args []string
	for i, name := range names {
		if s.have != nil && s.have[i] == s.want[i] {
			continue
		}
		args = p.appendSet(args, name, s.want[i])
	}
	if len(args) == 0 {
		return
	}
	if p.runner()(args...) != nil { // best effort: the map shows what it shows without them
		return
	}
	have := *s.want
	s.have = &have
}

// Clear unsets both options, now, and stops every write still to come. Call
// it on the way out.
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
	for _, name := range names {
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
	// tmux takes an argument ending in ";" as the end of a command. A value
	// from this package never ends in one (Clean turns it to ","), but a
	// value is escaped all the same rather than trusted not to.
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
