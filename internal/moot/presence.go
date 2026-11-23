package moot

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Watch is what a participant's .moot/watching and .moot/watch.paused say.
type Watch uint8

const (
	NotWatching Watch = iota // no watching file, or its pid is gone
	Watching                 // watching holds a live pid
	WatchPaused              // the same, and watch.paused exists
)

// Presence is what one participant's own files say about it, read-only.
type Presence struct {
	// Name is the name chain without $MOOT_NAME (OwnName), or the folder's own name
	// when the files give none usable.
	Name  string
	Watch Watch
	// Status is the text of line one of .moot/status, "" when there is none. Stamp is
	// that line's leading UTC stamp, zero when it has none: then the line is all text.
	Status string
	Stamp  time.Time
}

// ReadPresence reads the presence of the participant whose folder holds .moot/.
// Freshness comes from the status's stamp, never from a file's mtime, which git does
// not keep (moot's README, "Presence").
func ReadPresence(folder string) Presence {
	m := &Moot{folder: folder, dir: filepath.Join(folder, ".moot")}
	p := Presence{Name: filepath.Base(folder)}
	if name, err := m.OwnName(); err == nil {
		p.Name = name
	}
	if livePid(filepath.Join(m.dir, "watching")) {
		p.Watch = Watching
		if _, err := os.Lstat(filepath.Join(m.dir, "watch.paused")); err == nil {
			p.Watch = WatchPaused
		}
	}
	if data, err := os.ReadFile(filepath.Join(m.dir, "status")); err == nil {
		p.Status, p.Stamp = statusLine(strings.TrimRight(firstLine(data), " \t\r"))
	}
	return p
}

// statusLine splits line one of a status into its text and its stamp, the post-id
// grammar's <8 digits>-<6 digits> followed by a space. A line that does not open with
// one is all text.
func statusLine(line string) (string, time.Time) {
	if len(line) > 16 && line[15] == ' ' && isStampID(line[:15]+"-x") {
		if t, err := time.Parse("20060102-150405", line[:15]); err == nil {
			return strings.TrimSpace(line[16:]), t
		}
	}
	return strings.TrimSpace(line), time.Time{}
}

// livePid says the watching file's pid= line names a process that is alive: signal 0
// reaches it (or it belongs to someone else, which still means it runs).
func livePid(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "pid="); ok {
			pid, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || pid <= 0 {
				return false
			}
			err = syscall.Kill(pid, 0)
			return err == nil || err == syscall.EPERM
		}
	}
	return false
}
