// Package tui presents one folder's moot in a terminal shell: a sidebar of rooms,
// the selected room's posts as one transcript, a status line and a `:` palette.
// Rendering never touches the filesystem or starts processes; every read arrives
// as a message from a command.
//
// Key safety is the core rule, kept from Easyhog (quit and actions live only in
// the palette): arrows, Tab, the paging keys and the mouse
// only look; ">" opens the palette; Quit lives only there; ctrl+c is the one chord
// that always exits. A pane in a tmux window receives whatever is typed at it, so a
// stray letter must not quit or act. The shell is the structure Easyhog's
// internal/tui has (commit 01336f8): the layout, the key table, the palette, the
// cleaner and the renderer are copies, cut down to what Easymoot draws.
package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/twfocus"
	"github.com/zealtv/easymoot/internal/twglance"
	"github.com/zealtv/easymoot/internal/twpane"
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
	// OwnName is Name without $MOOT_NAME: the name the folder's files give it, which
	// moot.sh also names a DM room from.
	OwnName() (string, error)
	// Post is the write that carries text: the entry bar's Enter reaches it.
	Post(ctx context.Context, room, text string, opts ...moot.PostOption) (moot.PostResult, error)
	// Run is the other way into the folder's moot.sh: a room verb, or rooms (link
	// repair). A typed Enter on a confirm or name bar, and a poll that repairs,
	// reach it.
	Run(ctx context.Context, args []string, opts ...moot.PostOption) (moot.RunResult, error)
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
	// twFocus tells a pane previewer where the posts are (twfocus). Only Run gives
	// it a pane, so tests and one-frame output report nothing.
	twFocus twfocus.Pane
	// tw is the same pane, for @tw-next and the wait on a mention (glance.go).
	tw twpane.Pane
	// twg is the same pane, for the glance: the badge line and the listeners (glance.go).
	twg twglance.Pane

	// exit is set when ctrl+c stops a post in flight; ExitLine reports from it.
	exit *exitNote

	root         string
	depth        int
	discovery    *moot.Discovery
	pollRound    int
	walkSequence *atomic.Uint64 // shared by model copies; assigned when scheduling a walk
	appliedWalk  uint64         // newest successful walk answer applied
	src          Source
	home         string
	loc          *time.Location
	interval     time.Duration

	styles styles
	// colour is whether the terminal gets colour at all (see colourOn). With it off
	// a post's text shows its markers as written.
	colour        bool
	width, height int

	// What the last good poll read.
	// all is every room the last listing found; rooms is the ones on show, grouped
	// in the order the sidebar lists them (see kinds.go). Hidden archived rooms are in
	// all and nowhere else: they are not polled.
	all   []moot.Room
	rooms []moot.Room
	// showArchived puts archived rooms on show. Nothing stores it: it is off at launch.
	showArchived bool
	// ownName is the folder's name from its files, "" if they give none usable.
	ownName string
	room    string // the selected room's Key, "" when there is none
	sel     int    // index of room in rooms, -1 when there is none
	sideTop int    // first visible sidebar row (headers included)
	posts   []moot.Post
	bodies  map[string]postBody
	// loaded: the rooms have been listed once. read: this room's posts have been.
	loaded, read bool
	name         string
	nameErr      string
	// loadErr is the last failed read, "" once a read works again. The old data stays.
	loadErr string
	// acting says a room verb is in flight: one at a time, and while it runs the
	// palette lists no command that acts.
	acting bool

	// The participants (participants.go) list in the sidebar below the rooms. people is
	// what the last poll read of them; person is the last selected one's folder, and
	// onPerson says the selection is on it rather than on the room.
	people   []person
	person   string
	onPerson bool

	// sideWidth is the sidebar's width once a resize set it, 0 before; nothing stores
	// it. dragging says a press on the divider column is being dragged.
	sideWidth int
	dragging  bool

	// Unread marks, for this session only (nothing is stored). latest is what the
	// last listing of each room said (its post count and newest id; the polls in the
	// chain list the unselected rooms, and any read of the selected room updates it).
	// seen is the same for what each room held at launch, or when it was last on show;
	// a room whose latest is past its seen, and which is not selected, is in unread.
	latest map[string]roomTip
	seen   map[string]roomTip
	unread map[string]bool
	// hits is what the last chained poll found of the posts past seen in the other
	// heard rooms: which mention the reader (glance.go).
	hits map[string]mentionHit

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
		walkSequence: &atomic.Uint64{},
		sel:          -1, follow: true, bodies: map[string]postBody{},
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
	return m.pollAs("", nil, nil, true, m.root != "", pollOpts{repair: true})
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
	return computeLayout(w, h, m.focus == panePosts, m.sideWidth)
}

// postsLayout is the layout as it would be with the posts pane showing, so the
// transcript is sized, and the view held, the same whichever pane has focus.
func (m Model) postsLayout() layout {
	l := m.layout()
	if !l.show {
		l = computeLayout(m.sizeW(), m.sizeH(), true, m.sideWidth)
	}
	return l
}

// postsWidth is the width a post's text wraps to: the pane less a margin each side.
func (m Model) postsWidth() int { return max(1, m.postsLayout().posts.w-2) }

// postsRows is how many transcript lines the posts pane shows, below its title.
func (m Model) postsRows() int { return max(1, m.postsLayout().posts.h-1) }

// sideLayout is the layout as it would be with the sidebar showing.
func (m Model) sideLayout() layout {
	l := m.layout()
	if !l.showSide {
		l = computeLayout(m.sizeW(), m.sizeH(), false, m.sideWidth)
	}
	return l
}

// sideRows is how many rows fit in the sidebar, below its title.
func (m Model) sideRows() int { return max(1, m.sideLayout().side.h-1) }

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
	rows := m.sidebar()
	m.sideTop = window(m.sideTop, selectedRow(rows, m), m.sideRows(), len(rows))
	return m
}

// window is a list's first visible row, moved as little as keeps row at (if any) among
// the rows shown of n.
func window(top, at, rows, n int) int {
	if at >= 0 {
		if at < top {
			top = at
		}
		if at >= top+rows {
			top = at - rows + 1
		}
	}
	return max0(min(top, n-rows))
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
func pollCmd(src Source, asked string, known map[string]postBody, prior map[string]roomTip, chain bool, o pollOpts, requests ...discoveryRequest) tea.Cmd {
	return func() tea.Msg {
		src := src // each invocation has its own immutable read view
		msg := pollMsg{src: src, asked: asked, chain: chain}
		if o.repair {
			// moot.sh mends this folder's links at the start of every command, so
			// the listing below sees them mended.
			res, err := src.Run(context.Background(), []string{"rooms"})
			msg.repairs, msg.repairErr = res.Notices, err
		}
		if len(requests) > 0 {
			req := requests[0]
			msg.walk, msg.walkSequence = req.walk, req.sequence
			d := req.prior
			if req.walk {
				d, msg.roomsErr = moot.Discover(src.Folder(), req.root, req.depth)
			}
			if msg.roomsErr != nil {
				return msg
			}
			msg.discovery = d
			src = discoveredSource{Source: src, discovery: d}
			if o.people {
				// Presence files are read only while Participants shows, or inside tmux for the glance.
				msg.people = peopleOf(d.Participants, time.Now())
			}
		}
		msg.name, msg.nameErr = src.Name()
		msg.own, _ = src.OwnName()
		msg.rooms, msg.roomsErr = src.Rooms()
		if msg.roomsErr != nil {
			return msg
		}
		// Hidden archived rooms are neither read nor polled: only the rooms on show are.
		onShow := arrange(msg.rooms, o.show, len(requests) > 0, nil)
		room := asked
		if room == "" && len(onShow) > 0 {
			room = onShow[0].Key()
		}
		if o.pick != nil {
			if k := o.pick(onShow); k != "" {
				room, msg.picked = k, true
			}
		}
		if !hasRoom(onShow, room) {
			return msg // the model finds the room gone and chooses again
		}
		msg.room = room
		msg.posts, msg.listErr = src.List(room)
		if chain {
			listed := map[string][]moot.Post{}
			msg.tips = tipsOf(src, onShow, room, msg.posts, msg.listErr, prior, listed)
			if msg.nameErr == nil {
				exclude := room
				if o.onPerson {
					exclude = ""
				}
				msg.hits = checkMentions(src, onShow, exclude, msg.tips, o.seen, listed, o.hits, msg.name, time.Now())
			}
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

// pollOpts are what a poll takes besides the room asked for.
type pollOpts struct {
	show   bool // archived rooms are on show
	people bool // Participants shows, or the glance needs it: read each participant's presence too
	repair bool // run moot.sh rooms first, so links are mended before they are listed
	// pick, if set, names the room to select out of the rooms listed: where an action
	// leaves the reader's attention (the room just created). "" leaves the selection.
	pick func(rooms []moot.Room) string
	// seen and hits are the model's, for a chained poll's check of mentions.
	seen     map[string]roomTip
	hits     map[string]mentionHit
	onPerson bool // a participant is selected, so no room is on show
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
// room that cannot be listed has no entry. listed, if not nil, takes each listing made.
func tipsOf(src Source, rooms []moot.Room, room string, posts []moot.Post, listErr error, prior map[string]roomTip, listed map[string][]moot.Post) map[string]roomTip {
	tips := make(map[string]roomTip, len(rooms))
	for _, r := range rooms {
		if r.Key() == room {
			if listErr == nil {
				tips[r.Key()] = tipOf(posts)
				if listed != nil {
					listed[r.Key()] = posts
				}
			}
			continue
		}
		mtime, statErr := src.PostsModTime(r.Key())
		if statErr == nil {
			if p, ok := prior[r.Key()]; ok && !p.listed.IsZero() && p.mtime.Equal(mtime) && p.listed.Sub(mtime) > stampGrain {
				tips[r.Key()] = p
				continue
			}
		}
		ps, err := src.List(r.Key())
		if err != nil {
			continue
		}
		if listed != nil {
			listed[r.Key()] = ps
		}
		t := tipOf(ps)
		if statErr == nil { // with no stamp there is no gate: the room is listed each poll
			t.mtime, t.listed = mtime, time.Now()
		}
		tips[r.Key()] = t
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
		prev, had := m.seen[r.Key()]
		l, ok := latest[r.Key()]
		switch {
		case !ok:
			if had {
				seen[r.Key()], unread[r.Key()] = prev, m.unread[r.Key()]
			}
		case r.Key() == m.room && !m.onPerson || !had || r.Kind.Archived:
			// An archived room takes no unread mark: nothing new is posted there.
			seen[r.Key()] = l
		case l.past(prev):
			seen[r.Key()], unread[r.Key()] = prev, true
		default:
			seen[r.Key()] = l // posts deleted: seen comes down with them
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
		if r.Key() == name {
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
	hits  map[string]mentionHit // a chained poll's check of mentions (glance.go)

	name    string
	nameErr error
	own     string // the folder's name from its files

	// repairs are the lines moot.sh rooms wrote to standard error, when the poll ran it.
	repairs   []string
	repairErr error

	rooms    []moot.Room // every room found, archived ones too
	roomsErr error

	room    string // the room posts and bodies belong to; "" when none was read
	picked  bool   // room was chosen by the poll's pick, not asked for
	posts   []moot.Post
	bodies  map[string]postBody
	listErr error

	// people is each participant's presence, read while Participants shows or inside
	// tmux (peopleOnShow); else nil.
	people []person

	// tips is each room's tip, from a chained poll; nil from any other.
	tips         map[string]roomTip
	discovery    *moot.Discovery
	walk         bool
	walkSequence uint64
}

// tickMsg says the poll interval is up.
type tickMsg struct{ src Source }

func tick(src Source, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{src: src} })
}

// Update handles a message, then tells a pane previewer where the posts are now.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if f := nm.twFocus.Report(nm.focusRect()); f != nil {
		cmd = tea.Batch(cmd, f)
	}
	if g := nm.tw.Sync(nm.glance().values()); g != nil {
		cmd = tea.Batch(cmd, g)
	}
	if g := nm.twg.Sync(nm.glanceParts()); g != nil {
		cmd = tea.Batch(cmd, g)
	}
	return nm, cmd
}

// focusRect is the part of the screen a small preview should show: the posts
// pane, newest at the bottom. With the posts not on screen (one pane at a time,
// the sidebar's turn) it names nothing.
func (m Model) focusRect() twfocus.Rect {
	l := m.layout()
	if !l.show || l.posts.w < 1 || l.posts.h < 1 {
		return twfocus.Rect{}
	}
	return twfocus.Rect{X: l.posts.x, Y: l.posts.y, W: l.posts.w, H: l.posts.h, Bottom: true}
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.pollRound++
		return m, m.poll(m.room, m.bodies, m.latest, true, m.pollRound%15 == 0)
	case pollMsg:
		return m.applyPoll(msg)
	case postMsg:
		return m.applyPost(msg)
	case actionMsg:
		return m.applyAction(msg)
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
	m = m.noteRepairs(msg)
	if msg.walk && msg.walkSequence < m.appliedWalk {
		return m, next
	}
	stale := msg.asked != m.room && !msg.picked
	if stale && !msg.walk {
		return m, next
	}
	// A read started before a changed discovery must never restore its old tree.
	if msg.discovery != nil && m.discovery != nil && msg.discovery != m.discovery && !msg.walk {
		return m, next
	}
	if msg.people != nil {
		m = m.withPeople(msg.people)
	}
	if msg.roomsErr != nil {
		m.loadErr = "could not list rooms: " + cleanLine(msg.roomsErr.Error())
		return m, next
	}
	if msg.walk {
		m.appliedWalk = msg.walkSequence
	}
	m.loaded = true
	m.name, m.nameErr = msg.name, ""
	if msg.nameErr != nil {
		m.name, m.nameErr = "", cleanLine(msg.nameErr.Error())
	}
	prev := m.sel
	if !sameRooms(m.all, msg.rooms) {
		m.all = msg.rooms
	}
	m.ownName = msg.own
	m = m.withRooms()
	if msg.picked && msg.room != m.room {
		m.room = msg.room
		m = m.clearPosts()
	}
	if msg.discovery != nil && (m.discovery == nil || !sameRooms(m.discovery.Rooms, msg.discovery.Rooms) ||
		!slices.Equal(m.discovery.Participants, msg.discovery.Participants)) {
		m.discovery = msg.discovery
	}
	if m.room == "" && len(m.rooms) > 0 {
		m.room = msg.room // the first room, at start
	}
	m.sel = roomIndex(m.rooms, m.room)
	if msg.tips != nil {
		m = m.track(msg.tips).withRooms() // the order of direct follows the newest posts
		m.hits = msg.hits
	}
	if m.sel < 0 && m.room != "" {
		// The selected room is gone: take its neighbour and read that.
		m = m.takeNeighbour(prev)
		return m, tea.Batch(next, m.readSelected())
	}
	if stale {
		return m.rebuild(), next
	}
	switch {
	case msg.listErr != nil:
		m.loadErr = "could not read " + cleanLine(m.roomName()) + ": " + cleanLine(msg.listErr.Error())
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
	return m.poll(m.room, nil, nil, false, false)
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
	case "shift+left":
		return m.resize(m.sideLayout().side.w - resizeStep), nil
	case "shift+right":
		return m.resize(m.sideLayout().side.w + resizeStep), nil
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

// arrow moves the sidebar's selection with the sidebar focused, through the rooms
// and on into the participants as one list, and scrolls the posts with the posts
// focused.
func (m Model) arrow(delta int) (tea.Model, tea.Cmd) {
	if m.focus == panePosts {
		return m.scrollBy(delta), nil
	}
	rows := m.sidebar()
	for i := selectedRow(rows, m) + delta; i >= 0 && i < len(rows); i += delta {
		switch {
		case rows[i].room >= 0:
			return m.jump(rows[i].room)
		case rows[i].person >= 0:
			return m.pickPerson(rows[i].person), nil
		}
	}
	return m, nil
}

// resizeStep is how many columns shift+←/→ moves the divider.
const resizeStep = 2

// resize sets the sidebar's width for the session, within sideBounds. The one-pane
// layout has no divider and takes no width.
func (m Model) resize(w int) Model {
	if m.layout().mode != layoutSplit {
		return m
	}
	m.sideWidth = sideBounds(w, m.sizeW())
	return m.rebuild()
}

// jump selects room i. The posts shown go at once, and the read of the new room
// comes back as a message, so an answer for the room left behind is dropped. Back
// on the room a participant was selected from, the posts the poll kept are shown.
func (m Model) jump(i int) (tea.Model, tea.Cmd) {
	if len(m.rooms) == 0 {
		return m, nil
	}
	i = max0(min(i, len(m.rooms)-1))
	if i == m.sel {
		if m.onPerson {
			m.onPerson = false
			return m.track(m.latest).settle(), nil
		}
		return m, nil
	}
	m.onPerson = false
	m.sel, m.room = i, m.rooms[i].Key()
	m = m.clearPosts().rebuild().track(m.latest)
	return m, m.readSelected()
}

// updateMouse lets the wheel scroll, a single click select a room or a participant
// or focus a pane, and a drag on the divider column resize the sidebar. Clicks only
// look: a press selects the row under it and nothing more.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.palette != nil || m.bar != nil {
		return m, nil
	}
	l := m.layout()
	if l.mode == layoutTiny {
		return m, nil
	}
	if msg.Button == tea.MouseButtonLeft || msg.Action == tea.MouseActionRelease {
		switch {
		case msg.Action == tea.MouseActionPress && l.mode == layoutSplit && msg.X == l.side.w && l.body.contains(msg.X, msg.Y):
			m.dragging = true
			return m, nil
		case m.dragging && msg.Action == tea.MouseActionMotion:
			return m.resize(msg.X), nil
		case m.dragging:
			m.dragging = false
			return m, nil
		}
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
			m.sideTop = max0(min(m.sideTop+d, len(m.sidebar())-m.sideRows()))
		case inPosts:
			return m.scrollBy(d), nil
		}
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		switch {
		case inSide:
			m.focus = paneRooms
			if i := m.sideTop + msg.Y - l.side.y - 1; msg.Y > l.side.y && i < len(m.sidebar()) {
				switch row := m.sidebar()[i]; {
				case row.room >= 0:
					return m.jump(row.room)
				case row.person >= 0:
					return m.pickPerson(row.person), nil
				}
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
