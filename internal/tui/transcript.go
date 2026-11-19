package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// measure is the widest a post's prose is wrapped, in columns: a comfortable line
// for a paragraph (Bob asked for "a nice readable paragraph", 2026-11-19; 76 was
// picked by eye between 72 and 80). In a narrower pane the prose wraps to the pane.
// Fenced code and table rows keep the pane's width, as before. The post's heading
// line spans the pane.
const measure = 76

// postGap is how many blank lines sit between one post and the next heading, so
// each post reads as its own block.
const postGap = 2

// transcript lays the selected room's posts out as one run of lines, oldest first,
// as `moot.sh read` prints them: a heading line, then the post's text (prose wrapped
// to the measure), then postGap blank lines between posts. starts[i] is the line where
// posts[i] begins. It is pure: the bodies were read by the poll.
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
			lines = append(lines, renderMarkdown(clean(b.Text), width, measure, s, m.colour)...)
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
			for range postGap {
				lines = append(lines, "")
			}
		}
	}
	return lines, starts
}

// heading is a post's one heading line: the sender at the left and the time at the
// right, joined by the rule and right-aligned to the measure (or the pane, when that
// is narrower), so the time sits over the right edge of the prose:
// `── alice ────────── 2026-08-29 21:00 ──`. The time is in the reader's zone, to the
// minute (moot.sh read shows seconds and the zone too); a stamp that is no real date
// shows as written and marked UTC. An id that is no stamp (a stray README.md) shows
// under its own name with no time. A heading wider than the pane is cut.
func (m Model) heading(p moot.Post, width int) string {
	s := m.styles
	rule := s.faint.Render("── ")
	end := s.faint.Render(" ──")
	var line string
	if !p.Stamp {
		line = rule + s.soft.Render(cleanLine(p.ID)) + end
	} else {
		when := p.WrittenTime() + " UTC"
		if !p.Time.IsZero() {
			when = p.Time.In(m.loc).Format("2006-01-02 15:04")
		}
		left := rule + s.sender.Render(cleanLine(p.Sender)) + " "
		right := " " + s.soft.Render(when) + end
		fill := max(1, min(width, measure)-lipgloss.Width(left)-lipgloss.Width(right))
		line = left + s.faint.Render(strings.Repeat("─", fill)) + right
	}
	if lipgloss.Width(line) > width {
		line = ansi.Truncate(line, width, "…")
	}
	return line
}
