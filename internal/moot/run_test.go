package moot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each verb against the pinned script: one success, with the line it prints, and one
// refusal, exit 1 with the message minus its "moot: " prefix. Moot's own suite holds
// the rest of the refusals.
func TestRunVerbsAgainstThePinnedScript(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "alice")
	carol := newFixture(t, "carol")
	carol.room("shared")
	f.room("kept")

	steps := []struct {
		args    []string
		line    string // the stdout line on success
		refusal string // part of the message on exit 1
	}{
		{args: []string{"create", "ops"}, line: "created rooms/ops"},
		{args: []string{"create", "ops"}, refusal: "already here"},
		{args: []string{"join", filepath.Join(carol.dir, "rooms", "shared")}, line: "joined rooms/shared"},
		{args: []string{"join", f.path("elsewhere")}, refusal: "is not a room"},
		{args: []string{"archive", "shared"}, refusal: "only its keeper archives it"},
		{args: []string{"archive", "ops"}, line: "archived rooms/ops.archived"},
		{args: []string{"leave", "ops"}, refusal: "kept here (a real folder)"},
		{args: []string{"leave", "shared"}, line: "left rooms/shared"},
	}
	for _, s := range steps {
		res, err := f.open().Run(context.Background(), s.args)
		var se *ScriptError
		switch {
		case s.refusal == "" && err != nil:
			t.Errorf("%v: %v", s.args, err)
		case s.refusal == "" && (len(res.Lines) != 1 || !strings.HasPrefix(res.Lines[0], s.line)):
			t.Errorf("%v printed %q, want %q", s.args, res.Lines, s.line)
		case s.refusal != "" && (!errors.As(err, &se) || se.ExitCode != 1 || !strings.Contains(se.Message, s.refusal)):
			t.Errorf("%v gave %v, want exit 1 saying %q", s.args, err, s.refusal)
		}
	}
}

// Link repair's stderr lines come back as notices on a success.
func TestRunReportsLinkRepairAsNotices(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "alice")
	carol := newFixture(t, "carol")
	carol.room("shared")
	if err := os.Symlink(carol.path(".moot/rooms/shared"), f.path(".moot/rooms/shared")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(carol.path(".moot/rooms/shared"), carol.path(".moot/rooms/shared.archived")); err != nil {
		t.Fatal(err)
	}
	res, err := f.open().Run(context.Background(), []string{"rooms"})
	if err != nil || len(res.Notices) == 0 || !strings.Contains(res.Notices[0], "shared.archived") {
		t.Fatalf("rooms gave %+v, %v; want a notice naming shared.archived", res, err)
	}
}

// No script and a script that outlasts the bound are told apart from a refusal.
func TestRunNoScriptAndTimeout(t *testing.T) {
	f := newFixture(t, "alice")
	f.writeMode(".moot/moot.sh", "#!/bin/bash\nsleep 30\n", 0o755)
	if _, err := f.open().Run(context.Background(), []string{"rooms"}, WithTimeout(postTestBound)); !errors.Is(err, ErrTimeout) {
		t.Errorf("slow script: %v, want ErrTimeout", err)
	}
	if err := os.Remove(f.path(".moot/moot.sh")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.open().Run(context.Background(), []string{"rooms"}); !errors.Is(err, ErrNoScript) {
		t.Errorf("no script: %v, want ErrNoScript", err)
	}
}
