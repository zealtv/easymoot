package moot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultPostTimeout bounds a Post unless WithTimeout says otherwise (Easyhog's
// client default).
const DefaultPostTimeout = 30 * time.Second

// ErrTimeout reports that moot.sh did not finish within the bound. It was killed,
// but the post may already have landed.
var ErrTimeout = errors.New("moot.sh did not finish in time")

// ErrNoScript reports a .moot/ with no moot.sh to post through.
var ErrNoScript = errors.New("no moot.sh in .moot")

// ScriptError is a failed run: moot.sh exited non-zero. Message is what the script
// said on standard error ("moot: " prefixes removed).
type ScriptError struct {
	Message  string
	ExitCode int
}

func (e *ScriptError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("moot.sh exited with status %d", e.ExitCode)
}

// PostOption adjusts one Post call.
type PostOption func(*postConfig)

type postConfig struct{ timeout time.Duration }

// WithTimeout sets the bound (default DefaultPostTimeout). A value of zero or less
// leaves the default.
func WithTimeout(d time.Duration) PostOption {
	return func(c *postConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// PostResult is what a successful post reports.
type PostResult struct {
	// Room, ID and Sender come from the script's JSON answer. They stay empty if
	// the script exited zero without printing the expected object.
	Room, ID, Sender string
	// Notices are the lines the script wrote to standard error ("moot: " prefixes
	// removed).
	Notices []string
}

// maxOutput caps what Post keeps of each of the script's output streams.
const maxOutput = 64 << 10

// Post posts text to a room by running the folder's own script:
//
//	<folder>/.moot/moot.sh post --json --room <room> -
//
// with the text on standard input (see execScript for how it runs). It is the one
// write that carries text; Run is the other way in. On the bound passing, only the
// script itself (the direct child) is killed, and ErrTimeout is returned; the result
// then holds any notices written so far.
func (m *Moot) Post(ctx context.Context, room, text string, opts ...PostOption) (PostResult, error) {
	cfg := postConfig{timeout: DefaultPostTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	if room == "" {
		return PostResult{}, errors.New("no room to post to")
	}
	stdout, notices, err := m.execScript(ctx, cfg, text, "post", "--json", "--room", room, "-")
	res := PostResult{Notices: notices}
	if err != nil {
		return res, err
	}
	var answer struct{ Room, ID, Sender string }
	if json.Unmarshal(bytes.TrimSpace([]byte(stdout)), &answer) == nil {
		res.Room, res.ID, res.Sender = answer.Room, answer.ID, answer.Sender
	}
	return res, nil
}

// execScript runs the folder's own script with args and stdin:
//
//	<folder>/.moot/moot.sh <args...>
//
// with the folder as working directory, the environment inherited, no shell and no
// $PATH lookup. It returns the script's standard output, and the lines it wrote to
// standard error ("moot: " prefixes removed) even when it fails. A script that exits
// non-zero is a *ScriptError whose message is the lines the script itself said
// ("moot: …"), or all its standard error when it said none (a shell's own complaint).
// Post and Run are the two callers; both go through here, so the one bound, the one
// kill and the one way of reading an answer apply to every verb.
func (m *Moot) execScript(ctx context.Context, cfg postConfig, stdin string, args ...string) (string, []string, error) {
	script := m.dir + "/moot.sh"
	if info, err := os.Stat(script); err != nil || info.IsDir() {
		return "", nil, ErrNoScript
	}

	callCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	var stdout, stderr limitedBuffer
	cmd := exec.CommandContext(callCtx, script, args...)
	cmd.Dir = m.folder
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Descendants may keep the pipes open after the script dies; bound
	// the draining without signalling them.
	cmd.WaitDelay = time.Second
	err := cmd.Run()

	notices := lines(stderr.String())
	if err != nil {
		if ctx.Err() != nil {
			return "", notices, ctx.Err()
		}
		if callCtx.Err() != nil {
			return "", notices, ErrTimeout
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			said := notices
			if own := mootLines(stderr.String()); len(own) > 0 {
				said = own
			}
			return "", notices, &ScriptError{Message: strings.Join(said, "\n"), ExitCode: exit.ExitCode()}
		}
		return "", notices, err
	}
	return stdout.String(), notices, nil
}

// mootLines is the lines of s that the script itself wrote ("moot: …"), without the
// prefix. A refusal is one such line; the usage text an unknown command prints
// before it is not.
func mootLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l, ok := strings.CutPrefix(strings.TrimSpace(l), "moot: "); ok {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
	}
	return out
}

// lines splits script output into non-blank lines without the "moot: " prefix.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "moot: "))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// limitedBuffer keeps the first maxOutput bytes written and drops the rest.
type limitedBuffer struct{ buf bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxOutput - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
