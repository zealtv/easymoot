package moot

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultScanDepth matches the family tools. The root itself is depth zero.
const DefaultScanDepth = 4

// Discovery is an immutable listing of rooms, identified by keeper and name.
// PostName is the opened folder's local name for a heard room, or empty for read-only.
type Discovery struct {
	Rooms []Room
	// Participants are the folders the walk found holding .moot/rooms/, the opened
	// folder first (even when it lies outside root), the rest in walk order.
	Participants []string
	readers      map[string]*Moot
}

// Reader returns the keeper's read-only client for a room key.
func (d *Discovery) Reader(key string) (*Moot, string) {
	for _, r := range d.Rooms {
		if r.Key() == key {
			return d.readers[r.Key()], r.Name
		}
	}
	return nil, ""
}

// Discover adds real rooms below root to those the opened folder hears. Only
// hearing links are resolved; the directory walk never follows symlinked folders.
// Unreadable subtrees and git worktrees are skipped.
func Discover(opened, root string, depth int) (*Discovery, error) {
	if depth < 0 {
		return nil, fmt.Errorf("depth must be non-negative")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root %s: not a real directory", abs)
	}
	own, err := Open(opened)
	if err != nil {
		return nil, err
	}
	ownPath, err := filepath.EvalSymlinks(own.Folder())
	if err != nil {
		return nil, err
	}
	ownInfo, err := os.Lstat(ownPath)
	if err != nil {
		return nil, err
	}
	// EvalSymlinks preserves typed case. Keep one path spelling per directory
	// identity so hearing and walking agree on case-insensitive filesystems.
	type keeperIdentity struct {
		path string
		info os.FileInfo
	}
	keepers := []keeperIdentity{{ownPath, ownInfo}}
	keeperPath := func(path string) string {
		info, err := os.Lstat(path)
		if err != nil {
			return path
		}
		for _, k := range keepers {
			if os.SameFile(k.info, info) {
				return k.path
			}
		}
		keepers = append(keepers, keeperIdentity{path, info})
		return path
	}
	participants := []string{ownPath}
	found := map[string]Room{}
	paths := map[string]string{}
	roomFiles := map[string]os.FileInfo{}
	add := func(keeper, name, postName, roomPath string, standing Standing) {
		info, err := os.Lstat(roomPath)
		if err != nil || !info.IsDir() {
			return
		}
		// A heard target may use a different spelling of the room name, too.
		// Hearing is added first, so retain its local posting name and identity.
		for _, prior := range roomFiles {
			if os.SameFile(prior, info) {
				return
			}
		}
		r := Room{Name: name, Keeper: keeperPath(keeper), PostName: postName, Kind: KindOf(name, standing)}
		if r.Kind.Archived {
			r.PostName = "" // an archived room is read-only, as one the opened folder does not hear
		}
		if prev, ok := found[r.Key()]; ok && prev.PostName != "" {
			return
		}
		found[r.Key()] = r
		paths[r.Key()] = roomPath
		roomFiles[r.Key()] = info
	}
	// Scan the opened folder independently, including when it lies outside root.
	heard, err := own.Rooms()
	if err != nil {
		return nil, err
	}
	for _, r := range heard {
		target, err := filepath.EvalSymlinks(filepath.Join(own.Dir(), "rooms", r.Name))
		if err != nil {
			continue
		}
		keeper := filepath.Dir(filepath.Dir(filepath.Dir(target)))
		// Normal rooms live at <keeper>/.moot/rooms/<name>. Older arbitrary
		// hearing targets are still readable: their parent serves as their keeper.
		if filepath.Base(filepath.Dir(target)) != "rooms" || filepath.Base(filepath.Dir(filepath.Dir(target))) != ".moot" {
			keeper = filepath.Dir(target)
		}
		add(keeper, filepath.Base(target), r.Name, target, r.Kind.Standing)
	}
	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != abs {
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return fs.SkipDir
			}
		}
		if git, err := os.Lstat(filepath.Join(path, ".git")); err == nil && git.Mode().IsRegular() {
			return fs.SkipDir
		}
		// Lstat each level so a linked .moot or rooms directory isn't a scan route.
		roomsDir := filepath.Join(path, ".moot", "rooms")
		if realDir(filepath.Join(path, ".moot")) && realDir(roomsDir) {
			entries, _ := os.ReadDir(roomsDir)
			keeper, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil
			}
			keeperInfo, err := os.Lstat(keeper)
			if err != nil {
				return nil
			}
			if !os.SameFile(keeperInfo, ownInfo) {
				participants = append(participants, keeper)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".") || !realDir(filepath.Join(roomsDir, e.Name())) {
					continue
				}
				postName, standing := "", Elsewhere
				if os.SameFile(keeperInfo, ownInfo) {
					postName, standing = e.Name(), Kept
				}
				add(keeper, e.Name(), postName, filepath.Join(path, ".moot", "rooms", e.Name()), standing)
			}
		}
		rel, _ := filepath.Rel(abs, path)
		level := 0
		if rel != "." {
			level = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if level >= depth {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	d := &Discovery{Participants: participants, readers: map[string]*Moot{}}
	for _, r := range found {
		d.Rooms = append(d.Rooms, r)
	}
	sort.Slice(d.Rooms, func(i, j int) bool {
		a, b := d.Rooms[i], d.Rooms[j]
		if a.Keeper != b.Keeper {
			if a.Keeper == ownPath {
				return true
			}
			if b.Keeper == ownPath {
				return false
			}
			return a.Keeper < b.Keeper
		}
		return a.Name < b.Name
	})
	for _, r := range d.Rooms {
		d.readers[r.Key()] = &Moot{folder: r.Keeper, dir: filepath.Join(r.Keeper, ".moot"), roomRoot: filepath.Dir(paths[r.Key()]), cache: map[string]cachedBody{}}
	}
	return d, nil
}

func realDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}
