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

// buildBinary builds the easymoot command, or returns "" when there is no go
// command on PATH. It has to run before newWorld points HOME at the synthetic
// home: the toolchain's own caches and telemetry belong in the real one, never in
// the world the session checks.
func buildBinary(t *testing.T) string {
	t.Helper()
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return ""
	}
	bin := filepath.Join(t.TempDir(), "easymoot")
	build := exec.Command(goCmd, "build", "-o", bin, "./cmd/easymoot")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// binarySteps runs the built command over the synthetic home with standard output
// redirected, which prints one frame and exits: how start-up finds the one folder
// to open, and what a typed path may be. The binary's HOME is the
// synthetic one; nothing else is read.
func binarySteps(t *testing.T, w *world, bin string) {
	if bin == "" {
		t.Skip("no go command on PATH: the binary steps did not run")
	}
	run := func(dir string, args ...string) (stdout, stderr string, err error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "HOME="+w.home, "MOOT_NAME=")
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err = cmd.Run()
		return o.String(), e.String(), err
	}
	expect := func(t *testing.T, what, out string, parts ...string) {
		t.Helper()
		for _, p := range parts {
			if !strings.Contains(out, p) {
				t.Errorf("%s: output lacks %q\n%s", what, p, out)
			}
		}
	}

	t.Run("no path, no .moot/ here: an error naming the folder, nothing drawn", func(t *testing.T) {
		out, errOut, err := run(w.path("repos"))
		if err == nil {
			t.Fatalf("exit 0, want an error; output:\n%s", out)
		}
		expect(t, "stderr", errOut, "easymoot:", "no .moot", w.path("repos"))
		if out != "" {
			t.Errorf("drew something for a folder with no .moot/\n%s", out)
		}
	})
	t.Run("no path, a .moot/ here: that moot", func(t *testing.T) {
		out, _, err := run(w.path("repos/atlas"))
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "atlas", out, "as atlas", "dev", "lobby", "runs", "~/repos/atlas")
	})
	t.Run("a path: that folder's moot, from anywhere", func(t *testing.T) {
		out, _, err := run(w.path("work"), "../repos/harbor")
		if err != nil {
			t.Fatal(err)
		}
		// harbor's two rooms, and atlas's lobby: with no --root, the walk runs from
		// repos/, where folders beside harbor hold a .moot/.
		expect(t, "harbor", out, "as harbor", "3 rooms", " atlas ", "lobby")
	})
	t.Run("a path may name the .moot directory itself", func(t *testing.T) {
		out, _, err := run(w.path("work"), w.path("repos/harbor/.moot"))
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "harbor", out, "as harbor")
	})
	t.Run("a moot with no rooms, beside others, lists theirs", func(t *testing.T) {
		out, _, err := run(w.path("work"), w.path("repos/ledger"))
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "ledger", out, "1 room · as ledger", " atlas ", "lobby")
	})
	t.Run("a moot with no rooms, --root at itself, says how rooms get linked", func(t *testing.T) {
		out, _, err := run(w.path("work"), w.path("repos/ledger"), "--root", w.path("repos/ledger"))
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "ledger", out, "No rooms.", "linked in by hand")
	})
	t.Run("~ opens ~/.moot", func(t *testing.T) {
		out, _, err := run(w.path("work"), w.home)
		if err != nil {
			t.Fatal(err)
		}
		expect(t, "home", out, "as ada", "lobby")
	})
	t.Run("a typed path with no .moot/ is an error, and nothing is drawn", func(t *testing.T) {
		out, errOut, err := run(w.path("repos"), "atlas/nowhere")
		if err == nil {
			t.Fatalf("exit 0, want an error; output:\n%s", out)
		}
		expect(t, "stderr", errOut, "easymoot:", "no .moot")
		if out != "" {
			t.Errorf("drew something for a typed path with no .moot/\n%s", out)
		}
	})
	t.Run("two paths are refused", func(t *testing.T) {
		if _, errOut, err := run(w.home, "a", "b"); err == nil || !strings.Contains(errOut, "one folder at a time") {
			t.Errorf("two paths: err %v, stderr %q", err, errOut)
		}
	})
}
