package moot

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeScript replaces the folder's moot.sh with a small script of the test's own.
func (f *fixture) fakeScript(body string) {
	f.t.Helper()
	f.writeMode(".moot/moot.sh", "#!/usr/bin/env bash\nset -u\nd=$(cd \"$(dirname \"$0\")/..\" && pwd -P)\n"+body, 0o755)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPostRunsExactlyTheDocumentedCall(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	f.fakeScript(`
printf '%s\0' "$@" > "$d/argv"
printf '%s' "$0" > "$d/arg0"
pwd -P > "$d/cwd"
cat > "$d/stdin"
printf 'moot: warning: a notice\n' >&2
printf '{"schema_version":1,"command":"post","ok":true,"changed":true,"room":"r","id":"20260829-100000-fake","path":"rooms/r/posts/x.md","sender":"fake","timestamp":"t"}\n'
`)
	text := "-m not a flag\n\n\"quoted\" 'single' $HOME `tick`\n\n\nlast\n"
	res, err := f.open().Post(context.Background(), "r", text)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Split(strings.TrimSuffix(readFile(t, f.folder+"/argv"), "\x00"), "\x00")
	if want := []string{"post", "--json", "--room", "r", "-"}; !reflect.DeepEqual(argv, want) {
		t.Errorf("argv %q, want %q", argv, want)
	}
	if got := readFile(t, f.folder+"/arg0"); got != f.dir+"/moot.sh" {
		t.Errorf("script run as %q, want the absolute path %q", got, f.dir+"/moot.sh")
	}
	if got := readFile(t, f.folder+"/cwd"); strings.TrimSpace(got) != f.folder {
		t.Errorf("cwd %q, want the folder %q", got, f.folder)
	}
	if got := readFile(t, f.folder+"/stdin"); got != text {
		t.Errorf("stdin %q, want the text %q", got, text)
	}
	if res.Room != "r" || res.ID != "20260829-100000-fake" || res.Sender != "fake" {
		t.Errorf("result %+v", res)
	}
	if want := []string{"warning: a notice"}; !reflect.DeepEqual(res.Notices, want) {
		t.Errorf("notices %q, want %q", res.Notices, want)
	}
}

func TestPostWithPinnedScriptKeepsTheBodyWhole(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "proj")
	f.room("r")
	m := f.open()
	texts := []string{
		"-m not a flag\n— proj",
		"she said \"it's\" $HOME `ls` \\n back\\slash",
		"first\n\n\nafter two blank lines\n\n\n",
		"  leading spaces\n\ttabbed\nüñí ©",
	}
	for i, text := range texts {
		res, err := m.Post(context.Background(), "r", text)
		if err != nil {
			t.Fatalf("text %d: %v", i, err)
		}
		if res.Room != "r" || res.Sender != "proj" || res.ID == "" {
			t.Fatalf("text %d: result %+v", i, res)
		}
		want := strings.TrimRight(text, "\n") + "\n" // the script trims trailing newlines
		if got := readFile(t, f.dir+"/rooms/r/posts/"+res.ID+".md"); got != want {
			t.Errorf("text %d: file holds %q, want %q", i, got, want)
		}
		if name, _ := m.Name(); name != res.Sender {
			t.Errorf("text %d: Name %q, post went out as %q", i, name, res.Sender)
		}
	}
	posts, err := m.List("r")
	if err != nil || len(posts) != len(texts) {
		t.Fatalf("listed %d posts (%v), want %d", len(posts), err, len(texts))
	}
}

func TestPostRefusals(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "proj")
	f.room("r")
	m := f.open()
	cases := []struct {
		name, room, text, want string
	}{
		{"empty body", "r", "", "empty message"},
		{"blank body", "r", " \n\t\n\n", "empty message"},
		{"missing room", "nope", "hello", "no posts/ in room nope"},
		{"unsafe room", "a/b", "hello", "not filename-safe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := m.Post(context.Background(), tc.room, tc.text)
			var se *ScriptError
			if !errors.As(err, &se) {
				t.Fatalf("got %v, want a *ScriptError", err)
			}
			if !strings.Contains(se.Error(), tc.want) || strings.HasPrefix(se.Error(), "moot: ") || se.ExitCode != 1 {
				t.Errorf("error %q (exit %d), want it to say %q without a prefix", se, se.ExitCode, tc.want)
			}
			if res.ID != "" {
				t.Errorf("a failed post has an id: %+v", res)
			}
		})
	}
	if entries, _ := os.ReadDir(f.dir + "/rooms/r/posts"); len(entries) != 0 {
		t.Errorf("failed posts left files: %v", entries)
	}
	if _, err := os.Stat(f.dir + "/rooms/nope"); err == nil {
		t.Error("a missing room was made")
	}
}

func TestPostNeedsARoomAndAScript(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	m := f.open()
	if _, err := m.Post(context.Background(), "", "hi"); err == nil {
		t.Error("an empty room posted")
	}
	if err := os.Remove(f.dir + "/moot.sh"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Post(context.Background(), "r", "hi"); !errors.Is(err, ErrNoScript) {
		t.Errorf("got %v, want ErrNoScript", err)
	}
}

func TestPostReportsScriptFailureAndOddSuccess(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	m := f.open()

	f.fakeScript("cat >/dev/null; echo 'moot: boom' >&2; echo 'moot: second' >&2; exit 3\n")
	_, err := m.Post(context.Background(), "r", "hi")
	var se *ScriptError
	if !errors.As(err, &se) || se.ExitCode != 3 || se.Message != "boom\nsecond" {
		t.Errorf("got %#v", err)
	}

	f.fakeScript("cat >/dev/null; exit 4\n")
	_, err = m.Post(context.Background(), "r", "hi")
	if !errors.As(err, &se) || !strings.Contains(se.Error(), "status 4") {
		t.Errorf("silent failure: %v", err)
	}

	// Exit zero without the expected JSON counts as posted, with nothing to report.
	f.fakeScript("cat >/dev/null; echo 'posted somewhere'\n")
	res, err := m.Post(context.Background(), "r", "hi")
	if err != nil || res.ID != "" || res.Room != "" {
		t.Errorf("odd success: %+v, %v", res, err)
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func pidFile(t *testing.T, path string) int {
	t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(readFile(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func settleGoroutines(t *testing.T, before int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("goroutines left running: %d, started with %d", runtime.NumGoroutine(), before)
}

// postTestBound is the time-out the time-out tests give a fake script. It has to cover
// bash starting and writing its pid file on a loaded machine: 300ms didn't, on
// 2026-11-19, with the load average at 7.5.
const postTestBound = 2 * time.Second

func TestPostTimeoutKillsTheScript(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	f.fakeScript("echo $$ > \"$d/pid\"\nexec sleep 60\n") // the sleeping script itself
	m := f.open()
	before := runtime.NumGoroutine()

	start := time.Now()
	_, err := m.Post(context.Background(), "r", "hi", WithTimeout(postTestBound))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	if took := time.Since(start); took > postTestBound+5*time.Second {
		t.Errorf("took %v for a %v bound", took, postTestBound)
	}
	pid := pidFile(t, f.folder+"/pid")
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	if alive(pid) {
		// It may be a zombie for a moment; Wait has reaped it by now.
		t.Errorf("script %d still running after the time-out", pid)
	}
	settleGoroutines(t, before)
}

func TestPostTimeoutKillsOnlyTheDirectChild(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	// A "git step": a descendant that holds the script's pipes open.
	f.fakeScript("sleep 30 &\necho $! > \"$d/gpid\"\necho $$ > \"$d/pid\"\nwait\n")
	m := f.open()
	before := runtime.NumGoroutine()

	start := time.Now()
	_, err := m.Post(context.Background(), "r", "hi", WithTimeout(postTestBound))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	// The bound plus the 1 s WaitDelay for the pipes the descendant holds.
	if took := time.Since(start); took > postTestBound+5*time.Second {
		t.Errorf("took %v", took)
	}
	gpid := pidFile(t, f.folder+"/gpid")
	t.Cleanup(func() { syscall.Kill(gpid, syscall.SIGKILL) })
	if pid := pidFile(t, f.folder+"/pid"); alive(pid) {
		t.Errorf("script %d still running", pid)
	}
	if !alive(gpid) {
		t.Errorf("the script's descendant %d was killed too", gpid)
	}
	settleGoroutines(t, before)
}

func TestPostStopsWhenTheCallerCancels(t *testing.T) {
	f := newFixture(t, "proj")
	f.room("r")
	f.fakeScript("exec sleep 60\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := f.open().Post(ctx, "r", "hi")
	if errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want the caller's context error", err)
	}
}

func TestPostDefaultBound(t *testing.T) {
	if DefaultPostTimeout != 30*time.Second {
		t.Errorf("default bound %v, the architecture says 30 s", DefaultPostTimeout)
	}
	var c postConfig
	WithTimeout(0)(&c)
	WithTimeout(-time.Second)(&c)
	if c.timeout != 0 {
		t.Errorf("non-positive bound changed the config: %v", c.timeout)
	}
}
