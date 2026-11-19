package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// transcript lays the selected room's posts out as one run of lines, oldest first,
// as `moot.sh read` prints them: a heading line, then the post's text, then a blank
// line between posts. starts[i] is the line where posts[i] begins. It is pure: the bodies were
// read by the poll.
func (m Model) transcript(width int) (lines []string, starts []int) {
	s := m.styles
	for i, p := range m.posts {
		starts = append(starts, len(lines))
		lines = append(lines, m.heading(p, width))
		b, ok := m.bodies[p.ID]
		switch {
		case !ok:
			lines = append(lines, s.faint.Render("…"))
		case b.err != "":
			lines = append(lines, s.warn.Render("(could not read: "+cleanLine(b.err)+")"))
		case b.Missing:
			lines = append(lines, s.faint.Render("(no post.md in posts/"+cleanLine(p.ID)+")"))
		default:
			lines = append(lines, renderMarkdown(clean(b.Text), width, s, m.colour)...)
		}
		if b.Truncated {
			lines = append(lines, s.faint.Render("… (cut: the post is longer than Easymoot reads)"))
		}
		if len(b.Attached) > 0 {
			names := make([]string, len(b.Attached))
			for j, n := range b.Attached {
				names[j] = cleanLine(n)
			}
			lines = append(lines, "", s.faint.Render("attached: "+strings.Join(names, " ")))
		}
		if i < len(m.posts)-1 {
			lines = append(lines, "")
		}
	}
	return lines, starts
}

// heading is a post's one heading line, `── 2026-08-29 21:00 · alice ──`. The time is
// in the reader's zone, to the minute (moot.sh read shows seconds and the zone
// too); a stamp that is no real date shows as written and marked UTC. An id that is
// no stamp (a stray README.md) shows under its own name with no time.
func (m Model) heading(p moot.Post, width int) string {
	s := m.styles
	rule := s.faint.Render("── ")
	end := s.faint.Render(" ──")
	var body string
	switch {
	case !p.Stamp:
		body = s.soft.Render(cleanLine(p.ID))
	default:
		when := p.WrittenTime() + " UTC"
		if !p.Time.IsZero() {
			when = p.Time.In(m.loc).Format("2006-01-02 15:04")
		}
		body = s.soft.Render(when) + s.faint.Render(" · ") + s.sender.Render(cleanLine(p.Sender))
	}
	line := rule + body + end
	if lipgloss.Width(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	return line
}
