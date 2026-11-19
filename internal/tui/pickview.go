package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/picker"
)

// pickWidth is the picker box's outer width in a window w cells wide.
func pickWidth(w int) int { return min(90, max(30, w-6)) }

// pickRows is how many folders the box shows at most: what the window allows, less
// the box's title, rules, notice and hints.
func (m Model) pickRows() int {
	_, h := m.size()
	return max(1, min(15, h-9))
}

// pickWindow is the first row shown, so that the cursor is always inside the box.
func pickWindow(cursor, n, rows int) int {
	return max(0, min(cursor-rows+1, n-rows))
}

// renderPick draws the picker box: a title with the count, one row per folder (name,
// path with ~, room count), a notice when there is one, and the keys.
func (m Model) renderPick(width int) string {
	s := m.styles
	list := m.pick.list
	w := pickWidth(width)
	inner := w - 2
	rows := list.Rows()

	title := " " + s.brand.Render("Choose a folder")
	var count string
	switch {
	case list.Scanning():
		count = s.faint.Render("scanning… ")
	default:
		count = s.faint.Render(strconv.Itoa(len(rows)) + " " + plural(len(rows), "folder", "folders") + " ")
	}
	lines := []string{padTo(title, inner-lipgloss.Width(count)) + count, s.faint.Render(strings.Repeat("─", inner))}

	n := m.pickRows()
	switch {
	case len(rows) == 0:
		lines = append(lines, m.pickEmpty(inner)...)
	default:
		start := pickWindow(list.Cursor(), len(rows), n)
		nameW, roomsW := pickColumns(rows)
		for i := start; i < len(rows) && i < start+n; i++ {
			lines = append(lines, m.pickRow(rows[i], nameW, roomsW, inner))
		}
	}
	if note := list.Notice(); note != "" && len(rows) > 0 {
		lines = append(lines, padTo(s.warn.Render(" ! "+cleanLine(note)), inner))
	}
	keys := s.hintKey.Render("↑↓") + s.hint.Render(" move   ") + s.hintKey.Render("enter") + s.hint.Render(" open   ")
	if m.src != nil {
		keys += s.hintKey.Render("esc") + s.hint.Render(" back")
	} else {
		keys += s.hintKey.Render("ctrl+c") + s.hint.Render(" quit")
	}
	lines = append(lines, s.faint.Render(strings.Repeat("─", inner)), padTo(ansi.Truncate(" "+keys, inner, ""), inner))
	for i, l := range lines {
		lines[i] = clip(l, inner)
	}
	return s.border.Render(strings.Join(lines, "\n"))
}

// pickEmpty is what the list area says when there is no folder to offer.
func (m Model) pickEmpty(inner int) []string {
	s := m.styles
	list := m.pick.list
	var out []string
	switch {
	case list.Scanning():
		out = []string{s.faint.Render("scanning " + cleanLine(m.displayPath(m.home)) + "…")}
	case list.Notice() != "":
		out = append([]string{s.warn.Render("! the scan failed")}, wrapPlain(cleanLine(list.Notice()), inner-2)...)
	default:
		out = []string{
			s.soft.Render("No folder under " + cleanLine(m.displayPath(m.home)) + " holds a .moot directory."),
			s.faint.Render("easymoot <path> opens one that the scan does not reach."),
		}
	}
	for i, l := range out {
		out[i] = padTo(" "+l, inner)
	}
	return out
}

func roomsText(n int) string {
	switch {
	case n < 0:
		return "? rooms"
	case n == 0:
		return "no rooms"
	}
	return strconv.Itoa(n) + " " + plural(n, "room", "rooms")
}

// pickColumns sizes the name and room-count columns to the rows.
func pickColumns(rows []picker.Row) (nameW, roomsW int) {
	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(cleanLine(r.Name)))
		roomsW = max(roomsW, lipgloss.Width(roomsText(r.Rooms)))
	}
	return min(nameW, 20), roomsW
}

func (m Model) pickRow(r picker.Row, nameW, roomsW, inner int) string {
	s := m.styles
	on := func(st lipgloss.Style) lipgloss.Style {
		if r.Selected {
			return st.Background(s.bar.GetBackground())
		}
		return st
	}
	marker, nameStyle := "  ", lipgloss.NewStyle()
	if r.Selected {
		marker, nameStyle = "❯ ", s.selected
	}
	name := fit(cleanLine(r.Name), nameW)
	rooms := roomsText(r.Rooms)
	roomsStyle := s.soft
	if r.Rooms == 0 {
		roomsStyle = s.faint
	}
	// 1 leading space, the marker, the name, 2 gaps, the rooms, 1 trailing space.
	pathW := inner - 1 - lipgloss.Width(marker) - nameW - 2 - roomsW - 2 - 1
	path := ""
	if pathW >= 8 {
		p := cleanLine(r.Path)
		if lipgloss.Width(p) > pathW {
			p = "…" + ansi.TruncateLeft(p, lipgloss.Width(p)-pathW+1, "")
		}
		path = p
	} else {
		pathW = 0
	}
	var b strings.Builder
	b.WriteString(on(lipgloss.NewStyle()).Render(" "))
	b.WriteString(on(s.selected).Render(marker))
	b.WriteString(on(nameStyle).Render(name))
	b.WriteString(on(lipgloss.NewStyle()).Render("  "))
	if pathW > 0 {
		b.WriteString(on(s.soft).Render(padTo(path, pathW)))
		b.WriteString(on(lipgloss.NewStyle()).Render("  "))
	}
	b.WriteString(on(roomsStyle).Render(padTo(strings.Repeat(" ", max(0, roomsW-lipgloss.Width(rooms)))+rooms, roomsW)))
	line := b.String()
	if gap := inner - lipgloss.Width(line); gap > 0 {
		line += on(lipgloss.NewStyle()).Render(strings.Repeat(" ", gap))
	}
	return clip(line, inner)
}
