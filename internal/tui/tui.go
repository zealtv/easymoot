// Package tui presents one folder's moot in a terminal shell: a sidebar of rooms,
// the selected room's posts as one transcript, a status line and a `:` palette.
// Rendering never touches the filesystem or starts processes; every read arrives
// as a message from a command.
//
// Key safety is the core rule, kept from Easyhog (quit and actions live only in
// the palette): arrows, Tab, the paging keys and the mouse
// only look; ":" opens the palette; Quit lives only there; ctrl+c is the one chord
// that always exits. A pane in a tmux window receives whatever is typed at it, so a
// stray letter must not quit or act. The shell is the structure Easyhog's
// internal/tui has (commit 01336f8): the layout, the key table, the palette, the
// cleaner and the renderer are copies, cut down to what Easymoot draws.
package tui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
)

// Source is what the shell reads from: one folder's moot. *moot.Moot is one. It is
// an interface so that a test can watch what is called and when; the shell calls it
// only from commands, never from Update or View.
type Source interface {
	Folder() string
	Rooms() ([]moot.Room, error)
	List(room string) ([]moot.Post, error)
	// PostsModTime is one stat of a room's posts/ directory: the chained poll uses it
	// to list a room other than the selected one only when something in it changed.
	PostsModTime(room string) (time.Time, error)
	Body(room, id string) (moot.Body, error)
	Name() (string, error)
	// Post is the one write: the entry bar's Enter reaches it, and nothing else does.
	Post(ctx context.Context, room, text string, opts ...moot.PostOption) (moot.PostResult, error)
}

var _ Source = (*moot.Moot)(nil)

// DefaultPollInterval is how often the selected room is read again. `moot.sh read
// --follow` polls at 2 seconds because nothing notifies it, and so does this
// (architecture, "Picking up new posts").
const DefaultPollInterval = 2 * time.Second

type pane int

const (
	paneRooms pane = iota
	panePosts
)

// postBody is what the poll read for one post: its body, or why it could not.
type postBody struct {
	moot.Body
	err string
}

// Model is the Bubble Tea model for the shell.
type Model struct {
	// exit is set when ctrl+c stops a post in flight; ExitLine reports from it.
	exit *exitNote

	src      Source
	home     string
	loc      *time.Location
	interval time.Duration

	styles styles
	// colour is whether the terminal gets colour at all (see colourOn). With it off
	// a post's text shows its markers as written.
	colour        bool
	width, height int

	// What the last good poll read.
	rooms   []moot.Room
	room    string // the selected room's name, "" when there is none
	sel     int    // index of room in rooms, -1 when there is none
	sideTop int    // first visible room
	posts   []moot.Post
	bodies  map[string]postBody
	// loaded: the rooms have been listed once. read: this room's posts have been.
	loaded, read bool
	name         string
	nameErr      string
	// loadErr is the last failed read, "" once a read works again. The old data stays.
	loadErr string

	// Unread marks, for this session only (nothing is stored). latest is what the
	// last listing of each room said (its post count and newest id; the polls in the
	// chain list the unselected rooms, and any read of the selected room updates it).
	// seen is the same for what each room held at launch, or when it was last on show;
	// a room whose latest is past its seen, and which is not selected, is in unread.
	latest map[string]roomTip
	seen   map[string]roomTip
	unread map[string]bool

	// The transcript at the pane's width, and where each post of posts begins in it.
	lines  []string
	starts []int
	// The view onto it. follow keeps the last line at the bottom. Otherwise the view
	// holds a place by post id and a line offset into that post, so a post that
	// arrives above the view (a same-second sender, a late sync) moves nothing
	// (review note L10). top is derived from these by settle.
	top       int
	follow    bool
	anchorID  string
	anchorOff int

	focus   pane
	palette *palette
	bar     *bar // the entry bar, while it is open; never open together with the palette
	notice  string
	failure bool
}

// Option customises a Model.
type Option func(*Model)

// WithPollInterval sets how often the selected room is read again.
func WithPollInterval(d time.Duration) Option { return func(m *Model) { m.interval = d } }

// WithLocation sets the zone post times are shown in; the default is the local one.
func WithLocation(loc *time.Location) Option { return func(m *Model) { m.loc = loc } }

// New shows the moot behind src. Nothing is read until Init's command runs.
func New(src Source, opts ...Option) Model {
	m := Model{
		src: src, loc: time.Local, interval: DefaultPollInterval,
		styles: newStyles(), colour: colourOn(),
		sel: -1, follow: true, bodies: map[string]postBody{},
	}
	if home, err := os.UserHomeDir(); err == nil {
		m.home = home
	}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// Init starts the first read, which also starts the poll.
func (m Model) Init() tea.Cmd {
	return pollCmd(m.src, "", nil, nil, true)
}

// size is the window size, or the default before one is known.
func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	return w, h
}

func (m Model) sizeW() int { w, _ := m.size(); return w }
func (m Model) sizeH() int { _, h := m.size(); return h }

func (m Model) layout() layout {
	w, h := m.size()
	return computeLayout(w, h, m.focus == panePosts)
}

// postsLayout is the layout as it would be with the posts pane showing, so the
// transcript is sized, and the view held, the same whichever pane has focus.
func (m Model) postsLayout() layout {
	l := m.layout()
	if !l.show {
		l = computeLayout(m.sizeW(), m.sizeH(), true)
	}
	return l
}

// postsWidth is the width a post's text wraps to: the pane less a margin each side.
func (m Model) postsWidth() int { return max(1, m.postsLayout().posts.w-2) }

// postsRows is how many transcript lines the posts pane shows, below its title.
func (m Model) postsRows() int { return max(1, m.postsLayout().posts.h-1) }

// sideRows is how many rooms fit in the sidebar, below its title.
func (m Model) sideRows() int {
	l := m.layout()
	if !l.showSide {
		l = computeLayout(m.sizeW(), m.sizeH(), false)
	}
	return max(1, l.side.h-1)
}

func (m Model) maxTop() int { return max(0, len(m.lines)-m.postsRows()) }

// rebuild lays the transcript out again, after posts, bodies or the size change,
// and puts the view back where it was held.
func (m Model) rebuild() Model {
	m.lines, m.starts = m.transcript(m.postsWidth())
	return m.settle()
}

// settle derives top from the held view and keeps the sidebar's window on the
// selected room.
func (m Model) settle() Model {
	limit := m.maxTop()
	switch i := m.anchorIndex(); {
	case m.follow || i < 0:
		m.top = limit
	default:
		m.top = m.starts[i] + max0(min(m.anchorOff, m.blockLen(i)-1))
	}
	m.top = max0(min(m.top, limit))
	rows := m.sideRows()
	if m.sel >= 0 {
		if m.sel < m.sideTop {
			m.sideTop = m.sel
		}
		if m.sel >= m.sideTop+rows {
			m.sideTop = m.sel - rows + 1
		}
	}
	m.sideTop = max0(min(m.sideTop, len(m.rooms)-rows))
	return m
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// anchorIndex is where the held post sits in posts now, -1 if there is none.
func (m Model) anchorIndex() int {
	if m.anchorID == "" || len(m.starts) != len(m.posts) {
		return -1
	}
	for i, p := range m.posts {
		if p.ID == m.anchorID {
			return i
		}
	}
	return -1
}

// blockLen is the number of transcript lines post i occupies, the blank lines after
// it included.
func (m Model) blockLen(i int) int {
	if i+1 < len(m.starts) {
		return m.starts[i+1] - m.starts[i]
	}
	return len(m.lines) - m.starts[i]
}

// scrollBy moves the view by delta lines. Reaching the bottom follows new posts
// again; anywhere else the view is held by the post at its top.
func (m Model) scrollBy(delta int) Model { return m.scrollTo(m.top + delta) }

func (m Model) scrollTo(top int) Model {
	limit := m.maxTop()
	top = max0(min(top, limit))
	if top >= limit {
		m.follow, m.anchorID, m.anchorOff, m.top = true, "", 0, limit
		return m
	}
	m.follow, m.top = false, top
	for i := len(m.starts) - 1; i >= 0; i-- {
		if m.starts[i] <= top {
			m.anchorID, m.anchorOff = m.posts[i].ID, top-m.starts[i]
			break
		}
	}
	return m
}

// page is how far a page key scrolls.
func (m Model) page() int { return max(1, m.postsRows()-1) }

// pollCmd reads the moot once: the rooms, the name, and the posts of the room
// asked for ("" asks for the first room). It is the only reading the shell does,
// and it runs as a command. known holds the bodies already read, which are not read
// again; the command never changes it. chain says the answer should start the next
// poll: the poll loop is one chain, and a read a key asks for is not part of it.
// Only a chained poll also looks at the other rooms for the unread marks, and it
// reads no body of theirs: prior is what the last poll found of each, and a room whose
// posts/ directory has not changed since is not listed again (see tipsOf).
func pollCmd(src Source, asked string, known map[string]postBody, prior map[string]roomTip, chain bool) tea.Cmd {
	return func() tea.Msg {
		msg := pollMsg{src: src, asked: asked, chain: chain}
		msg.name, msg.nameErr = src.Name()
		msg.rooms, msg.roomsErr = src.Rooms()
		if msg.roomsErr != nil {
			return msg
		}
		room := asked
		if room == "" && len(msg.rooms) > 0 {
			room = msg.rooms[0].Name
		}
		if !hasRoom(msg.rooms, room) {
			return msg // the model finds the room gone and chooses again
		}
		msg.room = room
		msg.posts, msg.listErr = src.List(room)
		if chain {
			msg.tips = tipsOf(src, msg.rooms, room, msg.posts, msg.listErr, prior)
		}
		if msg.listErr != nil {
			return msg
		}
		msg.bodies = make(map[string]postBody, len(msg.posts))
		for _, p := range msg.posts {
			if b, ok := known[p.ID]; ok && b.err == "" {
				msg.bodies[p.ID] = b
				continue
			}
			b, err := src.Body(room, p.ID)
			pb := postBody{Body: b}
			if err != nil {
				pb.err = err.Error()
			}
			msg.bodies[p.ID] = pb
		}
		return msg
	}
}

// roomTip is what one listing of a room said: how many posts it held and its newest
// id. A room has an unread post when either is past what Bob last saw: the count
// catches a post that sorts before the newest (a late sync) and the id catches one
// that replaces another. mtime and listed belong to the poll's gate (see tipsOf).
type roomTip struct {
	count  int
	newest string
	// mtime is the room's posts/ directory stamp read just before the listing, and
	// listed is when the listing ended. Zero for the selected room, which is read in
	// full each poll anyway.
	mtime, listed time.Time
}

func tipOf(posts []moot.Post) roomTip {
	t := roomTip{count: len(posts)}
	if len(posts) > 0 {
		t.newest = posts[len(posts)-1].ID // List is in bytewise order
	}
	return t
}

// past says t holds something the tip seen does not.
func (t roomTip) past(seen roomTip) bool {
	return t.count > seen.count || t.newest > seen.newest
}

// stampGrain is how long after a change a directory stamp cannot be trusted to have
// moved again: a coarse file system stamps a second or two, so a post landing in the
// same tick as a listing could leave the stamp as it was. A room changed more
// recently than this before its listing is listed again.
const stampGrain = 2 * time.Second

// tipsOf is every room's tip: from the listing the poll already has for the selected
// room, and for each other room from prior when its posts/ directory has the stamp it
// had when it was last listed (one stat), else from a fresh listing, names only. A
// room that cannot be listed has no entry.
func tipsOf(src Source, rooms []moot.Room, room string, posts []moot.Post, listErr error, prior map[string]roomTip) map[string]roomTip {
	tips := make(map[string]roomTip, len(rooms))
	for _, r := range rooms {
		if r.Name == room {
			if listErr == nil {
				tips[r.Name] = tipOf(posts)
			}
			continue
		}
		mtime, statErr := src.PostsModTime(r.Name)
		if statErr == nil {
			if p, ok := prior[r.Name]; ok && !p.listed.IsZero() && p.mtime.Equal(mtime) && p.listed.Sub(mtime) > stampGrain {
				tips[r.Name] = p
				continue
			}
		}
		ps, err := src.List(r.Name)
		if err != nil {
			continue
		}
		t := tipOf(ps)
		if statErr == nil { // with no stamp there is no gate: the room is listed each poll
			t.mtime, t.listed = mtime, time.Now()
		}
		tips[r.Name] = t
	}
	return tips
}

// track works out the unread marks from latest. The selected room never takes a mark
// and has its seen tip move up; a room met for the first time (at launch, or back
// after going away) starts read. A room with no entry in latest keeps what it had.
// The mark is derived each time from latest and seen, so a deleted post cannot leave
// one stuck.
func (m Model) track(latest map[string]roomTip) Model {
	seen := make(map[string]roomTip, len(m.rooms))
	unread := make(map[string]bool, len(m.rooms))
	for _, r := range m.rooms {
		prev, had := m.seen[r.Name]
		l, ok := latest[r.Name]
		switch {
		case !ok:
			if had {
				seen[r.Name], unread[r.Name] = prev, m.unread[r.Name]
			}
		case r.Name == m.room || !had:
			seen[r.Name] = l
		case l.past(prev):
			seen[r.Name], unread[r.Name] = prev, true
		default:
			seen[r.Name] = l // posts deleted: seen comes down with them
		}
	}
	m.latest, m.seen, m.unread = latest, seen, unread
	return m
}

// see takes posts the selected room shows now as seen, from a read that is not part
// of the chain (a key's, the one after a post): Bob's own post, or one he has watched
// arrive, must not mark the room when he leaves it.
func (m Model) see(posts []moot.Post) Model {
	latest := make(map[string]roomTip, len(m.latest)+1)
	for k, v := range m.latest {
		latest[k] = v
	}
	latest[m.room] = tipOf(posts)
	return m.track(latest)
}

func hasRoom(rooms []moot.Room, name string) bool {
	return roomIndex(rooms, name) >= 0
}

func roomIndex(rooms []moot.Room, name string) int {
	if name == "" {
		return -1
	}
	for i, r := range rooms {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// pollMsg is the answer to pollCmd.
type pollMsg struct {
	src   Source
	asked string // the room the command was asked for
	chain bool

	name    string
	nameErr error

	rooms    []moot.Room
	roomsErr error

	room    string // the room posts and bodies belong to; "" when none was read
	posts   []moot.Post
	bodies  map[string]postBody
	listErr error

	// tips is each room's tip, from a chained poll; nil from any other.
	tips map[string]roomTip
}

// tickMsg says the poll interval is up.
type tickMsg struct{ src Source }

func tick(src Source, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{src: src} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Keep the last good size if a terminal reports zeros.
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m.rebuild(), nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tickMsg:
		if msg.src != m.src {
			return m, nil
		}
		return m, pollCmd(m.src, m.room, m.bodies, m.latest, true)
	case pollMsg:
		return m.applyPoll(msg)
	case postMsg:
		return m.applyPost(msg)
	case quitMsg:
		if msg.src == m.src && m.bar != nil && m.bar.quitting {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// applyPoll takes a read's answer. An answer for another folder, or for a room that
// is no longer the selected one, is dropped, but a chained poll still starts the
// next one, so the loop never stops.
func (m Model) applyPoll(msg pollMsg) (tea.Model, tea.Cmd) {
	if msg.src != m.src {
		return m, nil
	}
	var next tea.Cmd
	if msg.chain {
		next = tick(m.src, m.interval)
	}
	if msg.asked != m.room {
		return m, next
	}
	if msg.roomsErr != nil {
		m.loadErr = "could not list rooms: " + cleanLine(msg.roomsErr.Error())
		return m, next
	}
	m.loaded = true
	m.name, m.nameErr = msg.name, ""
	if msg.nameErr != nil {
		m.name, m.nameErr = "", cleanLine(msg.nameErr.Error())
	}
	prev := m.sel
	m.rooms = msg.rooms
	if m.room == "" && len(m.rooms) > 0 {
		m.room = msg.room // the first room, at start
	}
	m.sel = roomIndex(m.rooms, m.room)
	if msg.tips != nil {
		m = m.track(msg.tips)
	}
	if m.sel < 0 && m.room != "" {
		// The selected room is gone: take its neighbour and read that.
		i := min(max0(prev), len(m.rooms)-1)
		m.room, m.sel = "", -1
		if i >= 0 {
			m.room, m.sel = m.rooms[i].Name, i
		}
		m = m.clearPosts().rebuild().track(m.latest) // the neighbour is on show, so read
		return m, tea.Batch(next, m.readSelected())
	}
	switch {
	case msg.listErr != nil:
		m.loadErr = "could not read " + cleanLine(m.room) + ": " + cleanLine(msg.listErr.Error())
	case msg.room == m.room && m.room != "":
		m.loadErr = ""
		m.posts, m.bodies, m.read = msg.posts, msg.bodies, true
		if msg.tips == nil {
			m = m.see(msg.posts) // a read outside the chain still shows Bob these posts
		}
	default:
		m.loadErr = ""
	}
	return m.rebuild(), next
}

// readSelected is the read a looking key may schedule: the selected room, and
// nothing else, outside the poll chain.
func (m Model) readSelected() tea.Cmd {
	if m.room == "" {
		return nil
	}
	return pollCmd(m.src, m.room, nil, nil, false)
}

// clearPosts forgets the posts shown, for a room just selected.
func (m Model) clearPosts() Model {
	m.posts, m.bodies, m.read = nil, map[string]postBody{}, false
	m.follow, m.anchorID, m.anchorOff = true, "", 0
	return m
}

// updateKey is the whole key routing table. Everything that is not listed here
// does nothing, so adding a binding means adding it here, and the routing tests
// fail if it quits or acts outside the palette.
func (m Model) updateKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type == tea.KeyCtrlC {
		return m.stopPost()
	}
	if m.bar != nil {
		return m.updateBar(k)
	}
	if m.palette != nil {
		return m.updatePalette(k)
	}
	if k.Paste {
		return m, nil
	}
	switch k.String() {
	case paletteKey:
		return m.openPalette(), nil
	case "tab", "shift+tab":
		if m.focus == paneRooms {
			m.focus = panePosts
		} else {
			m.focus = paneRooms
		}
		return m.settle(), nil
	case "up":
		return m.arrow(-1)
	case "down":
		return m.arrow(1)
	case "pgup":
		return m.scrollBy(-m.page()), nil
	case "pgdown":
		return m.scrollBy(m.page()), nil
	case "ctrl+u":
		return m.scrollBy(-max(1, m.page()/2)), nil
	case "ctrl+d":
		return m.scrollBy(max(1, m.page()/2)), nil
	case "shift+up":
		return m.scrollBy(-1), nil
	case "shift+down":
		return m.scrollBy(1), nil
	case "home":
		return m.scrollTo(0), nil
	case "end":
		return m.scrollTo(len(m.lines)), nil
	}
	return m, nil
}

// arrow moves the selected room with the sidebar focused, and scrolls the posts
// with the posts focused.
func (m Model) arrow(delta int) (tea.Model, tea.Cmd) {
	if m.focus == panePosts {
		return m.scrollBy(delta), nil
	}
	return m.jump(m.sel + delta)
}

// jump selects room i. The posts shown go at once, and the read of the new room
// comes back as a message, so an answer for the room left behind is dropped.
func (m Model) jump(i int) (tea.Model, tea.Cmd) {
	if len(m.rooms) == 0 {
		return m, nil
	}
	i = max0(min(i, len(m.rooms)-1))
	if i == m.sel {
		return m, nil
	}
	m.sel, m.room = i, m.rooms[i].Name
	m = m.clearPosts().rebuild().track(m.latest)
	return m, m.readSelected()
}

// updateMouse lets the wheel scroll and a single click select a room or focus a
// pane. Clicks only look: a press selects the room under it and nothing more.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.palette != nil || m.bar != nil {
		return m, nil
	}
	l := m.layout()
	if l.mode == layoutTiny {
		return m, nil
	}
	inSide := l.showSide && l.side.contains(msg.X, msg.Y)
	inPosts := l.show && l.posts.contains(msg.X, msg.Y)
	switch {
	case msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown:
		d := 3
		if msg.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		switch {
		case inSide:
			m.sideTop = max0(min(m.sideTop+d, len(m.rooms)-m.sideRows()))
		case inPosts:
			return m.scrollBy(d), nil
		}
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		switch {
		case inSide:
			m.focus = paneRooms
			if i := m.sideTop + msg.Y - l.side.y - 1; msg.Y > l.side.y && i < len(m.rooms) {
				return m.jump(i)
			}
		case inPosts:
			m.focus = panePosts
		}
	}
	return m, nil
}

// displayPath shortens the folder with ~ for the header.
func (m Model) displayPath(p string) string {
	if m.home != "" && (p == m.home || strings.HasPrefix(p, m.home+"/")) {
		return "~" + strings.TrimPrefix(p, m.home)
	}
	return p
}
