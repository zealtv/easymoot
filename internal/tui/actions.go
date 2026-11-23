package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// Room actions: Create, Archive, Join, Leave. Each runs one moot.sh verb through
// Source.Run, one at a time. Easymoot decides only where a command is listed
// (docs/design.md, "Room actions"); the script decides whether it runs, and its
// refusal is what the status line shows. Archive and Leave ask once, on the bar's row;
// Create takes a name there; Join asks nothing.

// actionMsg is the script's answer.
type actionMsg struct {
	src                 Source
	verb, label, target string
	res                 moot.RunResult
	err                 error
}

var gerunds = map[string]string{"create": "creating", "archive": "archiving", "join": "joining", "leave": "leaving"}

// localName is the name of the selected room in the opened folder's rooms/, which is
// what the verbs take: the posting name of a heard room, else the room's own name.
func localName(r moot.Room) string {
	if r.PostName != "" {
		return r.PostName
	}
	return r.Name
}

// The three verbs that need a room, offered by kind (docs/design.md, "Room actions"):
// Archive on a live room this folder keeps that is not a DM, Join on a live one it
// does not hear, Leave on any link in its rooms/.
func (m Model) canArchive() bool {
	r, ok := m.roomKind()
	return ok && r.Kind.Standing == moot.Kept && !r.Kind.Archived && !r.Kind.DM
}

func (m Model) canJoin() bool {
	r, ok := m.roomKind()
	return ok && r.Kind.Standing == moot.Elsewhere && !r.Kind.Archived && !r.Kind.DM
}

func (m Model) canLeave() bool {
	r, ok := m.roomKind()
	return ok && r.Kind.Standing == moot.Heard
}

// openCreate puts the name bar up, for the folder's rooms/.
func (m Model) openCreate() Model {
	folder := m.ownName
	if folder == "" {
		folder = filepath.Base(m.src.Folder())
	}
	m.bar = &bar{mode: barCreate, verb: "create", label: folder, note: "names use A-Z a-z 0-9 . _ -"}
	m.notice, m.failure = "", false
	return m
}

// openConfirm puts the question for Archive or Leave on the selected room up. What
// it will run is fixed now, so Enter confirms what the row shows.
func (m Model) openConfirm(verb string) Model {
	r, _ := m.roomKind()
	b := &bar{mode: barConfirm, verb: verb, target: localName(r), label: m.roomName()}
	if verb == "archive" {
		b.why, b.note = "posting closes for everyone", "archiving renames the room's folder; mv puts it back"
	} else {
		b.why, b.note = "removes this folder's link", "the keeper's room stays; join links it again"
	}
	m.bar = b
	m.notice, m.failure = "", false
	return m
}

// joinRoom runs join on the selected room's folder, at its keeper.
func (m Model) joinRoom() (tea.Model, tea.Cmd) {
	r, _ := m.roomKind()
	return m.startAction("join", m.roomName(), filepath.Join(r.Keeper, ".moot", "rooms", r.Name))
}

// startAction runs verb on target, the one write besides Post, and says so.
func (m Model) startAction(verb, label, target string) (tea.Model, tea.Cmd) {
	m.bar, m.palette = nil, nil
	m.acting = true
	m.notice, m.failure = gerunds[verb]+" "+cleanLine(label)+"…", false
	src := m.src
	return m, func() tea.Msg {
		res, err := src.Run(context.Background(), []string{verb, target})
		return actionMsg{src: src, verb: verb, label: label, target: target, res: res, err: err}
	}
}

// applyAction takes the script's answer: its line (and any notices) on success, its
// message on a refusal. Either way the rooms are listed again, after link repair, and
// a created room, or an archived one while archived rooms show, becomes the selected one.
func (m Model) applyAction(msg actionMsg) (tea.Model, tea.Cmd) {
	if msg.src != m.src {
		return m, nil
	}
	m.acting = false
	label := cleanLine(msg.label)
	var pick func([]moot.Room) string
	switch {
	case msg.err == nil:
		text := strings.Join(append(append([]string{}, msg.res.Lines...), msg.res.Notices...), " · ")
		m.notice, m.failure = cleanLine(text), false
		switch {
		case msg.verb == "create":
			pick = pickOwn(msg.label)
		case msg.verb == "archive" && m.showArchived:
			pick = pickOwn(msg.target + moot.ArchivedSuffix)
		}
	case errors.Is(msg.err, moot.ErrTimeout):
		m.notice, m.failure = "moot.sh did not finish "+gerunds[msg.verb]+" "+label+"; Refresh shows where it stands", true
	default:
		m.notice, m.failure = "Could not "+msg.verb+" "+label+": "+cleanLine(msg.err.Error()), true
	}
	return m, m.pollAs(m.room, m.bodies, nil, false, true, pollOpts{repair: true, pick: pick})
}

// pickOwn finds the room in this folder's rooms/ under name, kept or heard.
func pickOwn(name string) func([]moot.Room) string {
	return func(rooms []moot.Room) string {
		for _, r := range rooms {
			if r.Kind.Standing != moot.Elsewhere && localName(r) == name {
				return r.Key()
			}
		}
		return ""
	}
}

// noteRepairs puts what moot.sh rooms said while mending links on the status line,
// after whatever is there. A folder with no moot.sh has nothing to mend, and says nothing.
func (m Model) noteRepairs(msg pollMsg) Model {
	if msg.repairErr != nil && !errors.Is(msg.repairErr, moot.ErrNoScript) {
		m.notice, m.failure = "moot.sh rooms: "+cleanLine(msg.repairErr.Error()), true
		return m
	}
	if len(msg.repairs) == 0 {
		return m
	}
	text := cleanLine(strings.Join(msg.repairs, " · "))
	if m.notice != "" {
		text = m.notice + " · " + text
	}
	m.notice = text
	return m
}
