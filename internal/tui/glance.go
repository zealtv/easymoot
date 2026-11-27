package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/twglance"
	"github.com/zealtv/easymoot/internal/twpane"
)

// What Easymoot tells tireless-watcher about itself through its own tmux pane, beside
// @tw-focus (twfocus), written by twpane (the same file as EasyLoom's). The options and
// their meanings are tireless-watcher's (docs/interface.md there): @tw-next is "what
// is ready to do", here the unread posts; @tw_status wait, with @tw_status_text and
// @tw_status_at, says someone needs the reader, here an unread post that mentions the
// opened folder's own name.

// mentionHit is one post the chained poll checked in a room with unread posts: whether
// it counts as a mention of the reader, moot.sh's counts_for_me (an @ of the name or
// of all outside code, or any post from the other side of a DM).
type mentionHit struct {
	room, id, sender string
	counts           bool
	found            int64 // when the poll first checked it, Unix seconds
}

func hitKey(room, id string) string { return room + "\x00" + id }

// mentionWord is moot.sh's mention pattern (mentions_in), before it trims a trailing
// run of '.', '_' and '-'.
var mentionWord = regexp.MustCompile(`@[A-Za-z0-9._-]+`)

// mentions says text @-mentions name, or @all, outside fenced and inline code, as
// moot.sh's mentions_in reads it.
func mentions(text, name string) bool {
	fence := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		line = inlineCode.ReplaceAllString(line, "")
		for _, w := range mentionWord.FindAllString(line, -1) {
			w = strings.TrimRight(strings.TrimPrefix(w, "@"), "._-")
			if w == name || w == "all" {
				return true
			}
		}
	}
	return false
}

var inlineCode = regexp.MustCompile("`[^`]*`")

// hears says the opened folder keeps or hears r: a room found only by --root is not
// one Bob's seat hears, so it neither counts as new nor waits on him.
func hears(r moot.Room) bool { return r.Kind.Standing != moot.Elsewhere && !r.Kind.Archived }

// checkMentions reads the posts that arrived in each heard room, other than the one
// on show (exclude; "" while a participant is selected, when no room is), whose tip
// is past what the reader has seen, and says which count as a mention of name. It
// takes the posts tipsOf listed this poll (listed): a room it didn't list has not
// changed, so it keeps its earlier hits (prior). A post already checked is not read
// again, and one whose body could not be read is tried again next time. It runs
// inside the chained poll, so it only reads.
func checkMentions(src Source, rooms []moot.Room, exclude string, tips, seen map[string]roomTip, listed map[string][]moot.Post, prior map[string]mentionHit, name string, now time.Time) map[string]mentionHit {
	out := map[string]mentionHit{}
	if name == "" {
		return out
	}
	for _, r := range rooms {
		key := r.Key()
		tip, ok := tips[key]
		s, had := seen[key]
		if key == exclude || !hears(r) || !ok || !had || !tip.past(s) {
			continue
		}
		posts, fresh := listed[key]
		if !fresh {
			for k, h := range prior {
				if h.room == key && h.id > s.newest {
					out[k] = h
				}
			}
			continue
		}
		for _, p := range posts {
			if p.ID <= s.newest || p.Sender == name {
				continue
			}
			k := hitKey(key, p.ID)
			if h, ok := prior[k]; ok {
				out[k] = h
				continue
			}
			h := mentionHit{room: key, id: p.ID, sender: p.Sender, found: now.Unix(), counts: r.Kind.DM}
			if !r.Kind.DM {
				b, err := src.Body(key, p.ID)
				if err != nil {
					continue // not cached: the next poll that lists the room tries again
				}
				h.counts = mentions(b.Text, name)
			}
			out[k] = h
		}
	}
	return out
}

// waiting is the unread posts that mention the reader, oldest first: in a room still
// marked unread, past what the reader has seen of it, and not the room on show.
func (m Model) waiting() []mentionHit {
	var out []mentionHit
	for _, h := range m.hits {
		if !h.counts || !m.unread[h.room] || h.id <= m.seen[h.room].newest {
			continue
		}
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b mentionHit) int { return strings.Compare(a.id, b.id) })
	return out
}

func (m Model) roomLabel(key string) string {
	for _, r := range m.rooms {
		if r.Key() == key {
			return short(m.label(r))
		}
	}
	return short(key)
}

// runDate is a run room's dated head, run-YYYYMMDD-, which says little on the map.
var runDate = regexp.MustCompile(`^run-[0-9]{8}-`)

// short is a room's label as the wait's text names it: a run room without its date
// (run-20260802-nightly is nightly), so the text fits a title bar.
func short(label string) string {
	if s := runDate.ReplaceAllString(label, ""); s != "" {
		return s
	}
	return label
}

// unreadCount is the unread posts in the heard rooms, at least one for each room
// marked unread, the room on show left out.
func (m Model) unreadCount() int {
	total := 0
	for _, r := range m.rooms {
		if !m.unread[r.Key()] || !hears(r) || r.Key() == m.room && !m.onPerson {
			continue
		}
		total += max(1, m.latest[r.Key()].count-m.seen[r.Key()].count)
	}
	return total
}

// nextLine is @tw-next, "3 new" (the unread posts), or "" when nothing is unread. The
// map shows it after its own "next:" label. It names no rooms, and the mentions are the
// wait's (Bob, 2026-11-27: "easymoot probably doesn't need a status, maybe n new").
func (m Model) nextLine() string {
	if total := m.unreadCount(); total > 0 {
		return fmt.Sprintf("%d new", total)
	}
	return ""
}

// waitText is @tw_status_text while a mention waits, short for a title bar and the
// mention first: "@bob ← grace · runs" for the newest one's sender and room, and "+N"
// for the others.
func (m Model) waitText(w []mentionHit) string {
	h := w[len(w)-1]
	text := fmt.Sprintf("@%s ← %s · %s", m.name, h.sender, m.roomLabel(h.room))
	if len(w) > 1 {
		text += fmt.Sprintf(" +%d", len(w)-1)
	}
	return cleanLine(text)
}

// glance is what the pane says now.
type glance struct {
	next     string
	wait     bool
	waitText string
	waitAt   int64
}

func (m Model) glance() glance {
	g := glance{next: m.nextLine()}
	if w := m.waiting(); len(w) > 0 && m.name != "" {
		g.wait, g.waitText, g.waitAt = true, m.waitText(w), w[len(w)-1].found
	}
	return g
}

// values is the glance as the options twpane writes: an empty one unsets.
func (g glance) values() twpane.Values {
	v := twpane.Values{twpane.Next: g.next}
	if g.wait {
		v[twpane.Status], v[twpane.StatusText] = "wait", g.waitText
		v[twpane.StatusAt] = strconv.FormatInt(g.waitAt, 10)
	}
	return v
}

// The glance proper: @tw-glance-far, a badge line, and @tw-glance-mid, a list, which
// tireless-watcher draws in Easymoot's own box on its map (docs/interface.md there,
// "Glance"): entries of a role the map colours and a text that opens with its mark,
// which twglance (the same file as EasyLoom's) cleans and writes.

// listening says the participant has a live watch, as the sidebar marks it ●. A paused
// watch (◐ there) is not listening, so it is left out.
func (p person) listening() bool { return p.Watch == moot.Watching }

// listeners is each participant listening, ok when its status is current, most
// pressing first: current ones, then the rest, each by name as the sidebar has them.
func (m Model) listeners() (ok, quiet []person) {
	for _, p := range m.people {
		switch {
		case !p.listening():
		case p.current:
			ok = append(ok, p)
		default:
			quiet = append(quiet, p)
		}
	}
	return ok, quiet
}

// badges is the badge line, marks and counts so that it fits one line on a far card:
// "moot", the unread posts (•N, the sidebar's unread mark) and the mentions among them
// (@N), then the listeners with a current status (●N) and those with none or an old
// one (●?N). A zero is left out.
func (m Model) badges() []twglance.Entry {
	out := []twglance.Entry{{Role: twglance.Label, Text: "moot"}}
	if total := m.unreadCount(); total > 0 {
		out = append(out, twglance.Entry{Role: twglance.Label, Text: fmt.Sprintf("•%d", total)})
	}
	if w := m.waiting(); len(w) > 0 && m.name != "" {
		out = append(out, twglance.Entry{Role: twglance.Hold, Text: fmt.Sprintf("@%d", len(w))})
	}
	ok, quiet := m.listeners()
	if len(ok) > 0 {
		out = append(out, twglance.Entry{Role: twglance.OK, Text: fmt.Sprintf("●%d", len(ok))})
	}
	if len(quiet) > 0 {
		out = append(out, twglance.Entry{Role: twglance.Quiet, Text: fmt.Sprintf("●?%d", len(quiet))})
	}
	return out
}

// listRows is the list: one row per listener, current ones (ok) then the rest
// (quiet), each "name › status", a head and a body that the map sets in two
// columns (tireless-watcher's docs/glance.md, "A head and a body"; only the first
// " › " splits, so a status may hold one). A listener with no status is its name
// alone. The map draws each row's mark from its role (● and ○). No posts: the live
// view shows those.
func (m Model) listRows() []twglance.Entry {
	ok, quiet := m.listeners()
	var out []twglance.Entry
	row := func(role string, p person) {
		text := p.label()                                      // a name holding " › " would split there; moot's names don't
		if s := twglance.Clean(cleanLine(p.Status)); s != "" { // as the encoder will clean it
			text += headBody + s
		}
		out = append(out, twglance.Entry{Role: role, Text: text})
	}
	for _, p := range ok {
		row(twglance.OK, p)
	}
	for _, p := range quiet {
		row(twglance.Quiet, p)
	}
	return out
}

// headBody ends a list row's head: a "›" (U+203A) with a space either side.
const headBody = " \u203a "

// glanceParts is the badge line and the list as twglance writes them.
func (m Model) glanceParts() twglance.Glance {
	return twglance.Glance{Badges: m.badges(), Rows: m.listRows()}
}
