package moot

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MaxBody is the most of one post's text Body reads.
const MaxBody = 1 << 20

// Body is what a post holds.
type Body struct {
	// Text is the post's text as stored, up to MaxBody bytes.
	Text      string
	Truncated bool
	// Missing marks a post directory with no readable post.md.
	Missing bool
	// Attached names a post directory's other files, as moot.sh's "attached:" line
	// does: visible, existing, not post.md, in byte order.
	Attached []string
}

type bodySig struct {
	path     string
	size     int64
	mtime    time.Time
	dirMtime time.Time // for a post directory, so Attached follows changes
	exists   bool
}

type cachedBody struct {
	sig  bodySig
	body Body
}

// Body reads a post's text. Results are cached by room and id, and reused while
// the body file's size and modification time (and, for a post directory, the
// directory's) stay the same.
func (m *Moot) Body(room, id string) (Body, error) {
	if !plainName(room) || !plainName(id) {
		return Body{}, errors.New("invalid room or post id")
	}
	postsDir := m.postsDir(room)
	entry := filepath.Join(postsDir, id)
	var path string
	var dirMtime time.Time
	isDir := false
	if info, err := os.Stat(entry); err == nil && info.IsDir() {
		isDir = true
		path = filepath.Join(entry, "post.md")
		dirMtime = info.ModTime()
	} else {
		path = entry + ".md"
	}

	sig := bodySig{path: path, dirMtime: dirMtime}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		sig.exists, sig.size, sig.mtime = true, info.Size(), info.ModTime()
	}

	key := room + "\x00" + id
	m.mu.Lock()
	c, ok := m.cache[key]
	m.mu.Unlock()
	if ok && c.sig == sig {
		return c.body, nil
	}

	var b Body
	if !sig.exists {
		b.Missing = isDir
		if !isDir {
			return Body{}, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}
	} else {
		f, err := os.Open(path)
		if err != nil {
			if !isDir {
				return Body{}, err
			}
			b.Missing = true
		} else {
			data, err := io.ReadAll(io.LimitReader(f, MaxBody+1))
			f.Close()
			if err != nil {
				return Body{}, err
			}
			if len(data) > MaxBody {
				data, b.Truncated = data[:MaxBody], true
			}
			b.Text = string(data)
		}
	}
	if isDir {
		b.Attached = attached(entry)
	}

	m.mu.Lock()
	m.cache[key] = cachedBody{sig: sig, body: b}
	m.mu.Unlock()
	return b, nil
}

func attached(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") || n == "post.md" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
