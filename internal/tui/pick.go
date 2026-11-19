package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/picker"
)

// The folder picker (architecture, "Choosing a folder"). The list and the scan are
// internal/picker's; this file is how the shell drives them. The scan is I/O, so it
// runs in a command and comes back as a message, like every read here; the model
// holds only the answer.
//
// It opens two ways. At start-up with no moot (NewPicker) there is no Source yet:
// nothing is drawn behind it, and Esc does nothing, since nothing outside the
// palette quits but ctrl+c (Easyhog's and EasyLoom's rule) and there is no shell to
// go back to. From the palette's Switch folder it opens over the running shell,
// which goes on polling underneath, and Esc takes it back unchanged.

// pickState is the picker while it is open. It is replaced, never edited in place,
// since a Model is copied on every Update.
type pickState struct {
	list picker.Model
	// gen says which opening this is. A scan or an open that answers for an earlier
	// one is dropped, so a late answer never lands on a picker opened afterwards.
	gen int
}

// scanMsg is the answer to scanCmd.
type scanMsg struct {
	gen     int
	entries []picker.Entry
	err     error
}

// openedMsg is the answer to openCmd.
type openedMsg struct {
	gen    int
	folder string
	src    Source
	err    error
}

// NewPicker is a shell with no moot open and the picker showing: what `easymoot`
// starts as in a folder with no .moot/ when no path was typed. Its first command is
// the scan.
func NewPicker(opts ...Option) Model {
	m, _ := New(nil, opts...).openPicker()
	return m
}

// openPicker shows the picker and returns the command that scans for it.
func (m Model) openPicker() (Model, tea.Cmd) {
	m.palette = nil
	m.scanGen++
	m.pick = &pickState{list: picker.New(m.home).Start(), gen: m.scanGen}
	return m, scanCmd(m.home, m.scanGen)
}

// scanCmd scans the home directory. It is the only reading the picker does.
func scanCmd(home string, gen int) tea.Cmd {
	return func() tea.Msg {
		if home == "" {
			return scanMsg{gen: gen, err: errors.New("no home directory to scan")}
		}
		entries, err := picker.Scan(context.Background(), home)
		return scanMsg{gen: gen, entries: entries, err: err}
	}
}

// openCmd opens the moot of a folder the scan found. It can fail: the folder may have
// lost its .moot/ since.
func openCmd(gen int, folder string) tea.Cmd {
	return func() tea.Msg {
		src, err := moot.Open(folder)
		if err != nil {
			return openedMsg{gen: gen, folder: folder, err: err}
		}
		return openedMsg{gen: gen, folder: folder, src: src}
	}
}

func (m Model) applyScan(msg scanMsg) (tea.Model, tea.Cmd) {
	if m.pick == nil || msg.gen != m.pick.gen {
		return m, nil
	}
	p := *m.pick
	p.list = p.list.Done(msg.entries, msg.err)
	m.pick = &p
	return m, nil
}

func (m Model) applyOpened(msg openedMsg) (tea.Model, tea.Cmd) {
	if m.pick == nil || msg.gen != m.pick.gen {
		return m, nil
	}
	if msg.err != nil {
		p := *m.pick
		p.list = p.list.Notify("could not open " + cleanLine(m.displayPath(msg.folder)) + ": " + cleanLine(msg.err.Error()))
		m.pick = &p
		return m, nil
	}
	return m.switchTo(msg.src)
}

// switchTo shows another moot. Everything read from the old one goes, and the old
// poll chain ends of itself: its ticks and answers name the old Source and are
// dropped. Fields this does not name (the window, the styles, the options) stay.
func (m Model) switchTo(src Source) (tea.Model, tea.Cmd) {
	m.src, m.pick = src, nil
	m.rooms, m.room, m.sel, m.sideTop = nil, "", -1, 0
	m = m.clearPosts()
	m.loaded, m.name, m.nameErr, m.loadErr = false, "", "", ""
	m.lines, m.starts, m.top = nil, nil, 0
	m.focus, m.palette, m.notice, m.failure = paneRooms, nil, "", false
	return m.rebuild(), pollCmd(src, "", nil, true)
}

// updatePick takes every key while the picker is open. Looking keys move the cursor,
// Enter opens the folder under it, Esc goes back to the shell if there is one, and
// ctrl+c (handled before this) quits. Nothing else does anything, and a paste is
// ignored.
func (m Model) updatePick(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Paste {
		return m, nil
	}
	list := m.pick.list
	switch k.String() {
	case "esc":
		if m.src != nil {
			m.pick = nil
		}
		return m, nil
	case "enter":
		if sel := list.Selected(); sel != nil {
			return m, openCmd(m.pick.gen, sel.Folder)
		}
		return m, nil
	case "up":
		list = list.Move(-1)
	case "down":
		list = list.Move(1)
	case "pgup":
		list = list.Move(-m.pickRows())
	case "pgdown":
		list = list.Move(m.pickRows())
	case "home":
		list = list.MoveTo(0)
	case "end":
		list = list.MoveTo(len(list.Entries()) - 1)
	default:
		return m, nil
	}
	p := *m.pick
	p.list = list
	m.pick = &p
	return m, nil
}
