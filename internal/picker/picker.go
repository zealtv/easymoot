package picker

// Model is the picker's list and its cursor. It is a plain component, as EasyLoom's
// is: the shell owns the scan (it is I/O, so it runs as a command) and the drawing.
type Model struct {
	home    string
	entries []Entry
	cursor  int
	// scanning is true from Start until Done, so the view can say so rather than
	// look hung.
	scanning bool
	// notice is the last thing that happened: a scan's failure, a folder that
	// would not open.
	notice string
}

// New returns a picker that shortens paths against home.
func New(home string) Model { return Model{home: home} }

// Start marks a scan as running and forgets the old list, so a list from before is
// never offered while the new one is being made.
func (m Model) Start() Model {
	m.entries, m.cursor, m.scanning, m.notice = nil, 0, true, ""
	return m
}

// Done takes a scan's answer. A failed scan leaves the list empty and says why.
func (m Model) Done(entries []Entry, err error) Model {
	m.scanning = false
	m.entries, m.cursor = entries, 0
	if err != nil {
		m.entries, m.notice = nil, err.Error()
	}
	return m
}

// Notify sets the notice.
func (m Model) Notify(text string) Model { m.notice = text; return m }

// Scanning reports whether a scan is in flight.
func (m Model) Scanning() bool { return m.scanning }

// Notice is the last thing that happened, for the view to show.
func (m Model) Notice() string { return m.notice }

// Empty reports whether there is nothing to choose.
func (m Model) Empty() bool { return len(m.entries) == 0 }

// Entries is the list as scanned.
func (m Model) Entries() []Entry { return m.entries }

// Cursor is where the selection sits.
func (m Model) Cursor() int { return m.cursor }

// Selected is the entry under the cursor, or nil when the list is empty.
func (m Model) Selected() *Entry {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return nil
	}
	return &m.entries[m.cursor]
}

// Move steps the cursor, clamped rather than wrapped (EasyLoom's rule: the ends of a
// list are a place).
func (m Model) Move(delta int) Model { return m.MoveTo(m.cursor + delta) }

// MoveTo puts the cursor at an index, clamped.
func (m Model) MoveTo(i int) Model {
	m.cursor = max(0, min(i, len(m.entries)-1))
	return m
}

// Row is one line to draw.
type Row struct {
	Selected bool
	Name     string
	Path     string // with ~
	Rooms    int    // -1 when unknown
}

// Rows is the list as plain data, for the shell to style and frame.
func (m Model) Rows() []Row {
	rows := make([]Row, len(m.entries))
	for i, e := range m.entries {
		rows[i] = Row{Selected: i == m.cursor, Name: e.Name, Path: Short(m.home, e.Folder), Rooms: e.Rooms}
	}
	return rows
}
