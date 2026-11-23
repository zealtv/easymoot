//go:build unix

package acceptance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecursiveDiscoveryBinary(t *testing.T) {
	gate(t)
	bin := buildBinary(t)
	if bin == "" {
		t.Fatal("go command is required for binary acceptance")
	}
	root := t.TempDir()
	mkdir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, rel), 0755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("opened/.moot/rooms/local/posts")
	mkdir("scan/alpha/.moot/rooms/dev/posts")
	mkdir("scan/beta/.moot/rooms/dev/posts")
	mkdir("outside/.moot/rooms/shared/posts")
	mkdir("scan/vendor/copy/.moot/rooms/pruned/posts")
	for name, target := range map[string]string{"alias": "scan/alpha/.moot/rooms/dev", "remote": "outside/.moot/rooms/shared", "dangling": "missing"} {
		if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, "opened/.moot/rooms", name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "opened/.moot/rooms/local/posts/20260101-120000-ada.md"), []byte("binary recursive acceptance body\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+root, "MOOT_NAME=", "NO_COLOR=1")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("binary %v: %v\n%s", args, err, stderr.String())
		}
		return string(out)
	}
	opened, scan := filepath.Join(root, "opened"), filepath.Join(root, "scan")
	// With no --root the root is the opened folder's parent, as outside, beside it,
	// holds a .moot/.
	if plain := run(opened); plain != run(opened, "--root", root) || !strings.Contains(plain, "beta") {
		t.Fatalf("no --root, a .moot/ beside it: not the parent's walk\n%s", plain)
	}
	out := run(opened, "--root", scan)
	for _, want := range []string{"4 rooms", "as opened", "opened", "alpha", "beta", "outside", "shared", "binary recursive acceptance body"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "dev") != 2 || strings.Contains(out, "alias") || strings.Contains(out, "dangling") || strings.Contains(out, "pruned") {
		t.Fatal(out)
	}
	// The root is depth zero; heard rooms survive even a root-only scan.
	out = run("--root", scan, "--depth", "0", opened)
	if !strings.Contains(out, "3 rooms") || strings.Contains(out, "beta") {
		t.Fatal(out)
	}
	linked := filepath.Join(root, "scan-link")
	if err := os.Symlink(scan, linked); err != nil {
		t.Fatal(err)
	}
	if got := run(opened, "--root", linked); got != run(opened, "--root", scan) {
		t.Fatalf("symlink root differs:\n%s", got)
	}
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("synthetic"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{filepath.Join(root, "missing-root"), file} {
		cmd := exec.Command(bin, opened, "--root", bad)
		cmd.Env = append(os.Environ(), "HOME="+root, "MOOT_NAME=", "NO_COLOR=1")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "easymoot: --root") || !strings.Contains(stderr.String(), bad) {
			t.Fatalf("invalid root: err %v stdout %q stderr %q", err, stdout.String(), stderr.String())
		}
	}

	for _, depth := range []string{"-1", "oops"} {
		cmd := exec.Command(bin, "--depth", depth, opened)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "depth") {
			t.Fatalf("depth %q: %s, %v", depth, out, err)
		}
	}
}
