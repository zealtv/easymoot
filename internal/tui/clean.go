package tui

import (
	"strings"
	"unicode"
)

// clean makes untrusted text safe to draw: tabs become spaces and every other
// control character, escape included, becomes a visible middle dot. Every piece
// of text that comes out of a moot (room names, sender names, post bodies, paths,
// errors) passes through here before it is drawn.
//
// Copied from Easyhog's internal/tui/state.go (commit 01336f8, `clean` and
// `cleanLine`), unchanged.
func clean(s string) string {
	var b strings.Builder
	for _, r := range strings.ReplaceAll(s, "\r\n", "\n") {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case unicode.IsControl(r) || r == ' ' || r == ' ':
			b.WriteRune('·')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleanLine is clean for a single-line label.
func cleanLine(s string) string {
	return strings.ReplaceAll(clean(s), "\n", " ")
}
