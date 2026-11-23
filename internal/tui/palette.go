package tui

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The command palette follows Easyhog's (internal/tui/palette.go at 01336f8) and
// EasyLoom's: every action, found by typing, run by Enter. Quit and everything that
// changes anything live here and nowhere else. A strict command matches only a
// prefix of its name, with no aliases and no scattered letters, so a sentence typed
// into the wrong tmux pane reaches it only by ending, after a colon, in its exact
// name or a prefix of it ("q: quit"). The palette selects nothing until the reader
// has typed or moved, which makes ":" then Enter do nothing at all.
//
// A command is one row in allCommands and one case in run, and nothing else here
// changes.

// paletteKey opens the palette.
const paletteKey = ":"

// paletteRows is how many commands the palette shows at most.
const paletteRows = 10

type action string

const (
	actionPost    action = "post"
	actionDM      action = "dm" // a post to a participant, through moot.sh dm
	actionQuit    action = "quit"
	actionRefresh action = "refresh"
	// The room actions, each a moot.sh verb (actions.go):
	actionCreate  action = "create"
	actionArchive action = "archive"
	actionJoin    action = "join"
	actionLeave   action = "leave"
	// The two that only look (they change what is on show, never the moot):
	actionArchived action = "archived" // Show archived / Hide archived
	actionPinned   action = "pinned"
)

type palette struct {
	query string
	// chosen is the action the reader has typed toward or moved to, or "" before
	// they have typed or moved: nothing is selected, so Enter runs nothing. It
	// names an action, not a list position, so a list that reshuffles or shrinks
	// never moves it onto a different action.
	chosen action
}

type command struct {
	action action
	name   string
	words  string // other names it can be found by
	glyph  string
	key    string // bare key shown beside it, if any
	// strict commands change what happens, so they match only a prefix of their
	// name: no aliases and no scattered letters.
	strict bool
	// when says whether the command is listed now; nil lists it always. A command is
	// listed only where it can run.
	when func(Model) bool
}

// allCommands is every command, in the order the palette lists them, each with the
// test that says when it is on offer: the room actions by the selected room's kind
// (actions.go), none that acts while a room action runs, Show/Hide archived when an
// archived room is known, Pinned post when the open room has one.
var allCommands = []command{
	{actionPost, "Post", "", "✎", "", true, idle},
	{actionDM, "DM", "", "✉", "", true, func(m Model) bool { p, ok := m.selectedPerson(); return idle(m) && m.onPerson && ok && !p.you }},
	{actionCreate, "Create room", "", "+", "", true, idle},
	{actionArchive, "Archive room", "", "▣", "", true, func(m Model) bool { return idle(m) && m.canArchive() }},
	{actionJoin, "Join room", "", "⇢", "", true, func(m Model) bool { return idle(m) && m.canJoin() }},
	{actionLeave, "Leave room", "", "⇠", "", true, func(m Model) bool { return idle(m) && m.canLeave() }},
	{actionPinned, "Pinned post", "pin", "◆", "", false, func(m Model) bool { return m.pinned() >= 0 }},
	{actionArchived, "Show archived", "archived", "◌", "", false, func(m Model) bool { return m.archivedCount() > 0 && !m.showArchived }},
	{actionArchived, "Hide archived", "archived", "◌", "", false, func(m Model) bool { return m.archivedCount() > 0 && m.showArchived }},
	{actionRefresh, "Refresh", "", "↻", "", true, idle},
	{actionQuit, "Quit", "", "×", "ctrl+c", true, nil},
}

// idle says no room action is running: one at a time.
func idle(m Model) bool { return !m.acting }

// onPersonCommands are the commands offered with a participant selected. The rest act
// on the selected room or its posts, and no room is selected then.
var onPersonCommands = map[action]bool{actionDM: true, actionRefresh: true, actionQuit: true}

// commands lists the commands on offer now.
func (m Model) commands() []command {
	var out []command
	for _, c := range allCommands {
		if m.onPerson && !onPersonCommands[c.action] {
			continue
		}
		if c.when == nil || c.when(m) {
			if c.action == actionDM {
				p, _ := m.selectedPerson()
				c.name += " " + cleanLine(p.Name)
			}
			out = append(out, c)
		}
	}
	return out
}

type match struct {
	command
	score int
	hits  []int // rune indexes of the name the query matched
}

// filtered is the commands the query matches, best first.
func (m Model) filtered() []match {
	query := ""
	if m.palette != nil {
		query = m.palette.query
	}
	var out []match
	for _, c := range m.commands() {
		if c.strict {
			if score, hits, ok := prefix(query, c.name); ok {
				out = append(out, match{c, score, hits})
			}
			continue
		}
		score, hits, ok := fuzzy(query, c.name)
		if !ok {
			if score, _, ok = fuzzy(query, c.words); !ok {
				continue
			}
			score -= 20
			hits = nil
		}
		out = append(out, match{c, score, hits})
	}
	if query != "" {
		sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	}
	return out
}

// prefix matches query as the start of text, ignoring case and the spaces of
// both, so "qu i" finds "Quit". hits are the rune indexes of text the
// query matched. An empty query matches.
func prefix(query, text string) (score int, hits []int, ok bool) {
	q := []rune(strings.ToLower(strings.ReplaceAll(query, " ", "")))
	t := []rune(strings.ToLower(text))
	ti := 0
	for _, r := range q {
		for ti < len(t) && t[ti] == ' ' {
			ti++
		}
		if ti >= len(t) || t[ti] != r {
			return 0, nil, false
		}
		hits = append(hits, ti)
		ti++
	}
	return 100 + len(q), hits, true
}

// fuzzy matches query against text as a subsequence, ignoring case and the
// query's spaces, scoring word starts and runs above scattered letters.
func fuzzy(query, text string) (score int, hits []int, ok bool) {
	q := []rune(strings.ToLower(strings.ReplaceAll(query, " ", "")))
	if len(q) == 0 {
		return 0, nil, true
	}
	t := []rune(strings.ToLower(text))
	qi, last := 0, -2
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			continue
		}
		score++
		if ti == 0 || !unicode.IsLetter(t[ti-1]) {
			score += 8
		}
		if ti == last+1 {
			score += 5
		}
		hits = append(hits, ti)
		last = ti
		qi++
	}
	if qi < len(q) {
		return 0, nil, false
	}
	score -= hits[0]
	if len(q) == len(t) {
		score += 20
	}
	return score, hits, true
}

func (m Model) openPalette() Model {
	m.palette = &palette{}
	m.notice, m.failure = "", false
	return m
}

// updatePalette takes every key while the palette is open.
func (m Model) updatePalette(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := *m.palette
	matches := m.filtered()
	at := -1 // where the chosen action sits in the list now, -1 if absent
	for i, c := range matches {
		if c.action == p.chosen && p.chosen != "" {
			at = i
		}
	}
	pick := func(i int) {
		if i >= 0 && i < len(matches) {
			p.chosen = matches[i].action
		}
	}
	switch k.String() {
	case "esc", "ctrl+g":
		m.palette = nil
		return m, nil
	case "enter":
		if p.chosen == "" {
			return m, nil
		}
		return m.run(p)
	case "up", "ctrl+p", "shift+tab":
		pick(step(at, -1, len(matches)))
	case "down", "ctrl+n", "tab":
		pick(step(at, 1, len(matches)))
	case "pgup":
		pick(max(0, at-paletteRows))
	case "pgdown":
		pick(min(len(matches)-1, max(0, at)+paletteRows))
	case "backspace", "ctrl+h":
		if r := []rune(p.query); len(r) > 0 {
			p.query = string(r[:len(r)-1])
		}
		p.chosen = ""
	case "ctrl+u", "ctrl+w":
		p.query, p.chosen = "", ""
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			p.query += cleanLine(string(k.Runes))
			if k.Type == tea.KeySpace {
				p.query += " "
			}
			p.chosen = ""
		}
	}
	m.palette = &p
	if (k.String() == "backspace" || k.String() == "ctrl+h" || k.Type == tea.KeyRunes || k.Type == tea.KeySpace) &&
		strings.TrimSpace(p.query) != "" {
		// Typing selects the best match, by action.
		if top := m.filtered(); len(top) > 0 {
			m.palette.chosen = top[0].action
		}
	}
	return m, nil
}

// cursor is the list position of the chosen action, -1 when there is none or the
// list no longer offers it.
func (m Model) cursor() int {
	if m.palette == nil || m.palette.chosen == "" {
		return -1
	}
	for i, c := range m.filtered() {
		if c.action == m.palette.chosen {
			return i
		}
	}
	return -1
}

func step(cursor, delta, n int) int {
	if n == 0 {
		return -1
	}
	if cursor < 0 {
		return 0
	}
	return (cursor + delta + n) % n
}

// refuse closes the palette and says, in the status line, why nothing ran.
func (m Model) refuse(why string) (tea.Model, tea.Cmd) {
	m.palette = nil
	m.notice, m.failure = "Nothing ran: "+why, true
	return m, nil
}

// run performs the chosen command, but only if the palette still offers exactly
// what the reader chose. Otherwise nothing runs and a notice says why.
func (m Model) run(p palette) (tea.Model, tea.Cmd) {
	offered := false
	for _, c := range m.filtered() {
		offered = offered || c.action == p.chosen
	}
	if !offered {
		return m.refuse(commandName(p.chosen) + " is no longer offered or no longer matches")
	}
	m.palette = nil
	switch p.chosen {
	case actionPost:
		return m.openBar()
	case actionDM:
		return m.openDM()
	case actionRefresh:
		return m, m.pollAs(m.room, m.bodies, m.latest, false, true, pollOpts{repair: true})
	case actionCreate:
		return m.openCreate(), nil
	case actionArchive:
		return m.openConfirm("archive"), nil
	case actionLeave:
		return m.openConfirm("leave"), nil
	case actionJoin:
		return m.joinRoom()
	case actionArchived:
		return m.toggleArchived()
	case actionPinned:
		return m.showPinned(), nil
	case actionQuit:
		return m, tea.Quit
	}
	return m, nil
}

func commandName(a action) string {
	for _, c := range allCommands {
		if c.action == a {
			return c.name
		}
	}
	return string(a)
}

// overlay draws the palette box over the frame, fading the frame behind it. On the
// rows the box covers, the faded frame is cleared to either side: a cut through the
// middle of the text there would leave a stray fragment beside the box.
func (m Model) overlay(frame []string, width int) []string {
	box := strings.Split(m.renderPalette(width), "\n")
	bw := lipgloss.Width(box[0])
	x := max(0, (width-bw)/2)
	y := max(0, (len(frame)-len(box))/3)
	out := make([]string, len(frame))
	for i, line := range frame {
		if i < y || i >= y+len(box) {
			out[i] = m.styles.faint.Render(ansi.Strip(line))
			continue
		}
		out[i] = strings.Repeat(" ", x) + box[i-y] + strings.Repeat(" ", max(0, width-x-bw))
	}
	return out
}

func (m Model) renderPalette(width int) string {
	s := m.styles
	_, h := m.size()
	w := min(64, max(26, width-6))
	inner := w - 2
	p := m.palette
	matches := m.filtered()

	count := s.faint.Render(fmtCount(len(matches), len(m.commands())))
	query := s.selected.Render(p.query) + s.thumb.Render("▌")
	if p.query == "" {
		query = s.thumb.Render("▌") + s.faint.Render("type a command")
	}
	prompt := padTo(" "+s.brand.Render(paletteKey)+" "+query, inner-lipgloss.Width(count))
	prompt = ansi.Truncate(prompt, inner-lipgloss.Width(count), "") + count

	rows := max(1, min(paletteRows, min(len(m.commands()), h-6)))
	start := 0
	if c := m.cursor(); c >= rows {
		start = c - rows + 1
	}
	lines := []string{prompt, s.faint.Render(strings.Repeat("─", inner))}
	for i := 0; i < rows; i++ {
		at := start + i
		switch {
		case at < len(matches):
			lines = append(lines, m.paletteRow(matches[at], at == m.cursor(), inner))
		case i == 0:
			lines = append(lines, padTo(s.faint.Render("   nothing matches"), inner))
		default:
			lines = append(lines, strings.Repeat(" ", inner))
		}
	}
	keys := s.hintKey.Render("↑↓") + s.hint.Render(" select   ") +
		s.hintKey.Render("enter") + s.hint.Render(" run   ") +
		s.hintKey.Render("esc") + s.hint.Render(" close")
	lines = append(lines, s.faint.Render(strings.Repeat("─", inner)), padTo(ansi.Truncate(" "+keys, inner, ""), inner))
	return s.border.Render(strings.Join(lines, "\n"))
}

func fmtCount(n, of int) string { return strconv.Itoa(n) + "/" + strconv.Itoa(of) + " " }

func (m Model) paletteRow(c match, selected bool, width int) string {
	s := m.styles
	on := func(st lipgloss.Style) lipgloss.Style { return onBar(st, selected, s.bar.GetBackground()) }
	plain := lipgloss.NewStyle()
	lit := map[int]bool{}
	for _, h := range c.hits {
		lit[h] = true
	}
	var b strings.Builder
	for i, r := range []rune(c.name) {
		st := plain
		if selected {
			st = st.Bold(true)
		}
		if lit[i] {
			st = s.match.Inherit(st)
		}
		b.WriteString(on(st).Render(string(r)))
	}
	marker := on(plain).Render("   ")
	if selected {
		marker = on(s.selected).Render(" ❯ ")
	}
	left := marker + on(s.soft).Render(c.glyph) + on(plain).Render(" ") + b.String()
	right := ""
	if c.key != "" {
		right = on(s.hintKey).Render(c.key) + on(plain).Render(" ")
	}
	if room := width - lipgloss.Width(right); lipgloss.Width(left) > room {
		left = ansi.Truncate(left, max(0, room-1), "…")
	}
	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + on(plain).Render(strings.Repeat(" ", gap)) + right
}

// padTo pads a rendered line with spaces to width cells.
func padTo(line string, width int) string {
	if gap := width - lipgloss.Width(line); gap > 0 {
		return line + strings.Repeat(" ", gap)
	}
	return line
}
