package tui

import (
	"path/filepath"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zealtv/easymoot/internal/moot"
)

// WithRoot enables recursive discovery. A zero depth searches the root itself.
func WithRoot(root string, depth int) Option {
	return func(m *Model) { m.root, m.depth = root, depth }
}

type discoveryRequest struct {
	root     string
	depth    int
	prior    *moot.Discovery
	walk     bool
	sequence uint64
}

func (m Model) poll(room string, known map[string]postBody, prior map[string]roomTip, chain, walk bool) tea.Cmd {
	return m.pollAs(room, known, prior, chain, walk, pollOpts{})
}

// pollAs is poll with the options a read after an action, or at open, asks for.
func (m Model) pollAs(room string, known map[string]postBody, prior map[string]roomTip, chain, walk bool, o pollOpts) tea.Cmd {
	o.show, o.people = m.showArchived, m.peopleOnShow()
	if chain {
		o.seen, o.hits, o.onPerson = m.seen, m.hits, m.onPerson
	}
	if m.root == "" {
		return pollCmd(m.src, room, known, prior, chain, o)
	}
	req := discoveryRequest{root: m.root, depth: m.depth, prior: m.discovery, walk: walk || m.discovery == nil}
	if req.walk {
		req.sequence = m.walkSequence.Add(1)
	}
	return pollCmd(m.src, room, known, prior, chain, o, req)
}

// discoveredSource is a command's immutable read view. Posting always uses the
// original opened source and its local room name, never a keeper's script.
type discoveredSource struct {
	Source
	discovery *moot.Discovery
}

func (s discoveredSource) Rooms() ([]moot.Room, error) { return s.discovery.Rooms, nil }
func (s discoveredSource) List(key string) ([]moot.Post, error) {
	r, name := s.discovery.Reader(key)
	return r.List(name)
}
func (s discoveredSource) Body(key, id string) (moot.Body, error) {
	r, name := s.discovery.Reader(key)
	return r.Body(name, id)
}
func (s discoveredSource) PostsModTime(key string) (time.Time, error) {
	r, name := s.discovery.Reader(key)
	return r.PostsModTime(name)
}
func sameRooms(a, b []moot.Room) bool { return slices.Equal(a, b) }

// roomName is what the selected room is called on screen (see label).
func (m Model) roomName() string {
	if i := roomIndex(m.rooms, m.room); i >= 0 {
		return m.label(m.rooms[i])
	}
	return m.room
}
func (m Model) postName(key string) string {
	if m.root == "" {
		return key
	}
	if i := roomIndex(m.rooms, key); i >= 0 {
		return m.rooms[i].PostName
	}
	return ""
}
func (m Model) selectedPost(room string) string {
	if m.root != "" && m.postName(m.room) == room {
		return m.room
	}
	return room
}

// Headers, the divider and notices are display rows, never selections. Arrow keys
// select rooms and participants; mouse hit testing and scrolling use this same
// projection, as in the peers. m.rooms is already in group order (kinds.go), so the
// projection only adds a header where a group starts. With a walk, the divider and
// the participants follow the rooms.
type sideRow struct {
	header  string // a header's text; "" on any other row
	faint   bool   // the header is the archived one
	line    string // a notice line, drawn as it is (no rooms to list)
	divider bool   // the rule above the participants
	room    int    // the room's index in m.rooms, -1 on any other row
	person  int    // the participant's index in m.people, -1 on any other row
}

const (
	directHeader   = "direct"
	archivedHeader = "archived"
)

func (m Model) sidebar() []sideRow {
	var rows []sideRow
	var last *group
	rooted := m.root != ""
	none := sideRow{room: -1, person: -1}
	for i, r := range m.rooms {
		g := groupOf(r, rooted)
		if last == nil || g != *last {
			last = &g
			h := none
			switch {
			case g.rank == rankDirect:
				h.header = directHeader
			case g.rank == rankArchived:
				h.header, h.faint = archivedHeader, true
			case rooted:
				h.header = filepath.Base(g.keeper)
			}
			if h.header != "" {
				rows = append(rows, h)
			}
		}
		rows = append(rows, sideRow{room: i, person: -1})
	}
	if len(m.rooms) == 0 {
		for _, line := range m.noRoomsLines(m.sideLayout().side.w - 2) {
			r := none
			r.line = line
			rows = append(rows, r)
		}
	}
	if rooted {
		d := none
		d.divider = true
		rows = append(rows, d)
		for i := range m.people {
			rows = append(rows, sideRow{room: -1, person: i})
		}
	}
	return rows
}

// selectedRow is where the selection sits in rows: the selected participant's row, or
// the selected room's; -1 when neither is listed.
func selectedRow(rows []sideRow, m Model) int {
	p := m.personIndex()
	for i, r := range rows {
		if m.onPerson && r.person >= 0 && r.person == p || !m.onPerson && r.room >= 0 && r.room == m.sel {
			return i
		}
	}
	return -1
}

// peopleOnShow says the poll should read presence: a participant is selected, the
// divider is in the sidebar's window (the participants follow it), or Easymoot runs
// inside tmux, where its glance lists the listeners for tireless-watcher's map (glance.go).
func (m Model) peopleOnShow() bool {
	if m.root == "" {
		return false
	}
	if m.onPerson || m.twg.ID != "" { // inside tmux: the glance lists the listeners
		return true
	}
	rows := m.sidebar()
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].divider {
			return i < m.sideTop+m.sideRows()
		}
	}
	return false
}

// indented says a room's row sits under a header, so its name steps in.
func (m Model) indented(i int) bool {
	g := groupOf(m.rooms[i], m.root != "")
	return m.root != "" || g.rank != rankRooms
}
