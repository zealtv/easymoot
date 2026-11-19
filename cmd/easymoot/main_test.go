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

// fakeHome points HOME at a synthetic home directory holding two moots, so that the
// folder picker scans that and never the real one.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, folder := range []string{"repos/alpha", "repos/beta"} {
		if err := os.MkdirAll(filepath.Join(home, folder, ".moot", "rooms", "dev", "posts"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	return home
}

// With no path and no .moot/ in the working directory, the folder picker opens. Here
// the output is no terminal, so the one frame printed is the picker over the scan of
// a fake home.
func TestNoPathAndNoMootOpensThePicker(t *testing.T) {
	fakeHome(t)
	// The test's working directory, cmd/easymoot, has no .moot.
	stdout, _, err := exercise(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Choose a folder", "alpha", "beta", "~/repos/alpha", "1 room", "2 folders"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("frame lacks %q:\n%s", want, stdout)
		}
	}
}

func TestNoPathWithAMootHereOpensIt(t *testing.T) {
	fakeHome(t)
	here := t.TempDir()
	if err := os.MkdirAll(filepath.Join(here, ".moot", "rooms", "lobby", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(here)
	stdout, _, err := exercise(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "lobby") || strings.Contains(stdout, "Choose a folder") {
		t.Fatalf("expected the shell on the moot here:\n%s", stdout)
	}
}

// A typed path with no .moot/ stays an error, whatever the picker could offer.
func TestATypedPathWithNoMootIsStillAnErrorNotAPicker(t *testing.T) {
	fakeHome(t)
	_, _, err := exercise(t, t.TempDir())
	if !errors.Is(err, moot.ErrNoMoot) {
		t.Fatalf("got %v, want ErrNoMoot", err)
	}
	_, _, err = exercise(t, filepath.Join(t.TempDir(), "missing"))
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
