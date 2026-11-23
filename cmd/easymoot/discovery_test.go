package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryFlags(t *testing.T) {
	for _, args := range [][]string{{"--root", "tree", "folder"}, {"folder", "--root", "tree"}} {
		opts, err := parseFlags(args, io.Discard)
		if err != nil || opts.root != "tree" || opts.path != "folder" || opts.depth != 4 {
			t.Fatalf("%v: %+v %v", args, opts, err)
		}
	}
	opts, err := parseFlags([]string{"--depth=0", "--root=tree", "folder"}, io.Discard)
	if err != nil || opts.depth != 0 {
		t.Fatalf("zero depth: %+v %v", opts, err)
	}
	for _, depth := range []string{"-1", "many"} {
		_, err := parseFlags([]string{"--depth", depth}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "depth") {
			t.Fatalf("depth %q: %v", depth, err)
		}
	}
}

func TestFlagTerminatorPreservesLiteralFolder(t *testing.T) {
	opts, err := parseFlags([]string{"--root", "tree", "--", "-folder"}, io.Discard)
	if err != nil || opts.path != "-folder" {
		t.Fatalf("literal folder: %+v %v", opts, err)
	}
}

func TestDiscoveryFlagsRequireTheirArguments(t *testing.T) {
	for _, flag := range []string{"--root", "--depth"} {
		if _, err := parseFlags([]string{"folder", flag}, io.Discard); err == nil || !strings.Contains(err.Error(), "argument") {
			t.Fatalf("missing %s argument: %v", flag, err)
		}
	}
}

func TestRunValidatesRootBeforeDrawing(t *testing.T) {
	root := t.TempDir()
	opened := filepath.Join(root, "opened")
	if err := os.MkdirAll(filepath.Join(opened, ".moot/rooms/local/posts"), 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "missing"), dangling); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{filepath.Join(root, "missing"), file, dangling} {
		t.Run(filepath.Base(bad), func(t *testing.T) {
			stdout, _, err := exercise(t, opened, "--root", bad)
			if err == nil || !strings.Contains(err.Error(), "--root") || !strings.Contains(err.Error(), bad) {
				t.Fatalf("root %q: %v", bad, err)
			}
			if stdout != "" {
				t.Fatalf("drew a frame for invalid root:\n%s", stdout)
			}
		})
	}
	scan := filepath.Join(root, "scan")
	if err := os.MkdirAll(filepath.Join(scan, "keeper/.moot/rooms/remote/posts"), 0755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(scan, linked); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := exercise(t, opened, "--root", linked)
	if err != nil || !strings.Contains(stdout, "remote") || !strings.Contains(stdout, "local") || !strings.Contains(stdout, "2 rooms") {
		t.Fatalf("linked root: %v\n%s", err, stdout)
	}
}

// With no --root the root is the opened folder's parent when a folder beside it holds a
// .moot/; with none there no walk runs; --root overrides either way.
func TestRootDefaultsToTheParentBesideAnotherMoot(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(base, rel), 0755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("alone/opened/.moot/rooms/local/posts")
	mkdir("alone/plain")
	mkdir("near/opened/.moot/rooms/local/posts")
	mkdir("near/sibling/.moot/rooms/beside/posts")
	mkdir("other/keeper/.moot/rooms/elsewhere/posts")
	if got := defaultRoot(filepath.Join(base, "alone/opened")); got != "" {
		t.Fatalf("no .moot/ beside it: root %q", got)
	}
	// The same folder under another spelling (a case difference on a case-insensitive
	// disk, here a link) is not a sibling.
	if err := os.Symlink("opened", filepath.Join(base, "alone/Opened-again")); err != nil {
		t.Fatal(err)
	}
	if got := defaultRoot(filepath.Join(base, "alone/opened")); got != "" {
		t.Fatalf("itself under another name counted as a sibling: root %q", got)
	}
	if got := defaultRoot(filepath.Join(base, "near/opened")); got != filepath.Join(base, "near") {
		t.Fatalf("a .moot/ beside it: root %q", got)
	}
	for _, c := range []struct {
		args       []string
		want, lack string
	}{
		{[]string{filepath.Join(base, "alone/opened")}, "1 room", "beside"},
		{[]string{filepath.Join(base, "near/opened")}, "beside", "elsewhere"},
		{[]string{filepath.Join(base, "near/opened"), "--root", filepath.Join(base, "other")}, "elsewhere", "beside"},
	} {
		stdout, _, err := exercise(t, c.args...)
		if err != nil || !strings.Contains(stdout, c.want) || strings.Contains(stdout, c.lack) {
			t.Fatalf("%v: want %q and no %q (%v):\n%s", c.args, c.want, c.lack, err, stdout)
		}
	}
}
