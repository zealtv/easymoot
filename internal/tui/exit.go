package tui

import (
	"errors"
	"time"

	"github.com/zealtv/easymoot/internal/moot"
)

// What Easymoot says on leaving. After a ctrl+c that stopped a post in flight it
// prints one line, once the terminal is restored, telling whether the post went out;
// any other way of quitting prints nothing. Run returns the line, main prints it.

type exitKind int

const (
	// exitUnknown: ctrl+c began a stop, and no answer came (the quit bound passed, or
	// a second ctrl+c).
	exitUnknown exitKind = iota
	// exitLanded: the script answered with success.
	exitLanded
	// exitStopped: the script was stopped or failed. ExitLine lists the room once to
	// see whether a post from this name appeared meanwhile.
	exitStopped
)

// exitNote is what ctrl+c while posting leaves for ExitLine.
type exitNote struct {
	kind       exitKind
	room, name string
	id         string    // the landed post's id, when the script said it
	since      time.Time // when Enter handed the text to the poster
}

// answered is the note once the poster's answer has come in.
func (e exitNote) answered(msg postMsg) exitNote {
	if msg.room != "" {
		e.room = msg.room // a DM's, once moot.sh dm named it
	}
	switch {
	case msg.err == nil:
		e.kind, e.id = exitLanded, msg.res.ID
	case errors.Is(msg.err, moot.ErrTimeout):
		e.kind = exitUnknown
	default:
		e.kind = exitStopped
	}
	return e
}

// ExitLine is the one line to print after the program ends, or "" when ctrl+c did not
// stop a post. For a stopped post it reads the room's listing once (never writes).
func (m Model) ExitLine() string {
	e := m.exit
	if e == nil {
		return ""
	}
	room := cleanLine(e.room)
	switch e.kind {
	case exitLanded:
		return "easymoot: the post had already gone out to " + room + idSuffix(e.id)
	case exitStopped:
		posts, err := m.src.List(e.room)
		if err != nil {
			break
		}
		from := e.since.UTC().Truncate(time.Second)
		for i := len(posts) - 1; i >= 0; i-- {
			p := posts[i]
			if p.Stamp && p.Sender == e.name && !p.Time.IsZero() && !p.Time.Before(from) {
				return "easymoot: the post had already gone out to " + room + idSuffix(p.ID)
			}
		}
		return "easymoot: stopped; nothing was posted to " + room
	}
	return "easymoot: stopped waiting; the post to " + room + " may or may not have gone out"
}

func idSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " (" + cleanLine(id) + ")"
}
