package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// A fake home under t.TempDir(): ~/repos/<name>/.moot for each name given, each with
// the rooms listed after the colon in the map. Nothing here scans or reads the real
// home or a real moot.
func homeWith(t *testing.T, moots map[string][]string) string {
	t.Helper()
	t.Setenv("MOOT_NAME", "")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "ada")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	for folder, rooms := range moots {
		dir := filepath.Join(home, folder, ".moot", "rooms")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, r := range rooms {
			if err := os.MkdirAll(filepath.Join(dir, r, "posts"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return home
}

func writePost(t *testing.T, home, folder, room, id, text string) {
	t.Helper()
	p := filepath.Join(home, folder, ".moot", "rooms", room, "posts", id+".md")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func threeMoots(t *testing.T) string {
	home := homeWith(t, map[string][]string{
		"repos/alpha": {"dev", "runs"},
		"repos/beta":  {"lobby"},
		"repos/gamma": nil,
	})
	writePost(t, home, "repos/beta", "lobby", "20260801-090000-ada", "hello from beta\n")
	return home
}

// runCmd runs a command that reads or scans and feeds its message back.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	next, cmd := m.Update(cmd())
	return next.(Model), cmd
}

// atStartup is NewPicker at 100x30 with its scan answered.
func atStartup(t *testing.T, home string, opts ...Option) Model {
	t.Helper()
	m := NewPicker(append([]Option{utc, WithHome(home), WithPollInterval(time.Hour)}, opts...)...)
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = runCmd(t, m, m.Init())
	return m
}

// overShell is the shell on the synthetic moot in f with Switch folder run and the
// scan answered.
func overShell(t *testing.T, src Source, home string) Model {
	t.Helper()
	m := shown(t, src, WithHome(home))
	m, cmd := feed(m, typeText(":switch")...)
	m, cmd = feed(m, key(tea.KeyEnter))
	if m.pick == nil || cmd == nil {
		t.Fatal("Switch folder did not open the picker with a scan")
	}
	m, _ = runCmd(t, m, cmd)
	return m
}

func names(m Model) []string {
	var out []string
	for _, r := range m.pick.list.Rows() {
		out = append(out, r.Name)
	}
	return out
}

func TestStartupPickerScansThenListsFolders(t *testing.T) {
	home := threeMoots(t)
	m := NewPicker(utc, WithHome(home))
	if m.pick == nil || m.src != nil {
		t.Fatal("NewPicker should show the picker with no moot open")
	}
	if !m.pick.list.Scanning() || !strings.Contains(m.View(), "scanning") {
		t.Fatalf("before the scan answers the picker says so:\n%s", m.View())
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should start the scan")
	}
	m, _ = runCmd(t, m, cmd)
	if got := names(m); strings.Join(got, ",") != "alpha,beta,gamma" {
		t.Fatalf("folders = %v", got)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Choose a folder", "3 folders", "alpha", "~/repos/alpha", "2 rooms", "1 room", "no rooms", "ctrl+c"} {
		if !strings.Contains(v, want) {
			t.Errorf("frame lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "esc") {
		t.Errorf("at start-up there is no shell to go back to, so no esc hint:\n%s", v)
	}
}

// Nothing outside the palette quits but ctrl+c, and at start-up there is no palette:
// no key but ctrl+c quits, opens the palette or does anything but move the cursor
// and open the folder under it.
func TestStartupPickerKeysNeverQuitOrAct(t *testing.T) {
	home := threeMoots(t)
	for _, over := range []bool{false, true} {
		var base Model
		if over {
			base = overShell(t, newSpy(synthetic(t)), home)
		} else {
			base = atStartup(t, home)
		}
		var keys []tea.KeyMsg
		for kt := tea.KeyType(-300); kt <= 127; kt++ {
			keys = append(keys, tea.KeyMsg{Type: kt})
		}
		for r := rune(0x20); r < 0x7f; r++ {
			keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true})
		}
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("quit"), Paste: true}, tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
		for _, k := range keys {
			if k.Type == tea.KeyCtrlC && !k.Alt {
				continue
			}
			next, cmd := base.Update(k)
			nm := next.(Model)
			if nm.palette != nil {
				t.Fatalf("over=%v key %q opened the palette", over, k.String())
			}
			if cmd != nil && quits(cmd) {
				t.Fatalf("over=%v key %q (type %d) quit", over, k.String(), k.Type)
			}
			if cmd != nil {
				// Only Enter schedules anything, and only the open of a folder.
				if k.String() != "enter" || k.Paste {
					t.Fatalf("over=%v key %q scheduled a command", over, k.String())
				}
				if msg, ok := cmd().(openedMsg); !ok || msg.err != nil {
					t.Fatalf("enter produced %#v", msg)
				}
			}
			if !over && nm.pick == nil {
				t.Fatalf("key %q closed the start-up picker", k.String())
			}
		}
		if _, cmd := base.Update(key(tea.KeyCtrlC)); !quits(cmd) {
			t.Fatalf("over=%v: ctrl+c did not quit", over)
		}
	}
}

func TestEscAtStartupDoesNothing(t *testing.T) {
	m := atStartup(t, threeMoots(t))
	next, cmd := m.Update(key(tea.KeyEsc))
	if cmd != nil || next.(Model).pick == nil || next.(Model).View() != m.View() {
		t.Fatal("Esc at start-up did something")
	}
}

func TestCursorKeysMoveAndEnterOpensTheFolder(t *testing.T) {
	home := threeMoots(t)
	m := atStartup(t, home)
	m, _ = feed(m, key(tea.KeyDown))
	if sel := m.pick.list.Selected(); sel == nil || sel.Name != "beta" {
		t.Fatalf("down selected %+v", sel)
	}
	if !strings.Contains(m.View(), "❯ beta") {
		t.Errorf("the cursor row shows no marker:\n%s", m.View())
	}
	m, _ = feed(m, key(tea.KeyEnd))
	if m.pick.list.Selected().Name != "gamma" {
		t.Fatal("end did not go to the last folder")
	}
	m, _ = feed(m, key(tea.KeyHome), key(tea.KeyDown))
	next, cmd := m.Update(key(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter on a folder did not open it")
	}
	m, cmd = runCmd(t, next.(Model), cmd)
	if m.pick != nil || m.src == nil || m.src.Folder() != filepath.Join(home, "repos/beta") {
		t.Fatalf("not opened on beta: pick=%v src=%v", m.pick != nil, m.src)
	}
	if cmd == nil {
		t.Fatal("opening a folder should start its first read")
	}
	m, _ = runCmd(t, m, cmd)
	v := ansi.Strip(m.View())
	for _, want := range []string{"~/repos/beta", "lobby", "hello from beta", "1 room"} {
		if !strings.Contains(strings.ReplaceAll(v, home, "~"), want) {
			t.Errorf("the shell on beta lacks %q:\n%s", want, v)
		}
	}
}

// Enter with nothing to open does nothing.
func TestEnterOnAnEmptyListDoesNothing(t *testing.T) {
	m := atStartup(t, homeWith(t, nil))
	next, cmd := m.Update(key(tea.KeyEnter))
	if cmd != nil || next.(Model).pick == nil {
		t.Fatal("enter on an empty list acted")
	}
}

func TestEmptyAndFailedScansSaySoAndOfferNoWayToOpen(t *testing.T) {
	home := homeWith(t, nil)
	if err := os.MkdirAll(filepath.Join(home, "repos/plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := atStartup(t, home)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "No folder under") || !strings.Contains(v, "0 folders") || !strings.Contains(v, "easymoot <path>") {
		t.Errorf("an empty scan:\n%s", v)
	}
	gone := filepath.Join(home, "does-not-exist")
	m = atStartup(t, gone)
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "the scan failed") || !strings.Contains(v, "does-not-exist") {
		t.Errorf("a failed scan:\n%s", v)
	}
	// No home at all.
	m = NewPicker(utc, WithHome(""))
	m, _ = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = runCmd(t, m, m.Init())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "no home directory") {
		t.Errorf("no home:\n%s", v)
	}
}

func TestSwitchFolderOpensThePickerOverTheShellAndScansAgain(t *testing.T) {
	home := threeMoots(t)
	spy := newSpy(synthetic(t))
	m := shown(t, spy, WithHome(home))
	spy.take()
	before := m.View()

	m, _ = feed(m, typeText(":")...)
	m, _ = feed(m, typeText("switch f")...)
	if m.palette == nil || m.palette.chosen != actionSwitch {
		t.Fatalf("typing toward Switch folder chose %#v", m.palette)
	}
	// Words find it too, as for any non-strict command; Enter runs it.
	m, cmd := feed(m, key(tea.KeyEnter))
	if m.pick == nil || m.palette != nil || cmd == nil {
		t.Fatal("Switch folder did not open the picker")
	}
	if calls := spy.take(); len(calls) != 0 {
		t.Fatalf("opening the picker read the moot: %v (the scan is a command)", calls)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "scanning") || !strings.Contains(v, "Choose a folder") {
		t.Errorf("while scanning:\n%s", v)
	}
	// The shell is still drawn behind the box: its header above, its sidebar beside.
	if v := ansi.Strip(m.View()); !strings.Contains(v, "atlas") || !strings.Contains(v, "lobby") {
		t.Errorf("the shell is not under the picker:\n%s", v)
	}
	m, _ = runCmd(t, m, cmd)
	if got := names(m); strings.Join(got, ",") != "alpha,beta,gamma" {
		t.Fatalf("folders = %v", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "esc") {
		t.Error("over a shell the hints offer esc")
	}

	// Scanning again: a second Switch folder after Esc scans afresh.
	next, _ := m.Update(key(tea.KeyEsc))
	m = next.(Model)
	if m.pick != nil || m.View() != before {
		t.Fatalf("Esc did not return the shell unchanged:\n%s\n---\n%s", before, m.View())
	}
	m, _ = feed(m, typeText(":switch")...)
	m, cmd = feed(m, key(tea.KeyEnter))
	if m.pick == nil || !m.pick.list.Scanning() || cmd == nil {
		t.Fatal("the second Switch folder did not scan again")
	}
}

// The shell keeps polling under the picker, so Esc returns to what is current.
func TestTheShellKeepsPollingUnderThePicker(t *testing.T) {
	home := threeMoots(t)
	f := synthetic(t)
	src := f.open()
	m := overShell(t, src, home)
	f.post("dev", "20260803-090000-ada", "arrived while the picker was open\n")
	// The tick and the read it starts run as usual.
	m, cmd := feed(m, tickMsg{src: src})
	m, _ = runCmd(t, m, cmd)
	m, _ = feed(m, key(tea.KeyEsc))
	if !strings.Contains(ansi.Strip(m.View()), "arrived while the picker was open") {
		t.Fatalf("the shell missed a post that arrived under the picker:\n%s", m.View())
	}
}

func TestChoosingAnotherFolderReplacesTheShell(t *testing.T) {
	home := threeMoots(t)
	f := synthetic(t)
	old := f.open()
	m := overShell(t, old, home)
	m, _ = feed(m, key(tea.KeyDown)) // beta
	m, cmd := feed(m, key(tea.KeyEnter))
	m, cmd = runCmd(t, m, cmd)
	if m.pick != nil || m.src.Folder() != filepath.Join(home, "repos/beta") {
		t.Fatal("did not switch to beta")
	}
	// Nothing of the old moot stays on the screen or in the model.
	if m.room != "" || len(m.posts) != 0 || len(m.rooms) != 0 || m.focus != paneRooms || !m.follow {
		t.Fatalf("old state remains: %+v", m)
	}
	if strings.Contains(ansi.Strip(m.View()), "rebuild") {
		t.Error("the old moot's posts are still drawn")
	}
	// The old poll chain ends: its tick and its answer are dropped.
	if _, cmd := m.Update(tickMsg{src: old}); cmd != nil {
		t.Error("a tick for the old moot started a read")
	}
	stale := pollCmd(old, "", nil, true)()
	if next, cmd := m.Update(stale); cmd != nil || next.(Model).src != m.src || len(next.(Model).rooms) != 0 {
		t.Error("an old answer was applied or chained")
	}
	// The new one is read, and its own poll chain runs.
	m, cmd = runCmd(t, m, pollCmd(m.src, "", nil, true))
	if cmd == nil {
		t.Fatal("the new moot's poll does not chain")
	}
	if got := ansi.Strip(m.View()); !strings.Contains(got, "hello from beta") {
		t.Fatalf("the new moot's room is not shown:\n%s", got)
	}
}

func TestALateScanOrOpenForAnEarlierPickerIsDropped(t *testing.T) {
	home := threeMoots(t)
	m := shown(t, newSpy(synthetic(t)), WithHome(home))
	m, _ = feed(m, typeText(":switch")...)
	m, first := feed(m, key(tea.KeyEnter))
	early := first() // the first scan's answer, held back
	m, _ = feed(m, key(tea.KeyEsc))
	m, _ = feed(m, typeText(":switch")...)
	m, _ = feed(m, key(tea.KeyEnter))
	if next, _ := m.Update(early); !next.(Model).pick.list.Scanning() {
		t.Fatal("the first scan's answer landed on the second picker")
	}
	// An answer with the picker closed is dropped too.
	closed, _ := feed(m, key(tea.KeyEsc))
	if next, _ := closed.Update(early); next.(Model).pick != nil {
		t.Fatal("a scan answer reopened the picker")
	}
	stale := openedMsg{gen: m.pick.gen - 1, folder: "x", src: newSpy(synthetic(t))}
	if next, _ := m.Update(stale); next.(Model).src != m.src {
		t.Fatal("a late open switched the shell")
	}
}

func TestAFolderThatLostItsMootStaysInThePickerWithANotice(t *testing.T) {
	home := threeMoots(t)
	m := atStartup(t, home)
	if err := os.RemoveAll(filepath.Join(home, "repos/alpha/.moot")); err != nil {
		t.Fatal(err)
	}
	m, cmd := feed(m, key(tea.KeyEnter)) // alpha
	m, _ = runCmd(t, m, cmd)
	if m.pick == nil || m.src != nil {
		t.Fatal("the picker closed on a folder with no .moot")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "could not open") || !strings.Contains(v, "alpha") {
		t.Errorf("no notice:\n%s", v)
	}
}

func TestMouseDoesNothingWhileThePickerIsOpen(t *testing.T) {
	m := overShell(t, newSpy(synthetic(t)), threeMoots(t))
	room, view := m.room, m.View()
	for _, b := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonWheelDown, tea.MouseButtonWheelUp} {
		for x := 0; x < 100; x += 7 {
			for y := 0; y < 30; y += 3 {
				next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
				if cmd != nil || next.(Model).room != room || next.(Model).View() != view {
					t.Fatalf("mouse %v at %d,%d acted", b, x, y)
				}
			}
		}
	}
}

func TestPickerFitsEverySize(t *testing.T) {
	home := threeMoots(t)
	bases := map[string]Model{
		"startup": atStartup(t, home),
		"over":    overShell(t, newSpy(synthetic(t)), home),
		"empty":   atStartup(t, homeWith(t, nil)),
	}
	for name, base := range bases {
		for w := 1; w <= 130; w += 3 {
			for h := 1; h <= 40; h += 3 {
				next, _ := base.Update(tea.WindowSizeMsg{Width: w, Height: h})
				rows := strings.Split(next.(Model).View(), "\n")
				if len(rows) > h {
					t.Fatalf("%s %dx%d: %d lines", name, w, h, len(rows))
				}
				for i, line := range rows {
					if lipgloss.Width(line) > w {
						t.Fatalf("%s %dx%d line %d is %d wide: %q", name, w, h, i, lipgloss.Width(line), ansi.Strip(line))
					}
				}
			}
		}
	}
}

// A long list scrolls with the cursor: the selected folder is always on screen.
func TestALongListKeepsTheCursorInView(t *testing.T) {
	moots := map[string][]string{}
	for i := 0; i < 40; i++ {
		moots[fmt.Sprintf("repos/proj%02d", i)] = []string{"dev"}
	}
	m := atStartup(t, homeWith(t, moots))
	for i := 0; i < 40; i++ {
		want := fmt.Sprintf("❯ proj%02d", i)
		if !strings.Contains(ansi.Strip(m.View()), want) {
			t.Fatalf("cursor %d is off screen:\n%s", i, m.View())
		}
		m, _ = feed(m, key(tea.KeyDown))
	}
	m, _ = feed(m, key(tea.KeyHome))
	if !strings.Contains(ansi.Strip(m.View()), "❯ proj00") {
		t.Fatal("home did not scroll back")
	}
	m, _ = feed(m, key(tea.KeyPgDown))
	if got := m.pick.list.Cursor(); got != m.pickRows() {
		t.Fatalf("page down moved to %d, want %d", got, m.pickRows())
	}
}

// Folder names come from the file system, so they are inert like everything else.
func TestPickerTextIsInert(t *testing.T) {
	home := homeWith(t, map[string][]string{"repos/ev\x1b[2Jil\x07": {"dev"}})
	m := atStartup(t, home)
	v := ansi.Strip(m.View())
	if strings.ContainsAny(strings.ReplaceAll(v, "\n", ""), "\x1b\x07\x00\r") {
		t.Fatalf("an escape or control reached the frame: %q", v)
	}
	if !strings.Contains(v, "ev") {
		t.Fatal("the folder is not listed")
	}
	// The same for the notice of a folder that failed to open.
	m, _ = feed(m, openedMsg{gen: m.pick.gen, folder: "/x\x1b[2J", err: fmt.Errorf("bad \x1b[31m")}, key(tea.KeyDown))
	if v := m.View(); strings.ContainsAny(strings.ReplaceAll(ansi.Strip(v), "\n", ""), "\x1b\x07\x00") {
		t.Fatalf("notice not cleaned: %q", v)
	}
}
