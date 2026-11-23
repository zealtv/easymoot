package moot

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pinnedScript = "testdata/upstream-ce10b46/moot.sh"

// fixture is a synthetic moot under t.TempDir(): <root>/<folder>/.moot/.
type fixture struct {
	t      *testing.T
	root   string
	folder string // absolute folder path, as the test built it
	dir    string // folder/.moot
}

// newFixture makes a folder with a .moot/ holding a copy of the pinned moot.sh
// (the real script is never run anywhere else) and an empty rooms/.
func newFixture(t *testing.T, folderName string) *fixture {
	t.Helper()
	root := t.TempDir()
	// t.TempDir may sit behind a symlink (macOS /var); resolve it once so the
	// folder path is the same in the test and in the script.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, folder: filepath.Join(root, folderName)}
	f.dir = filepath.Join(f.folder, ".moot")
	f.mkdir(".moot/rooms")
	script, err := os.ReadFile(pinnedScript)
	if err != nil {
		t.Fatal(err)
	}
	f.writeMode(".moot/moot.sh", string(script), 0o755)
	return f
}

func (f *fixture) path(rel string) string { return filepath.Join(f.folder, rel) }

func (f *fixture) mkdir(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(f.path(rel), 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) write(rel, content string) { f.t.Helper(); f.writeMode(rel, content, 0o644) }

func (f *fixture) writeMode(rel, content string, mode os.FileMode) {
	f.t.Helper()
	p := f.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
}

// room makes rooms/<name>/posts.
func (f *fixture) room(name string) { f.t.Helper(); f.mkdir(".moot/rooms/" + name + "/posts") }

func (f *fixture) open() *Moot {
	f.t.Helper()
	m, err := Open(f.folder)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

// setEnv sets MOOT_NAME for this test, or unsets it when value is nil, so the
// developer's own environment never leaks into the script or the Go chain.
func setMootName(t *testing.T, value *string) {
	t.Helper()
	t.Setenv("MOOT_NAME", "") // registers the restore
	if value == nil {
		os.Unsetenv("MOOT_NAME")
	} else {
		t.Setenv("MOOT_NAME", *value)
	}
}

func str(s string) *string { return &s }

// script runs the folder's pinned moot.sh with the given stdin and returns stdout,
// stderr and the error. The environment is the test process's (MOOT_NAME already
// controlled by setMootName).
func (f *fixture) script(stdin string, args ...string) (string, string, error) {
	f.t.Helper()
	cmd := exec.Command(f.dir+"/moot.sh", args...)
	cmd.Dir = f.folder
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}
