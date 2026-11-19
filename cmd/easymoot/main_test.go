package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zealtv/easymoot/internal/moot"
)

func exercise(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(args, strings.NewReader(""), &out, &errOut)
	return out.String(), errOut.String(), err
}

func TestVersionFlagPrintsAVersion(t *testing.T) {
	stdout, _, err := exercise(t, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("--version printed nothing")
	}
}

func TestVersionFlagWinsOverAPath(t *testing.T) {
	stdout, _, err := exercise(t, "--version", "some/folder")
	if err != nil || strings.TrimSpace(stdout) == "" {
		t.Fatalf("--version with a path: %q, %v", stdout, err)
	}
}

func TestOneFolderAtATime(t *testing.T) {
	_, _, err := exercise(t, "a", "b")
	if err == nil || !strings.Contains(err.Error(), "one folder at a time") {
		t.Fatalf("two paths: got %v", err)
	}
}

func TestUnknownFlagFails(t *testing.T) {
	if _, _, err := exercise(t, "--nope"); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}

func TestAFolderWithNoMootIsAnError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := exercise(t, dir)
	if err == nil || !errors.Is(err, moot.ErrNoMoot) {
		t.Fatalf("a folder with no .moot: got %v, want ErrNoMoot", err)
	}
}

// With no path and no .moot/ in the working directory, run fails with an error that
// names the folder, and prints nothing: no frame.
func TestNoPathAndNoMootIsAnErrorNamingTheFolder(t *testing.T) {
	here := t.TempDir()
	t.Chdir(here)
	want, err := filepath.EvalSymlinks(here)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := exercise(t)
	if !errors.Is(err, moot.ErrNoMoot) {
		t.Fatalf("got %v, want ErrNoMoot", err)
	}
	if !strings.Contains(err.Error(), filepath.Base(here)) {
		t.Errorf("error does not name the folder %s: %v", want, err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("drew something: stdout %q, stderr %q", stdout, stderr)
	}
}

func TestNoPathWithAMootHereOpensIt(t *testing.T) {
	here := t.TempDir()
	if err := os.MkdirAll(filepath.Join(here, ".moot", "rooms", "lobby", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(here)
	stdout, _, err := exercise(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "lobby") {
		t.Fatalf("expected the shell on the moot here:\n%s", stdout)
	}
}

// A path that does not exist is the same error.
func TestAPathThatDoesNotExistIsAnError(t *testing.T) {
	_, _, err := exercise(t, filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, moot.ErrNoMoot) {
		t.Fatalf("a path that does not exist: got %v, want ErrNoMoot", err)
	}
}

// With output that is no terminal the moot is read once and one frame is printed:
// a synthetic moot, never a real one.
func TestRedirectedOutputIsOneFrameOfTheMoot(t *testing.T) {
	dir := t.TempDir()
	posts := filepath.Join(dir, ".moot", "rooms", "lobby", "posts")
	if err := os.MkdirAll(posts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(posts, "20260101-090000-ada.md"), []byte("hello from a synthetic moot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := exercise(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lobby", "ada", "hello from a synthetic moot"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("frame lacks %q:\n%s", want, stdout)
		}
	}
}
