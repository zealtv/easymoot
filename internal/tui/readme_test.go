package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The README's terminal captures are rendered from the model over a synthetic moot
// (the one `synthetic` builds: no real name, room or post), at a size that reads in
// a README, and this test keeps them current: each fenced block that follows a
// `<!-- capture:NAME -->` marker has to equal the model's frame, trailing spaces
// aside. The README must show the shell capture; the others are there for any doc
// that wants them. To refresh after a change, run with EASYMOOT_CAPTURE_DIR set to a
// directory: it writes readme-NAME.txt there, and the block takes that text.

// readmeCaptures renders each capture the README shows, colour off.
func readmeCaptures(t *testing.T) map[string]string {
	t.Helper()
	f := synthetic(t)
	t.Setenv("HOME", filepath.Dir(f.folder)) // the header shows ~/atlas
	resize := func(m Model, w, h int) Model {
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		return next.(Model)
	}
	out := map[string]string{}

	m := resize(shown(t, f.open()), 80, 24)
	out["shell"] = m.View()

	b := openBarOn(t, resize(shown(t, &spySource{Moot: f.open()}), 80, 24))
	b, _ = feed(b, typeText("Fix pushed; rebuilding now.")...)
	out["bar"] = b.View()

	return out
}

// A capture sits in a four-backtick fence, since a post in it may hold a three-backtick one.
var captureBlock = regexp.MustCompile("(?s)<!-- capture:([a-z]+) -->\n````text\n(.*?)````")

func trimRight(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestReadmeCapturesAreCurrent(t *testing.T) {
	want := readmeCaptures(t)
	if dir := os.Getenv("EASYMOOT_CAPTURE_DIR"); dir != "" {
		for name, frame := range want {
			if err := os.WriteFile(filepath.Join(dir, "readme-"+name+".txt"), []byte(trimRight(frame)), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, m := range captureBlock.FindAllStringSubmatch(string(readme), -1) {
		name, got := m[1], m[2]
		frame, ok := want[name]
		if !ok {
			t.Errorf("README.md has a capture %q that no test renders", name)
			continue
		}
		found[name] = true
		if got != trimRight(frame) {
			t.Errorf("README.md's %q capture is out of date; the model now renders:\n%s", name, trimRight(frame))
		}
	}
	if !found["shell"] {
		t.Errorf("README.md has no <!-- capture:shell --> block")
	}
}
