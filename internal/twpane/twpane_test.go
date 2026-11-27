package twpane

import (
	"errors"
	"strings"
	"testing"
)

func recorder() (*Pane, *[]string) {
	var calls []string
	p := &Pane{ID: "%7", Run: func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}}
	return p, &calls
}

func syncNow(p *Pane, v Values) bool {
	cmd := p.Sync(v)
	if cmd != nil {
		cmd()
	}
	return cmd != nil
}

// The first Sync states every option, unsetting the empty ones, so a value an
// earlier run left behind (it crashed, or was killed) does not linger on the
// map. After that only changes are sent, all of them in one tmux call.
func TestSyncSendsEverythingOnceThenOnlyChanges(t *testing.T) {
	p, calls := recorder()
	if !syncNow(p, Values{Next: "fix-flake +2 queued"}) {
		t.Fatal("the first Sync sent nothing")
	}
	if syncNow(p, Values{Next: "fix-flake +2 queued"}) {
		t.Fatal("an unchanged Sync sent a call")
	}
	if !syncNow(p, Values{Next: "fix-flake +2 queued", Status: "work", StatusText: "claiming fix-flake", StatusAt: "1764200000"}) {
		t.Fatal("a status did not send")
	}
	if !syncNow(p, Values{}) || syncNow(p, Values{}) {
		t.Fatal("clearing must send once")
	}
	want := []string{
		"set-option -p -t %7 @tw-next fix-flake +2 queued ; set-option -pu -t %7 @tw_status ; " +
			"set-option -pu -t %7 @tw_status_text ; set-option -pu -t %7 @tw_status_at",
		"set-option -p -t %7 @tw_status work ; set-option -p -t %7 @tw_status_text claiming fix-flake ; " +
			"set-option -p -t %7 @tw_status_at 1764200000",
		"set-option -pu -t %7 @tw-next ; set-option -pu -t %7 @tw_status ; " +
			"set-option -pu -t %7 @tw_status_text ; set-option -pu -t %7 @tw_status_at",
	}
	if strings.Join(*calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(*calls, "\n"), strings.Join(want, "\n"))
	}
}

// Every call names the pane: nothing EasyLoom sets lands on its window, where
// it would speak for the panes beside it.
func TestEveryCallIsPaneScoped(t *testing.T) {
	p, calls := recorder()
	syncNow(p, Values{Next: "a"})
	syncNow(p, Values{Next: "b", Status: "fail", StatusText: "tie a failed", StatusAt: "1"})
	_ = p.Clear()
	for _, c := range *calls {
		for _, part := range strings.Split(c, " ; ") {
			if !strings.HasPrefix(part, "set-option -p -t %7 ") && !strings.HasPrefix(part, "set-option -pu -t %7 ") {
				t.Errorf("not pane-scoped: %q", part)
			}
		}
	}
}

// No pane, no calls: outside tmux, and in tests.
func TestNoPaneSendsNothing(t *testing.T) {
	var p *Pane
	if p.Sync(Values{Next: "x"}) != nil || p.Clear() != nil {
		t.Fatal("a nil pane did something")
	}
	q := &Pane{Run: func(...string) error { t.Fatal("ran tmux with no pane"); return nil }}
	if q.Sync(Values{Next: "x"}) != nil || q.Clear() != nil {
		t.Fatal("a pane with no ID did something")
	}
}

// Clear unsets everything, whatever was sent before, and nothing is written
// after it: the program is on its way out.
func TestClearUnsetsEveryOwnedOption(t *testing.T) {
	p, calls := recorder()
	syncNow(p, Values{Next: "a"})
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	got := (*calls)[len(*calls)-1]
	for _, name := range owned {
		if !strings.Contains(got, "set-option -pu -t %7 "+name) {
			t.Errorf("Clear did not unset %s: %q", name, got)
		}
	}
	n := len(*calls)
	syncNow(p, Values{Next: "b"})
	if len(*calls) != n {
		t.Errorf("a Sync after Clear wrote: %q", (*calls)[n:])
	}
}

// The map keeps 120 cells of a text option and draws one line, so a long id
// or a multi-line refusal is cut here, on one line.
func TestCutKeepsOneLineOf120Cells(t *testing.T) {
	long := strings.Repeat("a", 200)
	if got := Cut(long); len([]rune(got)) != MaxCells || !strings.HasSuffix(got, "…") {
		t.Errorf("Cut(200 a) = %d runes %q", len([]rune(got)), got)
	}
	if got := Cut(strings.Repeat("a", MaxCells)); got != strings.Repeat("a", MaxCells) {
		t.Error("a value of exactly 120 cells was cut")
	}
	if got := Cut("tie x failed\nnot_loose_end\tmore"); got != "tie x failed not_loose_end more" {
		t.Errorf("control characters kept: %q", got)
	}
	wide := strings.Repeat("界", 70) // 140 cells
	if got := Cut(wide); len([]rune(got)) > 60 {
		t.Errorf("wide runes counted as one cell each: %d runes", len([]rune(got)))
	}
}

// tmux ends a command at an argument ending in ";", even inside a list it was
// handed whole, so such a value is sent escaped and the rest of the list
// still runs.
func TestATrailingSemicolonIsEscaped(t *testing.T) {
	p, calls := recorder()
	syncNow(p, Values{Status: "fail", StatusText: ";", StatusAt: "1"})
	syncNow(p, Values{Status: "fail", StatusText: "refused; see log;", StatusAt: "1"})
	got := strings.Join(*calls, "\n")
	for _, want := range []string{`@tw_status_text \; ;`, `@tw_status_text refused; see log\;`} {
		if !strings.Contains(got, want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
}

// Bubble Tea runs commands concurrently and never waits for them, so an older
// write may run after a newer one, or after Clear. Each write applies the
// newest values, and nothing is written once Clear has run.
func TestWritesApplyTheNewestValuesAndStopAtClear(t *testing.T) {
	p, calls := recorder()
	older := p.Sync(Values{Status: "work", StatusText: "claiming a", StatusAt: "1"})
	newer := p.Sync(Values{})
	newer()
	older() // late: must not put "work" back
	for _, c := range *calls {
		if strings.Contains(c, "@tw_status work") {
			t.Fatalf("a late write restored an old value: %q", c)
		}
	}

	late := p.Sync(Values{Next: "b"})
	if err := p.Clear(); err != nil {
		t.Fatal(err)
	}
	n := len(*calls)
	late()
	if len(*calls) != n {
		t.Errorf("a write after Clear ran: %q", (*calls)[n:])
	}
}

// A failed tmux call is tried again by the next write, rather than being
// remembered as done.
func TestAFailedWriteIsRetried(t *testing.T) {
	fail := true
	var calls []string
	p := &Pane{ID: "%7", Run: func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if fail {
			return errors.New("no server")
		}
		return nil
	}}
	syncNow(p, Values{Next: "a"})
	fail = false
	syncNow(p, Values{Next: "a", Status: "work", StatusText: "x", StatusAt: "1"})
	if last := calls[len(calls)-1]; !strings.Contains(last, "@tw-next a") {
		t.Errorf("the failed @tw-next was not sent again: %q", last)
	}
}

// Copies made by New share their state: Clear on the copy the program kept
// stops a write from a copy the model made later.
func TestNewSharesStateAcrossCopies(t *testing.T) {
	var calls []string
	orig := New("%7")
	orig.Run = func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil }
	later := orig
	write := later.Sync(Values{Next: "a"})
	if err := orig.Clear(); err != nil {
		t.Fatal(err)
	}
	n := len(calls)
	write()
	if len(calls) != n {
		t.Errorf("a copy wrote after Clear: %q", calls[n:])
	}
}
