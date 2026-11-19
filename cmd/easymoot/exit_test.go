package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests run the terminal interface over pipes (forceInteractive), on a synthetic
// moot in a temp dir, and press keys by writing bytes. The room's moot.sh is a wrapper
// that holds in a read, then runs the pinned script, so ctrl+c finds a post in flight.

// heldMoot builds a folder named atlas with a dev room and a wrapper moot.sh. The
// wrapper writes its pid to <ctl>/pid, then runs extra (a shell fragment) and holds.
func heldMoot(t *testing.T, extra string) (folder, ctl string) {
	t.Helper()
	t.Setenv("MOOT_NAME", "")
	folder = filepath.Join(t.TempDir(), "atlas")
	moots := filepath.Join(folder, ".moot")
	if err := os.MkdirAll(filepath.Join(moots, "rooms", "dev", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../internal/moot/testdata/upstream-c25b5bb/moot.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moots, "moot-pinned.sh"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	ctl = t.TempDir()
	hold := filepath.Join(ctl, "hold")
	if err := syscall.Mkfifo(hold, 0o600); err != nil {
		t.Skipf("no fifo here: %v", err)
	}
	wrapper := "#!/usr/bin/env bash\necho $$ > '" + filepath.Join(ctl, "pid") + "'\n" + extra +
		"\nread -t 60 <> '" + hold + "'\n"
	if err := os.WriteFile(filepath.Join(moots, "moot.sh"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	return folder, ctl
}

// session runs easymoot over pipes. keys writes to its input.
type session struct {
	t      *testing.T
	in     *os.File
	done   chan error
	stderr *bytes.Buffer
}

func startSession(t *testing.T, folder string) *session {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	forceInteractive = true
	t.Cleanup(func() { forceInteractive = false; r.Close(); w.Close() })
	s := &session{t: t, in: w, done: make(chan error, 1), stderr: &bytes.Buffer{}}
	go func() { s.done <- run([]string{folder}, r, &bytes.Buffer{}, s.stderr) }()
	return s
}

// keys presses one key at a time: Bubble Tea reads a run of bytes that arrive together
// as one paste-like message, which the palette and the bar rightly never act on.
func (s *session) keys(text string) {
	s.t.Helper()
	for i := 0; i < len(text); i++ {
		_, _ = s.in.WriteString(text[i : i+1])
		time.Sleep(15 * time.Millisecond)
	}
}

// startPost types a post until the wrapper script has started.
func (s *session) startPost(ctl string) {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(ctl, "pid")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatal("the post never started")
		}
		// Until the first read has landed there is no room or name, and the palette
		// refuses; each try is whole, and keys after the post starts are inert.
		s.keys(":post\rhello\r")
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *session) wait() error {
	s.t.Helper()
	select {
	case err := <-s.done:
		return err
	case <-time.After(15 * time.Second):
		s.t.Fatal("easymoot did not quit")
		return nil
	}
}

// Nothing was posted: the held script is killed before it posts.
func TestCtrlCWhilePostingPrintsThatNothingWasPosted(t *testing.T) {
	folder, ctl := heldMoot(t, "")
	s := startSession(t, folder)
	s.startPost(ctl)
	s.keys("\x03")
	if err := s.wait(); err != nil {
		t.Fatal(err)
	}
	if got, want := s.stderr.String(), "easymoot: stopped; nothing was posted to dev\n"; got != want {
		t.Fatalf("stderr %q, want %q", got, want)
	}
}

// The post landed before ctrl+c killed the script (as in a clone room, where the git
// steps follow): the line says it went out, with the id.
func TestCtrlCWhilePostingPrintsThatThePostHadGoneOut(t *testing.T) {
	folder, ctl := heldMoot(t, "\"$(dirname \"$0\")/moot-pinned.sh\" \"$@\" >/dev/null")
	s := startSession(t, folder)
	s.startPost(ctl)
	time.Sleep(300 * time.Millisecond) // the pinned script has run; the wrapper holds
	s.keys("\x03")
	if err := s.wait(); err != nil {
		t.Fatal(err)
	}
	got := s.stderr.String()
	if !strings.HasPrefix(got, "easymoot: the post had already gone out to dev (") || !strings.HasSuffix(got, "-atlas)\n") {
		t.Fatalf("stderr %q", got)
	}
	posts, _ := filepath.Glob(filepath.Join(folder, ".moot", "rooms", "dev", "posts", "*-atlas.md"))
	if len(posts) != 1 || !strings.Contains(got, strings.TrimSuffix(filepath.Base(posts[0]), ".md")) {
		t.Fatalf("posts %v do not match the line %q", posts, got)
	}
}

// A second ctrl+c before the script has been gone a second: it may or may not have
// gone out. A background sleep keeps the script's pipes open, so the poster waits its
// second before it answers.
func TestSecondCtrlCPrintsThatItMayOrMayNotHaveGoneOut(t *testing.T) {
	folder, ctl := heldMoot(t, "sleep 30 &")
	s := startSession(t, folder)
	s.startPost(ctl)
	s.keys("\x03")
	time.Sleep(150 * time.Millisecond)
	s.keys("\x03")
	if err := s.wait(); err != nil {
		t.Fatal(err)
	}
	if got, want := s.stderr.String(), "easymoot: stopped waiting; the post to dev may or may not have gone out\n"; got != want {
		t.Fatalf("stderr %q, want %q", got, want)
	}
}

// Any other quit prints nothing.
func TestOtherQuitsPrintNothing(t *testing.T) {
	for name, keys := range map[string]string{"ctrl+c": "\x03", "palette quit": ":quit\r"} {
		t.Run(name, func(t *testing.T) {
			folder, _ := heldMoot(t, "")
			s := startSession(t, folder)
			time.Sleep(300 * time.Millisecond)
			s.keys(keys)
			if err := s.wait(); err != nil {
				t.Fatal(err)
			}
			if s.stderr.Len() != 0 {
				t.Fatalf("stderr %q, want nothing", s.stderr.String())
			}
		})
	}
}
