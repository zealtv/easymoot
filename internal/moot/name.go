package moot

import (
	"fmt"
	"os"
	"strings"
)

// Name resolves the sender name a post would go out under, read-only, by the chain
// of moot.sh's resolve_name (without --as, which Easymoot never passes):
//
//  1. $MOOT_NAME, if non-empty (an empty value is skipped);
//  2. the first line of .moot/name, whitespace removed (blank falls through);
//  3. ../.gremlin/name: the first name= line's value, else the file's first line
//     (an empty result falls through);
//  4. the name of the folder holding .moot/.
//
// A value found at any step must be filename-safe (A-Z a-z 0-9 . _ -, not starting
// with a dot); one that is not is an error, never a fall-through, as in moot.sh.
//
// It reads the same paths the script will: ../.gremlin is reached through
// <folder>/.moot/.., unresolved, as the script does it.
func (m *Moot) Name() (string, error) {
	if v := os.Getenv("MOOT_NAME"); v != "" {
		return checkSender(v)
	}
	return m.OwnName()
}

// OwnName is the name this folder goes by in the files alone: Name's chain without
// its first step, $MOOT_NAME. moot.sh names a DM room from the same files, so both
// sides compute the same room name; the environment variable is a per-session
// override and would give a different one. Errors as Name does.
func (m *Moot) OwnName() (string, error) {
	if v := firstLineName(m.dir + "/name"); v != "" {
		return checkSender(v)
	}
	if v := gremlinName(m.dir + "/../.gremlin/name"); v != "" {
		return checkSender(v)
	}
	name := m.folder[strings.LastIndex(m.folder, "/")+1:]
	if !safeSender(name) {
		return "", fmt.Errorf("folder name '%s' is not a filename-safe sender; write one to .moot/name", name)
	}
	return name, nil
}

func checkSender(v string) (string, error) {
	if !safeSender(v) {
		return "", fmt.Errorf("sender name '%s' is not filename-safe (use A-Z a-z 0-9 . _ -)", v)
	}
	return v, nil
}

// safeSender is moot.sh's check_segment: only A-Z a-z 0-9 . _ -, not starting with
// a dot, not empty.
func safeSender(s string) bool {
	if s == "" || s[0] == '.' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// stripSpace is `tr -d '[:space:]'` in the C locale.
func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			return -1
		}
		return r
	}, s)
}

func firstLine(data []byte) string {
	s, _, _ := strings.Cut(string(data), "\n")
	return s
}

// firstLineName is moot_name: the first line of the file, whitespace removed. An
// unreadable file gives "".
func firstLineName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return stripSpace(firstLine(data))
}

// gremlinName is gremlin_name: the value of the first name= line, whitespace
// removed, else (when that is empty or absent) the file's first line likewise.
func gremlinName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "name="); ok {
			if v = stripSpace(v); v != "" {
				return v
			}
			break
		}
	}
	return stripSpace(firstLine(data))
}
