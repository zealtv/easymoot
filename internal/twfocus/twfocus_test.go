package twfocus

import (
	"strings"
	"testing"
)

func TestRectString(t *testing.T) {
	for r, want := range map[Rect]string{
		{1, 2, 30, 10, false}: "1,2,30,10",
		{1, 2, 30, 10, true}:  "1,2,30,10,1",
		{}:                    "",
		{0, 0, 5, 0, true}:    "",
		{-1, 0, 5, 5, false}:  "",
	} {
		if got := r.String(); got != want {
			t.Errorf("%+v.String() = %q, want %q", r, got, want)
		}
	}
}

func TestReportSendsOnlyChanges(t *testing.T) {
	var calls []string
	p := &Pane{ID: "%7", Run: func(args ...string) error { calls = append(calls, strings.Join(args, " ")); return nil }}
	run := func(r Rect) bool {
		cmd := p.Report(r)
		if cmd != nil {
			cmd()
		}
		return cmd != nil
	}
	if !run(Rect{0, 2, 80, 10, false}) || run(Rect{0, 2, 80, 10, false}) {
		t.Fatal("first report must send, an unchanged one must not")
	}
	if !run(Rect{0, 2, 80, 12, false}) || !run(Rect{}) || run(Rect{}) {
		t.Fatal("a change and an unset must send once each")
	}
	want := []string{
		"set-option -p -t %7 @tw-focus 0,2,80,10",
		"set-option -p -t %7 @tw-focus 0,2,80,12",
		"set-option -pu -t %7 @tw-focus",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	var none *Pane
	if none.Report(Rect{0, 0, 1, 1, false}) != nil || (&Pane{}).Report(Rect{0, 0, 1, 1, false}) != nil {
		t.Error("no pane must report nothing")
	}
}
