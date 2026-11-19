// Package moot reads one folder's .moot/ and posts to it. It is headless: no
// terminal code, no goroutines of its own.
//
// Reading copies moot.sh's own rules rule for rule (subscribed_rooms, post_ids,
// post_body_path, is_stamp_id, render_post in the moot repository at c25b5bb) and
// never runs the script. The name a post will go out under is resolved read-only
// from the same chain as moot.sh's resolve_name. Post is the one write: it runs the
// folder's own moot.sh, and nothing else here writes anywhere. In particular no code
// path touches .moot/seen/: Easymoot never moves anyone's cursor.
//
// The tests in this package pin a copy of upstream moot.sh under testdata/ and
// compare against it.
package moot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNoMoot reports a folder with no .moot/ directory.
var ErrNoMoot = errors.New("no .moot directory")

// Moot is one folder's .moot/. It is safe for concurrent use.
type Moot struct {
	folder string // absolute, as typed, symlinks kept
	dir    string // folder + "/.moot"

	mu    sync.Mutex
	cache map[string]cachedBody
}

// Open opens the .moot/ of the folder at path. path may also name the .moot
// directory itself. It is made absolute but not resolved: moot.sh names the sender
// after the folder it is invoked through, so a symlinked folder keeps its own name.
func Open(path string) (*Moot, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if filepath.Base(abs) == ".moot" {
		abs = filepath.Dir(abs)
	}
	dir := filepath.Join(abs, ".moot")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w in %s", ErrNoMoot, abs)
	}
	return &Moot{folder: abs, dir: dir, cache: map[string]cachedBody{}}, nil
}

// Folder is the absolute folder path as given, symlinks kept.
func (m *Moot) Folder() string { return m.folder }

// Dir is Folder()/.moot.
func (m *Moot) Dir() string { return m.dir }

// Room is a room listed under rooms/.
type Room struct {
	Name string
	// Link is where the room's symlink leads, or "" for a plain directory.
	Link string
}

// Rooms lists the rooms in byte order: directories under rooms/ (symlinks followed),
// not hidden. A link that leads nowhere is not a room. No rooms/ means no rooms.
func (m *Moot) Rooms() ([]Room, error) {
	root := filepath.Join(m.dir, "rooms")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rooms []Room
	for _, e := range entries { // ReadDir sorts by filename, bytewise.
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(root, name)
		if info, err := os.Stat(full); err != nil || !info.IsDir() {
			continue
		}
		room := Room{Name: name}
		if e.Type()&os.ModeSymlink != 0 {
			room.Link, _ = os.Readlink(full)
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

// Post is one entry of a room's posts/, without its body (see Body).
type Post struct {
	ID string
	// Dir marks a post directory (<id>/post.md) rather than a file (<id>.md).
	Dir bool
	// Stamp marks an id shaped <8 digits>-<6 digits>-<sender>. Only a stamp has a
	// Sender and a time; any other id (a stray README.md) shows under its own name.
	Stamp  bool
	Sender string
	// Time is the stamp's UTC time. It stays zero when the id is no stamp, or the
	// stamp is no real date: then WrittenTime gives the digits as written.
	Time time.Time
}

// WrittenTime returns a stamp's date and time as written, "YYYY-MM-DD HH:MM:SS",
// which moot.sh shows with a UTC label when the stamp is no real date. It returns
// "" for an id that is no stamp.
func (p Post) WrittenTime() string {
	if !p.Stamp {
		return ""
	}
	d, c := p.ID[:8], p.ID[9:15]
	return d[:4] + "-" + d[4:6] + "-" + d[6:] + " " + c[:2] + ":" + c[2:4] + ":" + c[4:]
}

// List returns a room's posts, oldest first in bytewise id order, as moot.sh's
// post_ids does: a visible directory is a post under its name, a visible *.md file
// is a post under its name without .md, anything else is not a post, ids are unique,
// and a directory wins over a file of the same id. A room with no posts/ has none.
func (m *Moot) List(room string) ([]Post, error) {
	if !plainName(room) {
		return nil, fmt.Errorf("invalid room name %q", room)
	}
	dir := m.postsDir(room)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	isDir := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name)) // follows links; dangling ones are skipped
		if err != nil {
			continue
		}
		if info.IsDir() {
			isDir[name] = true
		} else if id, ok := strings.CutSuffix(name, ".md"); ok {
			if _, seen := isDir[id]; !seen {
				isDir[id] = false
			}
		}
	}
	ids := make([]string, 0, len(isDir))
	for id := range isDir {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	posts := make([]Post, len(ids))
	for i, id := range ids {
		posts[i] = classify(id, isDir[id])
	}
	return posts, nil
}

func (m *Moot) postsDir(room string) string {
	return filepath.Join(m.dir, "rooms", room, "posts")
}

// classify applies is_stamp_id and render_post's split of an id.
func classify(id string, dir bool) Post {
	p := Post{ID: id, Dir: dir}
	if !isStampID(id) {
		return p
	}
	p.Stamp = true
	p.Sender = id[16:]
	if t, err := time.Parse("20060102150405", id[:8]+id[9:15]); err == nil {
		p.Time = t
	}
	return p
}

// isStampID is moot.sh's is_stamp_id: 8 digits, a dash, 6 digits, a dash, then at
// least one character.
func isStampID(id string) bool {
	if len(id) < 17 || id[8] != '-' || id[15] != '-' {
		return false
	}
	for i := 0; i < 15; i++ {
		if i != 8 && (id[i] < '0' || id[i] > '9') {
			return false
		}
	}
	return true
}

// plainName rejects what could not be one path segment.
func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\x00")
}
