package tui

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zealtv/easymoot/internal/moot"
)

// The entry bar is where a post is written. It follows EasyLoom's drop prompt
// (internal/tui/tui.go footer and mutate.go updatePrompt at ca2f8f6): one row that
// takes the key hints' place, every key belongs to it while it is open, Enter
// confirms, Esc drops the text. Easyhog has no text entry beyond its palette's query
// line, which this bar's editing keys (backspace, ctrl+w, ctrl+u) match.
//
// Safety rule, the reason this file is small and flat: the only path to the poster is
// a typed Enter, with the bar open and idle, on text that is not blank. A paste is
// text, whatever it holds, and never confirms.

// bar is the open entry bar. room and name are fixed when it opens: the room that was
// selected then, and the name the poll last resolved.
type bar struct {
	room, name string
	text       string
	// busy: Enter has handed the text to the poster and no answer has come. The bar
	// stays up and takes no key but ctrl+c.
	busy bool
	// cancel stops the post in flight; it is set with busy. quitting: ctrl+c has
	// cancelled the post, and the program ends when the post's answer arrives or
	// quitBound passes.
	cancel   context.CancelFunc
	quitting bool
	started  time.Time // when Enter handed the text to the poster
}

// quitBound is how long ctrl+c waits, after cancelling a post, for the script to be
// gone before it quits anyway. The poster itself gives a killed script a second to
// release its pipes, so a normal stop answers well inside this.
const quitBound = 3 * time.Second

// quitMsg ends the wait that ctrl+c began on a busy bar.
type quitMsg struct{ src Source }

// breaks turns the line breaks a terminal sends for a paste (\r\n, or \r alone) into
// \n (review note L7).
var breaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// openBar is what the palette's Post runs. An unsafe name, or no name yet, or no room,
// refuses to open it, with a status line that says why (architecture, "The name in the
// bar").
func (m Model) openBar() (tea.Model, tea.Cmd) {
	switch {
	case m.nameErr != "":
		return m.cannotPost(m.nameErr)
	case m.room == "":
		return m.cannotPost("no room is selected")
	case m.name == "":
		return m.cannotPost("no name has been read yet")
	}
	m.bar = &bar{room: m.room, name: m.name}
	m.notice, m.failure = "", false
	return m, nil
}

func (m Model) cannotPost(why string) (tea.Model, tea.Cmd) {
	m.notice, m.failure = "Can't post: "+cleanLine(why), true
	return m, nil
}

// updateBar takes every key while the bar is open.
func (m Model) updateBar(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := *m.bar
	if b.busy {
		return m, nil // inert until the script answers; ctrl+c was taken before this
	}
	m.notice, m.failure = "", false
	switch {
	case k.Paste:
		// Whole, never confirming, whatever it holds.
		if k.Type == tea.KeyRunes {
			b.text += breaks.Replace(string(k.Runes))
		}
	case k.Alt:
		// An escape-prefixed key is how a terminal sends Esc fused with the next key
		// (tmux within escape-time, ssh, Option as Meta). Treat it as the Esc it holds:
		// close the bar and drop the text, so a draft cannot stay behind for a later
		// Enter to post (final review, M1).
		m.bar = nil
		return m, nil
	case k.Type == tea.KeyEnter:
		if strings.TrimSpace(b.text) == "" {
			return m, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		b.busy, b.cancel, b.started = true, cancel, time.Now()
		m.bar = &b
		m.notice = "posting to " + cleanLine(b.room) + "…"
		return m, postCmd(ctx, cancel, m.src, b.room, b.text)
	case k.Type == tea.KeyEsc:
		m.bar = nil
		return m, nil
	case k.Type == tea.KeyBackspace || k.Type == tea.KeyCtrlH:
		if r := []rune(b.text); len(r) > 0 {
			b.text = string(r[:len(r)-1])
		}
	case k.Type == tea.KeyCtrlW:
		b.text = dropWord(b.text)
	case k.Type == tea.KeyCtrlU:
		b.text = ""
	case k.Type == tea.KeySpace:
		b.text += " "
	case k.Type == tea.KeyRunes:
		for _, r := range k.Runes {
			if !unicode.IsControl(r) {
				b.text += string(r)
			}
		}
	}
	m.bar = &b
	return m, nil
}

// dropWord removes the whitespace at the end of s and then the word before it.
func dropWord(s string) string {
	r := []rune(s)
	n := len(r)
	for n > 0 && unicode.IsSpace(r[n-1]) {
		n--
	}
	for n > 0 && !unicode.IsSpace(r[n-1]) {
		n--
	}
	return string(r[:n])
}

// postMsg is the poster's answer.
type postMsg struct {
	src  Source
	room string
	res  moot.PostResult
	err  error
}

// stopPost is ctrl+c on a busy bar: cancel the post, so moot kills the script, and
// quit when the answer arrives (applyPost) or quitBound passes (quitMsg). With nothing
// in flight, or a second ctrl+c, it quits at once.
func (m Model) stopPost() (tea.Model, tea.Cmd) {
	b := m.bar
	if b == nil || !b.busy || b.cancel == nil || b.quitting {
		return m, tea.Quit
	}
	b.cancel()
	nb := *b
	nb.quitting = true
	m.bar = &nb
	m.exit = &exitNote{kind: exitUnknown, room: b.room, name: b.name, since: b.started}
	m.notice, m.failure = "stopping the post…", false
	src := m.src
	return m, tea.Tick(quitBound, func(time.Time) tea.Msg { return quitMsg{src: src} })
}

// postCmd runs the one write Easymoot makes, outside Update. moot.Post bounds it, and
// ctx lets ctrl+c stop it.
func postCmd(ctx context.Context, cancel context.CancelFunc, src Source, room, text string) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		res, err := src.Post(ctx, room, text)
		return postMsg{src: src, room: room, res: res, err: err}
	}
}

// applyPost takes the poster's answer (architecture, "Posting"). Success closes the
// bar, says what went out, returns the posts pane to the end and reads the room at once. A time-out closes it too, since
// the post file lands before any git step: it says the script didn't finish and the
// next poll shows the post if it landed. Any other failure leaves the text in the bar.
func (m Model) applyPost(msg postMsg) (tea.Model, tea.Cmd) {
	if msg.src != m.src {
		return m, nil
	}
	if m.bar != nil && m.bar.quitting {
		// ctrl+c stopped the post; the script has exited. Keep what it said for the
		// line Run returns.
		if m.exit != nil {
			e := m.exit.answered(msg)
			m.exit = &e
		}
		return m, tea.Quit
	}
	room := cleanLine(msg.room)
	switch {
	case msg.err == nil:
		m.bar = nil
		room, sender := msg.res.Room, msg.res.Sender
		if room == "" {
			room = msg.room
		}
		text := "posted to " + cleanLine(room)
		if sender != "" {
			text += " as " + cleanLine(sender)
		}
		for _, n := range msg.res.Notices {
			text += " · " + cleanLine(n)
		}
		m.notice, m.failure = text, false
		m = m.followAfterPost(room)
		return m, m.readAfterPost(msg.room)
	case errors.Is(msg.err, moot.ErrTimeout):
		m.bar = nil
		m.notice, m.failure = "moot.sh did not finish posting to "+room+"; the post may have landed, and the next poll will show it", true
		return m, m.readAfterPost(msg.room)
	}
	if m.bar != nil {
		b := *m.bar
		b.busy = false
		m.bar = &b
	}
	m.notice, m.failure = "Could not post to "+room+": "+cleanLine(msg.err.Error()), true
	return m, nil
}

// followAfterPost returns the posts pane to following the end, as the end key does,
// when the post went to the room on show, so the new post is in view once the read
// lands. A post to any other room leaves the view alone.
func (m Model) followAfterPost(room string) Model {
	if room == "" || room != m.room {
		return m
	}
	return m.scrollTo(len(m.lines))
}

// readAfterPost lists the room again at once, outside the poll chain, when it is the
// room on show.
func (m Model) readAfterPost(room string) tea.Cmd {
	if room == "" || room != m.room {
		return nil
	}
	return pollCmd(m.src, m.room, m.bodies, nil, false)
}

// barText is the bar's text as drawn: the cleaner has run over it and each line
// break shows as ↵.
func barText(text string) string {
	return strings.ReplaceAll(clean(breaks.Replace(text)), "\n", "↵")
}

// barLine draws the bar over the key hints' row, showing the end of a long text.
func (m Model) barLine(w int) string {
	s := m.styles
	b := m.bar
	prompt := " " + s.warn.Render("post to "+cleanLine(b.room)+" as "+cleanLine(b.name)+" ›") + " "
	hint := s.hintKey.Render("enter") + s.hint.Render(" post · ") + s.hintKey.Render("esc") + s.hint.Render(" cancel")
	cursor := s.thumb.Render("▌")
	if b.busy {
		hint = s.hint.Render("posting…")
		if b.quitting {
			hint = s.hint.Render("stopping…")
		}
		cursor = ""
	}
	text := barText(b.text)
	room := w - lipgloss.Width(prompt) - lipgloss.Width(cursor)
	if hw := lipgloss.Width(hint) + 3; room-hw >= 12 {
		room -= hw
	} else {
		hint = ""
	}
	if room < 1 {
		return fit(prompt, w)
	}
	if tw := lipgloss.Width(text); tw > room {
		text = "…" + ansi.TruncateLeft(text, tw-room+1, "")
	}
	if b.busy {
		text = s.faint.Render(text)
	}
	left := prompt + text + cursor
	if hint == "" {
		return fit(left, w)
	}
	gap := max(1, w-lipgloss.Width(left)-lipgloss.Width(hint)-1)
	return fit(left+strings.Repeat(" ", gap)+hint, w)
}
