// Package picker finds the folders that hold a .moot/ directory and lists them to
// choose from. It is plain data and a walk: no terminal code and no Bubble Tea
// import, so the shell draws the list and a test can drive it.
//
// It is cut down from EasyLoom's internal/picker and its walk
// (internal/target/scan.go) at commit ca2f8f6, read-only: the walk, the depth bound,
// the pruned names, the clamped cursor and the `~` shortening are EasyLoom's. Left
// out are the remembered list (internal/known), the tiers, the probe, the forget key
// and the probe's worker pool (architecture, "Choosing a folder", Decided 1).
package picker

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zealtv/easymoot/internal/moot"
)

// DefaultDepth bounds how far below the scan root a folder is looked for. A moot
// sits at a repository root, not buried (EasyLoom's bound for a loom).
const DefaultDepth = 4

// prunedDirs are never descended into, wherever they sit (EasyLoom's set).
var prunedDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
}

// rootPruned are skipped only directly under the scan root, which is the home
// directory: ~/Library on macOS is large, and holds folders that ask the user's
// permission before a program may list them. Pruned on every system rather than
// only on macOS, so one rule serves both and one test covers it.
var rootPruned = map[string]bool{"Library": true}

// mootDir is the directory whose presence makes a folder an entry.
const mootDir = ".moot"

// Entry is one folder that holds a .moot/.
type Entry struct {
	// Folder is the absolute path of the folder, as walked: symlinks are not
	// followed, so it is never a real path reached through a link.
	Folder string
	// Name is what to call the folder: its last path element, which is also the
	// name a post goes out under when nothing else names the moot.
	Name string
	// Rooms is how many rooms the moot lists; -1 when they could not be read.
	Rooms int
}

// Scan walks home looking for folders with a .moot/ directory and returns them with
// the home directory first (if it holds one) and the rest in path order. It follows
// no directory symlink, skips node_modules, .git, vendor, a .moot/ itself and
// ~/Library, goes DefaultDepth levels down, and passes over a directory it cannot
// read: one permission error must not end the search. An empty result is a nil
// slice and no error. It reads directory names only, so it moves no seen marker.
func Scan(ctx context.Context, home string) ([]Entry, error) {
	root, err := filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("scan %q: %w", home, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("scan %q: %w", home, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan %q: not a directory", home)
	}

	// WalkDir does not follow a link, the root's included. A home reached through a
	// link is walked by its target, and the paths come back under the name it was
	// given, which is the name `~` is shortened by and a post is sent from.
	walkRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("scan %q: %w", home, err)
	}
	var folders []string
	err = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// An unreadable directory is one directory not searched, not the end of
		// the search. A symlink is not a directory here (d.IsDir is false for
		// one), so no link is followed.
		if err != nil || !d.IsDir() {
			return nil //nolint:nilerr // skipping is the whole point
		}
		if path != walkRoot {
			name := d.Name()
			if prunedDirs[name] || name == mootDir || (filepath.Dir(path) == walkRoot && rootPruned[name]) {
				return fs.SkipDir
			}
		}
		if info, err := os.Stat(filepath.Join(path, mootDir)); err == nil && info.IsDir() {
			folders = append(folders, filepath.Join(root, strings.TrimPrefix(path, walkRoot)))
		}
		if depthOf(walkRoot, path) >= DefaultDepth {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}

	sort.Slice(folders, func(i, j int) bool {
		// The home directory first, then path order.
		if a, b := folders[i] == root, folders[j] == root; a != b {
			return a
		}
		return folders[i] < folders[j]
	})
	var out []Entry
	for _, f := range folders {
		out = append(out, Entry{Folder: f, Name: filepath.Base(f), Rooms: countRooms(f)})
	}
	return out, nil
}

// countRooms lists the rooms through internal/moot, which is read-only.
func countRooms(folder string) int {
	m, err := moot.Open(folder)
	if err != nil {
		return -1
	}
	rooms, err := m.Rooms()
	if err != nil {
		return -1
	}
	return len(rooms)
}

// depthOf reports how many directories below root path sits. The root is 0.
func depthOf(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// Short writes a path under home with a tilde, which is how people say these paths
// out loud. home is passed in (EasyLoom asked the OS), so a test can use a fake one.
func Short(home, path string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
