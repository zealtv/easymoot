package tui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// View draws the whole frame as exactly height lines of at most width cells. It
// reads the model only: no I/O, no clock. The layout, title rules and key hints are
// Easyhog's (internal/tui/render.go at 01336f8), with rooms and posts in place of
// its tree and preview.
func (m Model) View() string {
	w, h := m.size()
	l := m.layout()
	if l.mode == layoutTiny {
		return m.tinyView(w, h)
	}
	var frame []string
	if l.header {
		frame = append(frame, m.header(w))
	}
	frame = append(frame, m.body(l)...)
	frame = append(frame, m.footer(l, w)...)
	for len(frame) < h {
		frame = append(frame, strings.Repeat(" ", w))
	}
	frame = frame[:h]
	if m.palette != nil {
		frame = m.overlay(frame, w)
	}
	for i := range frame {
		frame[i] = clip(frame[i], w)
	}
	return strings.Join(frame, "\n")
}

// clip truncates a rendered line to width cells; it never wraps.
func clip(line string, width int) string {
	if lipgloss.Width(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return line
}

// fit truncates with an ellipsis and pads to exactly width cells.
func fit(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	return padTo(line, width)
}

func (m Model) tinyView(w, h int) string {
	lines := []string{"easymoot", "window too small", "ctrl+c exits"}
	if m.bar != nil {
		// Nothing of the bar fits, so say it is open and how to drop it.
		lines = []string{"bar open", "window too small", "esc drops · ctrl+c exits"}
	}
	for i := range lines {
		lines[i] = fit(lines[i], w)
	}
	if h < len(lines) {
		lines = lines[:max(h, 1)]
	}
	if m.palette != nil || h < 1 {
		return strings.Join(lines[:1], "\n")
	}
	return strings.Join(lines, "\n")
}

// header is one line: the brand, the room count and the name a post would go out
// under, then the folder on the right.
func (m Model) header(w int) string {
	s := m.styles
	live := m.liveCount()
	parts := []string{m.tally(live, plural(live, "room", "rooms"), s.soft)}
	if m.onPerson {
		n := len(m.people)
		parts = []string{m.tally(n, plural(n, "participant", "participants"), s.soft), m.tally(m.watchingCount(), "watching", s.soft)}
	} else if n := m.archivedCount(); n > 0 {
		// Hidden archived rooms are counted so they are known to exist; shown ones add on.
		if m.showArchived {
			parts[0] += s.faint.Render(" + ") + m.tally(n, "archived", s.faint)
		} else {
			parts = append(parts, s.faint.Render(strconv.Itoa(n)+" archived"))
		}
	}
	switch {
	case m.nameErr != "":
		parts = append(parts, s.warn.Render("no usable name"))
	case m.name != "":
		parts = append(parts, s.soft.Render("as ")+s.selected.Render(cleanLine(m.name)))
	}
	left := " " + s.brand.Render("Easymoot") + "  " + strings.Join(parts, s.faint.Render(" · "))
	// The folder is cleaned, so path names cannot inject terminal escapes.
	folder := cleanLine(m.displayPath(m.src.Folder()))
	room := w - lipgloss.Width(left) - 3
	if room < 8 {
		return fit(left, w)
	}
	if lipgloss.Width(folder) > room {
		folder = "…" + ansi.TruncateLeft(folder, lipgloss.Width(folder)-room+1, "")
	}
	pad := w - lipgloss.Width(left) - lipgloss.Width(folder) - 1
	return left + strings.Repeat(" ", max(1, pad)) + s.faint.Render(folder) + " "
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// tally is one header count: the number in the hue of what it counts, the word
// after it in the quiet grey. A count of zero recedes to faint.
func (m Model) tally(n int, word string, hue lipgloss.Style) string {
	if n == 0 {
		hue = m.styles.faint
	}
	return hue.Render(strconv.Itoa(n)) + m.styles.soft.Render(" "+word)
}

// body draws the panes into l.body.h lines.
func (m Model) body(l layout) []string {
	right := m.postsPane
	if m.onPerson {
		right = m.personPane
	}
	if l.mode == layoutSplit {
		left := m.sidePane(l.side.w, l.side.h)
		right := right(l.posts.w, l.posts.h)
		out := make([]string, l.body.h)
		for i := range out {
			sep := m.styles.faint.Render("│")
			if i == 0 {
				sep = m.styles.faint.Render("┬")
			}
			out[i] = left[i] + sep + right[i]
		}
		return out
	}
	if l.showSide {
		return m.sidePane(l.side.w, l.side.h)
	}
	return right(l.posts.w, l.posts.h)
}

// title draws a pane's top line as a rule with a label. The focused pane's label is
// lit and its rule is the heavy one, so which pane the arrows and the scroll keys
// act on still shows with colour off.
func (m Model) title(label, extra string, focused bool, w int) string {
	s := m.styles
	st, rule := s.faint, "─"
	if focused {
		st, rule = s.brand, "━"
	}
	left := s.faint.Render(strings.Repeat(rule, 2)+" ") + st.Render(label)
	if extra != "" {
		left += " " + s.soft.Render(extra)
	}
	left += " "
	return left + s.faint.Render(strings.Repeat(rule, max(0, w-lipgloss.Width(left))))
}

// sidePane lists the rooms and, below a divider, the participants.
func (m Model) sidePane(w, h int) []string {
	out := make([]string, 0, h)
	out = append(out, clip(m.title("Rooms", strconv.Itoa(len(m.rooms)), m.focus == paneRooms, w), w))
	rows := m.sidebar()
	for i := 0; i < h-1 && m.sideTop+i < len(rows); i++ {
		switch row := rows[m.sideTop+i]; {
		case row.room >= 0:
			out = append(out, m.roomRow(row.room, w))
		case row.person >= 0:
			out = append(out, m.personRow(row.person, w))
		case row.divider:
			out = append(out, clip(m.title("Participants", strconv.Itoa(len(m.people)), false, w), w))
		case row.line != "":
			out = append(out, fit(" "+row.line, w))
		default:
			st := m.styles.soft
			if row.faint {
				st = m.styles.faint
			}
			out = append(out, fit(" "+st.Render(cleanLine(row.header)), w))
		}
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out[:h]
}

// noRoomsLines is the sidebar's content when there is nothing to list: a read that
// failed, or a moot with no rooms, which Easymoot makes none for.
func (m Model) noRoomsLines(w int) []string {
	s := m.styles
	switch {
	case m.loadErr != "" && !m.loaded:
		return append([]string{s.warn.Render("! could not read")}, wrapPlain(m.loadErr, w)...)
	case !m.loaded:
		return []string{s.faint.Render("reading…")}
	}
	if m.archivedCount() > 0 {
		return append([]string{s.soft.Render("Only archived rooms.")}, wrapPlain("Show archived, from the : commands, lists them.", w)...)
	}
	lines := []string{s.soft.Render("No rooms.")}
	return append(lines, wrapPlain("Shared rooms are linked in by hand: see .moot/SKILL.md, \"The wiring\".", w)...)
}

func wrapPlain(text string, w int) []string {
	if w < 1 {
		return nil
	}
	return strings.Split(ansi.Wrap(text, w, ""), "\n")
}

func (m Model) roomRow(i, w int) string {
	s := m.styles
	selected := i == m.sel && !m.onPerson
	bar := s.barDim
	if m.focus == paneRooms {
		bar = s.bar
	}
	marker, markStyle := "  ", s.selected
	unread := !selected && m.unread[m.rooms[i].Key()]
	switch {
	case selected:
		marker = "❯ "
	case unread:
		marker, markStyle = "• ", s.unread
	}
	if m.indented(i) {
		marker = "  " + marker
	}
	label := cleanLine(m.label(m.rooms[i]))
	room := max(0, w-lipgloss.Width(marker)-1)
	if lipgloss.Width(label) > room {
		label = ansi.Truncate(label, room, "…")
	}
	name := lipgloss.NewStyle()
	switch {
	case selected:
		name = s.selected
	case unread:
		name = s.unread
	}
	switch {
	case m.rooms[i].Kind.Archived:
		if !selected {
			name = s.faint // the word "archived" marks it too: colour is never the only sign
		}
	case m.root != "" && m.rooms[i].PostName == "":
		name = name.Italic(true)
	}
	used := lipgloss.Width(marker) + lipgloss.Width(label)
	pad := strings.Repeat(" ", max(0, w-used))
	return clip(renderSegs([]seg{{marker, markStyle}, {label, name}, {pad, lipgloss.NewStyle()}}, selected, bar.GetBackground()), w)
}

// postsPane shows the transcript from m.top.
func (m Model) postsPane(w, h int) []string {
	rows := h - 1
	label, extra := "Posts", ""
	if m.room != "" {
		label = cleanLine(m.roomName())
		var bits []string
		if r, _ := m.roomKind(); r.Kind.Archived {
			bits = append(bits, "archived")
		}
		bits = append(bits, strconv.Itoa(len(m.posts))+" "+plural(len(m.posts), "post", "posts"))
		if d := m.pinnedDate(); d != "" {
			bits = append(bits, "pinned "+cleanLine(d))
		}
		if !m.follow {
			bits = append(bits, "scrolled up, end follows")
		}
		extra = strings.Join(bits, " · ")
	}
	out := []string{clip(m.title(label, extra, m.focus == panePosts, w), w)}
	if len(m.lines) == 0 {
		for _, line := range m.noPostsLines(w - 2) {
			out = append(out, fit(" "+line, w))
		}
	}
	top := max0(min(m.top, m.maxTop()))
	for i := 0; i < rows && top+i < len(m.lines); i++ {
		out = append(out, fit(" "+m.lines[top+i], w))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out[:h]
}

func (m Model) noPostsLines(w int) []string {
	s := m.styles
	switch {
	case m.room == "":
		return []string{s.faint.Render("No room selected.")}
	case m.loadErr != "" && !m.read:
		return []string{s.warn.Render("! could not read this room")}
	case !m.read:
		return []string{s.faint.Render("reading…")}
	}
	return []string{s.faint.Render("No posts yet.")}
}

type hint struct{ key, label string }

// footer is the status line (when there is room) and the key hints.
func (m Model) footer(l layout, w int) []string {
	status := m.status(w)
	hints := m.hintLine(w)
	if m.bar != nil {
		// The bar takes the hints' row. Where there is no status row, a status that
		// is waiting to be read (posting…, a failure) shows in its place until the
		// next key.
		switch {
		case l.statusRow:
			return []string{status, m.barLine(w)}
		case m.notice != "":
			return []string{status}
		}
		return []string{m.barLine(w)}
	}
	if l.statusRow {
		return []string{status, hints}
	}
	if m.notice != "" || m.loadErr != "" {
		return []string{status}
	}
	return []string{hints}
}

// status shows the latest notice, else a read error, else a quiet line.
func (m Model) status(w int) string {
	s := m.styles
	var text string
	switch {
	case m.notice != "" && m.failure:
		text = s.warn.Render(" ! " + m.notice)
	case m.notice != "":
		text = s.soft.Render(" " + m.notice)
	case m.loadErr != "":
		text = s.warn.Render(" ! showing what was last read: " + m.loadErr)
	case m.bar != nil && m.bar.note != "":
		text = s.faint.Render(" " + m.bar.note)
	case m.bar != nil:
		text = s.faint.Render(" a post can't be taken back")
	default:
		text = s.faint.Render(" keys only look · : opens commands")
		if line := m.archivedLine(w); line != "" && !m.onPerson {
			text = s.soft.Render(" " + line)
		}
	}
	return fit(text, w)
}

// hintLine fits as many hints as the width allows, most important first. As in
// EasyLoom the key is brighter than its label and pairs sit three spaces apart, so
// the line scans by key rather than by punctuation.
func (m Model) hintLine(w int) string {
	s := m.styles
	all := []hint{{":", "commands"}, {"↑↓", "rooms"}, {"tab", "focus"}, {"pgup/pgdn", "scroll"}, {"end", "latest"}, {"ctrl+c", "quit"}}
	var b strings.Builder
	b.WriteString(" ")
	used := 1
	for i, h := range all {
		piece := s.hintKey.Render(h.key) + s.hint.Render(" "+h.label)
		pw := lipgloss.Width(h.key) + 1 + lipgloss.Width(h.label)
		if i > 0 {
			pw += 3
		}
		if used+pw > w {
			break
		}
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(piece)
		used += pw
	}
	return fit(b.String(), w)
}

// archivedLine is the status line for an open archived room: what it is, and the way
// back where there is one. For a room the opened folder keeps that is the mv that
// reopens it, since no moot verb does (Bob's pick, 2026-11-22). "" for any other room.
func (m Model) archivedLine(w int) string {
	r, ok := m.roomKind()
	if !ok || !r.Kind.Archived {
		return ""
	}
	line := "archived · posting is closed"
	switch {
	case r.Kind.Standing == moot.Kept:
		name := cleanLine(r.Name)
		line = "archived · reopen: mv " + name + " " + strings.TrimSuffix(name, moot.ArchivedSuffix)
		// Where to run it, when the line has the room; the command comes first so a
		// narrow terminal never cuts it.
		if where := " in .moot/rooms"; lipgloss.Width(line+where)+2 <= w {
			line += where
		}
		return line
	case r.Keeper != "":
		line += " · kept by " + cleanLine(filepath.Base(r.Keeper))
	}
	return line
}
