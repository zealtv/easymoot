package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// The participants are the folders the walk found, listed in the sidebar below the
// rooms, one row each, sorted by name; the selected one's strip shows in the right
// pane in place of the posts (docs/design.md, "Participants").

// person is one participant as the poll read it: finished, so View reads no clock.
type person struct {
	folder string
	you    bool // the opened folder
	moot.Presence
	age string // how old the status's stamp was at the poll: "40s", "12m", "3h", "6d"; "" with none
}

// peopleOf reads each participant's presence, at now. The walk lists the opened folder
// first.
func peopleOf(folders []string, now time.Time) []person {
	out := make([]person, 0, len(folders))
	for i, f := range folders {
		p := person{folder: f, you: i == 0, Presence: moot.ReadPresence(f)}
		if !p.Stamp.IsZero() {
			p.age = ageOf(now.Sub(p.Stamp))
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func ageOf(d time.Duration) string {
	switch s := int(max(0, d/time.Second)); {
	case s < 60:
		return strconv.Itoa(s) + "s"
	case s < 3600:
		return strconv.Itoa(s/60) + "m"
	case s < 86400:
		return strconv.Itoa(s/3600) + "h"
	default:
		return strconv.Itoa(s/86400) + "d"
	}
}

// withPeople takes a poll's rows, keeping the selection on its folder.
func (m Model) withPeople(rows []person) Model {
	m.people = rows
	if m.personIndex() < 0 && len(rows) > 0 {
		m.person = rows[0].folder
	}
	return m.settle()
}

// personIndex is where the selected participant sits in people, -1 if nowhere.
func (m Model) personIndex() int {
	for i, p := range m.people {
		if p.folder == m.person {
			return i
		}
	}
	return -1
}

// selectedPerson is the selected participant; ok is false before any has been read.
func (m Model) selectedPerson() (person, bool) {
	if i := m.personIndex(); i >= 0 {
		return m.people[i], true
	}
	return person{}, false
}

// pickPerson selects participant i in the sidebar. The right pane shows its strip;
// the poll goes on reading the room left, so going back to it shows its posts at once.
func (m Model) pickPerson(i int) Model {
	m.onPerson, m.person = true, m.people[i].folder
	return m.settle()
}

func (m Model) watchingCount() int {
	n := 0
	for _, p := range m.people {
		if p.Watch == moot.Watching {
			n++
		}
	}
	return n
}

func mark(w moot.Watch) string {
	switch w {
	case moot.Watching:
		return "●"
	case moot.WatchPaused:
		return "◐"
	}
	return "○"
}

func (p person) label() string {
	name := cleanLine(p.Name)
	if p.you {
		name += " (you)"
	}
	return name
}

// personRow is participant i in the sidebar: the marker, the mark and the name.
func (m Model) personRow(i, w int) string {
	s := m.styles
	selected := m.onPerson && i == m.personIndex()
	bar, marker, name := s.barDim, "  ", lipgloss.NewStyle()
	if m.focus == paneRooms {
		bar = s.bar
	}
	if selected {
		marker, name = "❯ ", s.selected
	}
	label := m.people[i].label()
	if room := max(0, w-4-1); lipgloss.Width(label) > room {
		label = ansi.Truncate(label, room, "…")
	}
	pad := strings.Repeat(" ", max(0, w-4-lipgloss.Width(label)))
	return clip(renderSegs([]seg{{marker, s.selected}, {mark(m.people[i].Watch) + " ", s.soft}, {label, name},
		{pad, lipgloss.NewStyle()}}, selected, bar.GetBackground()), w)
}

// personPane is the selected participant's strip in the right pane, h lines of w
// cells: one header line, <mark> <name> · said <age> ago · <folder>, then line one of
// its status, faint when no live watch vouches for it.
func (m Model) personPane(w, h int) []string {
	s := m.styles
	p, _ := m.selectedPerson()
	extra := []string{}
	if p.age != "" {
		extra = append(extra, "said "+p.age+" ago")
	}
	extra = append(extra, cleanLine(m.displayPath(p.folder)))
	out := []string{clip(m.title(mark(p.Watch)+" "+p.label(), "· "+strings.Join(extra, " · "), m.focus == panePosts, w), w)}
	text, st := []string{"no status"}, s.faint
	if p.Status != "" {
		text = wrapPlain(cleanLine(p.Status), max(1, w-2))
		if p.Watch != moot.NotWatching {
			st = lipgloss.NewStyle()
		}
	}
	for _, line := range text {
		out = append(out, fit(" "+st.Render(line), w))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out[:h]
}
