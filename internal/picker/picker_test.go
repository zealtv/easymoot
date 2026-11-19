package picker

import (
	"errors"
	"testing"
)

func three() []Entry {
	return []Entry{
		{Folder: "/h/bob", Name: "bob", Rooms: 2},
		{Folder: "/h/bob/repos/a", Name: "a", Rooms: 0},
		{Folder: "/h/bob/repos/b", Name: "b", Rooms: -1},
	}
}

func TestEmptyPickerHasNoSelection(t *testing.T) {
	m := New("/h/bob")
	if !m.Empty() || m.Selected() != nil || len(m.Rows()) != 0 {
		t.Fatalf("an empty picker offers something: %+v", m)
	}
	if m.Move(1).Cursor() != 0 || m.Move(-1).Cursor() != 0 {
		t.Fatal("the cursor left 0 in an empty list")
	}
}

func TestCursorIsClampedNotWrapped(t *testing.T) {
	m := New("/h/bob").Done(three(), nil)
	if m.Cursor() != 0 || m.Selected().Name != "bob" {
		t.Fatalf("starts on %d", m.Cursor())
	}
	if got := m.Move(-1).Cursor(); got != 0 {
		t.Errorf("up from the top went to %d", got)
	}
	if got := m.Move(1).Move(1).Move(1).Move(1).Cursor(); got != 2 {
		t.Errorf("down past the end went to %d", got)
	}
	if got := m.MoveTo(99).Selected().Name; got != "b" {
		t.Errorf("MoveTo(99) selected %q", got)
	}
	if got := m.MoveTo(-5).Cursor(); got != 0 {
		t.Errorf("MoveTo(-5) = %d", got)
	}
}

func TestRowsShortenPathsAndMarkTheCursor(t *testing.T) {
	rows := New("/h/bob").Done(three(), nil).Move(1).Rows()
	want := []Row{
		{Name: "bob", Path: "~", Rooms: 2},
		{Selected: true, Name: "a", Path: "~/repos/a", Rooms: 0},
		{Name: "b", Path: "~/repos/b", Rooms: -1},
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestScanLifecycle(t *testing.T) {
	m := New("/h/bob").Done(three(), nil).Move(2).Start()
	if !m.Scanning() || !m.Empty() || m.Cursor() != 0 {
		t.Fatalf("Start kept the old list: %+v", m)
	}
	m = m.Done(three(), nil)
	if m.Scanning() || m.Cursor() != 0 || len(m.Entries()) != 3 {
		t.Fatalf("Done: %+v", m)
	}
	m = m.Start().Done(three(), errors.New("scan failed"))
	if !m.Empty() || m.Notice() != "scan failed" || m.Scanning() {
		t.Fatalf("a failed scan: %+v", m)
	}
	if m.Notify("x").Notice() != "x" {
		t.Fatal("Notify")
	}
}
