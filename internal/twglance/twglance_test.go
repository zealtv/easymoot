package twglance

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func recorder() (*Pane, *[]string) {
	var calls []string
	p := &Pane{ID: "%7", Run: func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}}
	return p, &calls
}

func syncNow(p *Pane, g Glance) bool {
	cmd := p.Sync(g)
	if cmd != nil {
		cmd()
	}
	return cmd != nil
}

// The example in the map's docs/interface.md encodes as written there.
func TestEncodesTheInterfaceExample(t *testing.T) {
	far := FarValue([]Entry{{Label, "loom"}, {Busy, "◐ 2"}, {OK, "● 1"}, {Hold, "‖ 1"}})
	if want := ":loom;busy:◐ 2;ok:● 1;hold:‖ 1"; far != want {
		t.Errorf("far = %q, want %q", far, want)
	}
	mid := MidValue([]Entry{{Busy, "◐ tour-polish"}, {OK, "● selection-set"}, {Hold, "‖ stall-leftovers"}})
	if want := "busy:◐ tour-polish;ok:● selection-set;hold:‖ stall-leftovers"; mid != want {
		t.Errorf("mid = %q, want %q", mid, want)
	}
}

// The text is cleaned here, not left to the reader: ";" would end the entry,
// a control character or a newline would break the line, and a colon in the
// text stays text because the role's colon is always written.
func TestCleansText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a;b", "a,b"},
		{"a\nb\tc", "a b c"},
		{"  lots   of\t\tspace  ", "lots of space"},
		{"\x1b[31mred\x1b[0m text", "red text"},
		{"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"a\u2028b\u3000c\u2003d", "a b c d"},
		{"x\u200by\u202ez\u2066w", "xyzw"},
		{"10:52 standup", "10:52 standup"},
		{"a|b", "a|b"},
		{"\u0085x y", "x y"},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := MidValue([]Entry{{Label, "10:52 standup"}}); got != ":10:52 standup" {
		t.Errorf("a colon in the text lost its role's colon: %q", got)
	}
	if got := MidValue([]Entry{{"b:u;sy", "x"}}); got != "bu,sy:x" {
		t.Errorf("a role kept a colon: %q", got)
	}
}

// Entries are cut to the cells the map keeps, ending "…", and the badge line
// keeps its first eight. An entry with no text is left out and does not count.
func TestCaps(t *testing.T) {
	long := strings.Repeat("x", 100)
	far := FarValue([]Entry{{OK, long}})
	if text := strings.TrimPrefix(far, "ok:"); ansi.StringWidth(text) != BadgeCells || !strings.HasSuffix(text, "…") {
		t.Errorf("a badge was not cut to %d cells: %q", BadgeCells, far)
	}
	mid := MidValue([]Entry{{OK, "● " + long}})
	if text := strings.TrimPrefix(mid, "ok:"); ansi.StringWidth(text) != RowCells || !strings.HasSuffix(text, "…") {
		t.Errorf("a row was not cut to %d cells: %q", RowCells, mid)
	}
	wide := MidValue([]Entry{{OK, strings.Repeat("界", 40)}})
	if w := ansi.StringWidth(strings.TrimPrefix(wide, "ok:")); w > RowCells {
		t.Errorf("a wide row is %d cells", w)
	}

	var badges []Entry
	badges = append(badges, Entry{OK, "  "})
	for i := range 10 {
		badges = append(badges, Entry{OK, fmt.Sprint(i)})
	}
	if got, want := FarValue(badges), "ok:0;ok:1;ok:2;ok:3;ok:4;ok:5;ok:6;ok:7"; got != want {
		t.Errorf("badges = %q, want %q", got, want)
	}
}

// The list is not cut to sixteen rows: the map counts the rows it cannot show
// ("+N"), so it is sent every row that fits the 8 KiB the map reads.
func TestTheListKeepsEveryRowThatFits(t *testing.T) {
	var rows []Entry
	for i := range 40 {
		rows = append(rows, Entry{OK, fmt.Sprintf("● stitch-%d", i)})
	}
	if got := strings.Count(MidValue(rows), ";") + 1; got != 40 {
		t.Errorf("%d rows sent, want 40", got)
	}
	rows = rows[:0]
	for range 1000 {
		rows = append(rows, Entry{OK, strings.Repeat("y", 60)})
	}
	v := MidValue(rows)
	if len(v) > MaxBytes {
		t.Errorf("the list is %d bytes, over %d", len(v), MaxBytes)
	}
	for _, e := range strings.Split(v, ";") {
		if e != "ok:"+strings.Repeat("y", 60) {
			t.Fatalf("a row was cut short to fit: %q", e)
		}
	}
}

// Empty means unset, on the first write too, so a glance an earlier run left
// behind does not linger; after that only changes are sent, both in one call.
func TestSyncSendsEverythingOnceThenOnlyChanges(t *testing.T) {
	p, calls := recorder()
	g := Glance{Badges: []Entry{{Label, "loom"}, {OK, "●1"}}}
	if !syncNow(p, g) {
		t.Fatal("the first Sync sent nothing")
	}
	if syncNow(p, g) {
		t.Fatal("an unchanged Sync sent a call")
	}
	g.Rows = []Entry{{OK, "● a"}}
	if !syncNow(p, g) {
		t.Fatal("a new list did not send")
	}
	if !syncNow(p, Glance{}) || syncNow(p, Glance{}) {
		t.Fatal("clearing must send once")
	}
	want := []string{
		"set-option -p -t %7 @tw-glance-far :loom;ok:●1 ; set-option -pu -t %7 @tw-glance-mid",
		"set-option -p -t %7 @tw-glance-mid ok:● a",
		"set-option -pu -t %7 @tw-glance-far ; set-option -pu -t %7 @tw-glance-mid",
	}
	if strings.Join(*calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(*calls, "\n"), strings.Join(want, "\n"))
	}
}

// Entries with no text at all make an empty value, which unsets.
func TestOnlyEmptyEntriesUnset(t *testing.T) {
	p, calls := recorder()
	syncNow(p, Glance{Badges: []Entry{{OK, " "}}, Rows: []Entry{{OK, "\n"}}})
	if want := "set-option -pu -t %7 @tw-glance-far ; set-option -pu -t %7 @tw-glance-mid"; (*calls)[0] != want {
		t.Errorf("call = %q, want %q", (*calls)[0], want)
	}
}

// Outside tmux there is no pane, and nothing is written.
func TestNoPaneWritesNothing(t *testing.T) {
	var p Pane
	if p.Sync(Glance{Badges: []Entry{{OK, "x"}}}) != nil {
		t.Error("a Pane with no ID made a command")
	}
	if p.Clear() != nil {
		t.Error("Clear with no ID failed")
	}
	var nilPane *Pane
	if nilPane.Sync(Glance{}) != nil || nilPane.Clear() != nil {
		t.Error("a nil Pane did something")
	}
}

// A failed write is tried again by the next Sync, even with the glance
// unchanged: otherwise a stale glance from an earlier run would stay on the
// map until the program's own glance changed.
func TestAFailedWriteIsRetriedUnchanged(t *testing.T) {
	fail := true
	var calls []string
	p := &Pane{ID: "%7", Run: func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if fail {
			return errors.New("no server")
		}
		return nil
	}}
	g := Glance{Badges: []Entry{{OK, "x"}}}
	syncNow(p, g)
	fail = false
	if !syncNow(p, g) {
		t.Fatal("an unchanged Sync after a failed write sent nothing")
	}
	if syncNow(p, g) {
		t.Fatal("a Sync after a good write sent again")
	}
	if len(calls) != 2 || calls[0] != calls[1] {
		t.Errorf("calls = %q", calls)
	}
}

// A failed write is tried again by the next Sync, rather than taken as done.
func TestAFailedWriteIsRetried(t *testing.T) {
	fail := true
	var calls int
	p := &Pane{ID: "%7", Run: func(args ...string) error {
		calls++
		if fail {
			return errors.New("no server")
		}
		return nil
	}}
	g := Glance{Badges: []Entry{{OK, "x"}}}
	syncNow(p, g)
	fail = false
	g.Rows = []Entry{{OK, "y"}}
	syncNow(p, g)
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

// Clear unsets both options and stops every write still to come, from any
// copy of the Pane: Bubble Tea may run a command after the program has gone.
func TestClearStopsLaterWrites(t *testing.T) {
	p, calls := recorder()
	shared := New("%7")
	shared.Run = p.Run
	copyOf := shared
	cmd := copyOf.Sync(Glance{Badges: []Entry{{OK, "x"}}})
	if err := shared.Clear(); err != nil {
		t.Fatal(err)
	}
	cmd()
	if len(*calls) != 1 || (*calls)[0] != "set-option -pu -t %7 @tw-glance-far ; set-option -pu -t %7 @tw-glance-mid" {
		t.Errorf("calls = %q", *calls)
	}
}

// Every call names the pane: a glance on the window would draw for every
// pane in it.
func TestEveryCallIsPaneScoped(t *testing.T) {
	p, calls := recorder()
	syncNow(p, Glance{Badges: []Entry{{OK, "a"}}, Rows: []Entry{{OK, "b"}}})
	p.Clear()
	for _, c := range *calls {
		for _, part := range strings.Split(c, " ; ") {
			if !strings.HasPrefix(part, "set-option -p -t %7 ") && !strings.HasPrefix(part, "set-option -pu -t %7 ") {
				t.Errorf("not pane-scoped: %q", part)
			}
		}
	}
}
