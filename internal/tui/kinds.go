package tui

import (
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zealtv/easymoot/internal/moot"
)

// Room kinds shape the sidebar. m.rooms holds the rooms on show, in the order the
// sidebar lists them, and m.all everything the last listing found. The groups, in
// order (docs/design.md, "Room kinds"): the opened folder's rooms (with --root, each keeper's rooms
// under its name, the opened folder's first); direct, the DMs; other keepers' rooms;
// archived, while shown. Archived rooms go in one group at the end, so showing
// them only adds rows below and nothing above moves.

// Group ranks, in sidebar order.
const (
	rankRooms    = iota // the opened folder's rooms; with no --root, every live room
	rankDirect          // DMs
	rankKeepers         // rooms kept elsewhere (--root)
	rankArchived        // archived rooms
)

// group is a run of the sidebar under one header.
type group struct {
	rank   int
	keeper string // the keeper's path, for a rooted rooms or keepers group
}

func groupOf(r moot.Room, rooted bool) group {
	switch {
	case r.Kind.Archived:
		return group{rank: rankArchived}
	case r.Kind.DM:
		return group{rank: rankDirect}
	case !rooted:
		return group{rank: rankRooms}
	case r.Kind.Standing == moot.Kept:
		return group{rank: rankRooms, keeper: r.Keeper}
	}
	return group{rank: rankKeepers, keeper: r.Keeper}
}

// arrange is the rooms on show, grouped. Archived rooms are left out unless show.
// With rooted (--root) keepers are told apart. The order within a group is the
// listing's, except in direct, which is newest post first (Bob, 2026-11-22), then by
// name; latest is each room's last listing, for that order.
func arrange(all []moot.Room, show, rooted bool, latest map[string]roomTip) []moot.Room {
	out := make([]moot.Room, 0, len(all))
	for _, r := range all {
		if r.Kind.Archived && !show {
			continue
		}
		out = append(out, r)
	}
	recent := func(r moot.Room) string {
		if t := latest[r.Key()].newest; moot.IsStampID(t) {
			return t
		}
		return ""
	}
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := groupOf(out[i], rooted), groupOf(out[j], rooted)
		if gi.rank != gj.rank {
			return gi.rank < gj.rank
		}
		if gi.rank == rankDirect {
			if ri, rj := recent(out[i]), recent(out[j]); ri != rj {
				return ri > rj
			}
			return out[i].Name < out[j].Name
		}
		return false
	})
	return out
}

// withRooms takes m.all as the rooms on show and keeps the selection on its room.
func (m Model) withRooms() Model {
	if rooms := arrange(m.all, m.showArchived, m.root != "", m.latest); !sameRooms(m.rooms, rooms) {
		m.rooms = rooms // an unchanged set keeps its slice
	}
	m.sel = roomIndex(m.rooms, m.room)
	return m
}

// archivedCount is how many archived rooms the last listing found, shown or not.
func (m Model) archivedCount() int {
	n := 0
	for _, r := range m.all {
		if r.Kind.Archived {
			n++
		}
	}
	return n
}

// liveCount is how many rooms on show are not archived.
func (m Model) liveCount() int {
	n := 0
	for _, r := range m.rooms {
		if !r.Kind.Archived {
			n++
		}
	}
	return n
}

// roomKind is the selected room's kind; ok is false when none is selected.
func (m Model) roomKind() (moot.Room, bool) {
	if m.sel < 0 || m.sel >= len(m.rooms) {
		return moot.Room{}, false
	}
	return m.rooms[m.sel], true
}

// label is what the sidebar and the pane title call a room: an archived room without
// its .archived ending, a DM as @other (ownName is this folder's name from its files),
// another pair's DM as a ↔ b.
func (m Model) label(r moot.Room) string {
	name := strings.TrimSuffix(r.Name, moot.ArchivedSuffix)
	if r.Kind.DM {
		name = dmLabel(r, name, m.ownName)
	}
	return name
}

// dmLabel names a DM room, dm-<a>-<b> with the two names in byte order. Names may hold
// hyphens, so it takes the half off one end it knows. For a room kept elsewhere that
// is its keeper's (a DM is kept by one of its two sides): the other half shows as
// @keeper when it is this folder's own name, else as keeper ↔ other. Otherwise it is
// this folder's own name, and the other half shows as @other. A room neither end
// names shows as it is written.
func dmLabel(r moot.Room, name, own string) string {
	rest := strings.TrimPrefix(name, moot.DMPrefix)
	if r.Kind.Standing == moot.Elsewhere {
		if r.Keeper == "" {
			return rest
		}
		keeper := filepath.Base(r.Keeper)
		o := otherSide(rest, keeper)
		switch {
		case o == "":
			return rest
		case o == own:
			return "@" + keeper
		default:
			return keeper + " ↔ " + o
		}
	}
	if o := otherSide(rest, own); o != "" {
		return "@" + o
	}
	return rest
}

// otherSide is the half of a DM's names that is not own, or "" if own is on neither end.
func otherSide(rest, own string) string {
	if own == "" {
		return ""
	}
	if o, ok := strings.CutPrefix(rest, own+"-"); ok && o != "" {
		return o
	}
	if o, ok := strings.CutSuffix(rest, "-"+own); ok && o != "" {
		return o
	}
	return ""
}

// pinned is the index in posts of the room's pinned post: the newest one whose
// sender is `pinned` (moot's README); -1 when there is none.
func (m Model) pinned() int {
	for i := len(m.posts) - 1; i >= 0; i-- {
		if p := m.posts[i]; p.Stamp && p.Sender == "pinned" {
			return i
		}
	}
	return -1
}

// pinnedDate is the pinned post's day, in the reader's zone, as the transcript shows it.
func (m Model) pinnedDate() string {
	i := m.pinned()
	if i < 0 {
		return ""
	}
	if p := m.posts[i]; !p.Time.IsZero() {
		return p.Time.In(m.loc).Format("2006-01-02")
	}
	return m.posts[i].WrittenTime()[:10]
}

// toggleArchived shows or hides the archived rooms. The selection stays on its room
// by key, so a room that is not archived never moves; the rows below it come or go.
// If the selected room is itself archived and gets hidden, its neighbour in the list
// (the last room left, as archived ones sit at the end) is selected and read. On a
// moot with only archived rooms nothing is selected until they show, and then the
// first is selected and read.
func (m Model) toggleArchived() (tea.Model, tea.Cmd) {
	prev := m.sel
	m.showArchived = !m.showArchived
	m = m.withRooms()
	if m.sel < 0 && m.room != "" || m.room == "" && len(m.rooms) > 0 {
		// The selected room left the list, or there was none and now rooms show.
		m = m.takeNeighbour(prev)
		return m, m.readSelected()
	}
	return m.track(m.latest).rebuild(), nil
}

// takeNeighbour moves the selection off a room that has left the list, to the room now
// at its old index prev (or the last one left), and clears the posts for the caller's
// read of it.
func (m Model) takeNeighbour(prev int) Model {
	i := min(max0(prev), len(m.rooms)-1)
	m.room, m.sel = "", -1
	if i >= 0 {
		m.room, m.sel = m.rooms[i].Key(), i
	}
	return m.clearPosts().rebuild().track(m.latest)
}

// showPinned scrolls the transcript to the room's pinned post, held like any other
// scroll: `end` returns to the latest. In the one-pane layout it also brings the
// posts into view, which is what was asked for.
func (m Model) showPinned() Model {
	i := m.pinned()
	if i < 0 || i >= len(m.starts) {
		return m
	}
	if m.layout().mode == layoutSingle {
		m.focus = panePosts
	}
	// Said even when the pinned post is already on screen, so the command answers.
	m.notice, m.failure = "the pinned post of "+cleanLine(m.roomName())+", "+m.pinnedDate()+" · end returns to the latest", false
	return m.scrollTo(m.starts[i]).settle()
}
