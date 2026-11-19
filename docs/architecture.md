# Easymoot architecture

How the MVP in `docs/design.md` gets built. Written on 2026-11-18, before the build, by
Claude Fable 5.2, and revised after one review the same night; Bob decided its open
questions that day (see [Decided](#decided)). The build has since moved past it in
places (the code is the authority).

It starts from Easyhog's and EasyLoom's shapes and says where Easymoot departs. The MVP
stays simple: a question that use can answer sits under [Left for use](#left-for-use).

## What this rests on

All read on 2026-11-18; check a detail again before building on it.

| Source | Commit |
|---|---|
| [moot](https://github.com/zealtv/moot) (`.moot/moot.sh`, `README.md`, `SKILL.md`) | `c25b5bb` |
| EasyLoom | `ca2f8f6` |
| Easyhog | `9d82857` |

"Experiment" below means a throwaway Go program run against a synthetic moot with a
copy of that `moot.sh`. The author's real moots (twelve folders, `~/.moot` among them)
were only read, and showed:

- Rooms are symlinks with absolute targets into a shared store.
  Four of the twelve folders had no rooms at all.
- No folder had a `name` file or a `.gremlin/name`, so every name came from its folder.
- Posts are single `.md` files of prose with little Markdown, usually several long
  lines, with a sign-off line directly under the text. Rooms are small: about 100 posts
  in all, the largest post about 5 KB.

## Shape

```
cmd/easymoot/        arguments, the terminal, start-up
internal/moot/       one folder's .moot/, headless: rooms, posts, the name, post
internal/picker/     scan for folders that have a .moot/, and the list to pick from
internal/tui/        Bubble Tea shell: sidebar, posts pane, palette, entry bar
```

Go 1.24 (`go 1.24.0`), Bubble Tea v1.3.10, Lip Gloss v1.1.0, `charmbracelet/x/term`
v0.2.1 and `charmbracelet/x/ansi` v0.10.1 (both peers' palettes import it), as the
peers' `go.mod` files have them. No other dependency. Module path
`github.com/zealtv/easymoot`, beside the peers' (a name only: no remote exists, and
none is made without Bob).

`internal/moot` is the headless core. Easyhog splits a core from a script client
(`internal/core`, `internal/groundhog`); a moot keeps no state beyond its files and the
only script call is `post`, so one package holds both.

Two rules from the peers carry over unchanged:

- **I/O stays out of `Update` and `View`.** Listing, reading bodies, resolving the name,
  scanning and posting run in Bubble Tea commands and come back as messages; an answer
  for a room or folder that is no longer selected is dropped (Easyhog,
  `docs/architecture.md`, `internal/tui`).
- **Text from a moot is inert.** Room names, sender names and post bodies pass through
  one cleaner before they are drawn, which turns every control character, escape
  included, into a visible middle dot (Easyhog, `internal/tui/state.go:272`).

## Choosing a folder

- `easymoot [path]` opens `<path>/.moot/` (the working directory by default). `path` may
  also name the `.moot/` directory itself.
- If that folder has no `.moot/`, and no path was typed, the picker opens. A typed path
  with no `.moot/` is an error, as an unresolved loom path is in EasyLoom
  (`cmd/easyloom/main.go:140`).
- The picker lists every folder that a scan finds holding a `.moot/` directory: name,
  path (with `~`), and how many rooms. Folders with rooms come first, `~` at the top,
  the rest in path order. The scan is EasyLoom's walk (`internal/target/scan.go`): from
  the parent of the working directory, four levels down, following no directory
  symlink, skipping `node_modules`, `.git` and `vendor`. It also looks at `~/.moot`
  itself.
- The palette's **Switch folder** opens the same picker over the running shell and scans
  again.

Departures from EasyLoom:

1. **No walk up from the working directory** (EasyLoom: `internal/target/resolve.go:102`).
   `~/.moot` exists, so a walk up from anywhere under home would end there and the
   picker would never open. The folder also decides the posting name, so Easymoot opens
   exactly the folder it was given, or asks.
2. **The path is kept as typed, made absolute, with symlinks left alone** (EasyLoom
   resolves to the real path, `resolve.go:139`). `moot.sh` names the sender after the
   folder it was invoked through (`moot.sh:15`, `126`); in the experiment a post through
   a symlinked folder went out under the link's name.
3. **No remembered list.** A scan is cheap and a moot needs no probe, so the picker
   scans each time and stores nothing. EasyLoom's `internal/known` is not copied.
4. **A match is a directory named `.moot`**, with or without a script. Reading needs no
   tool; where `moot.sh` is missing, Post says so when tried.

`internal/picker` is copied from EasyLoom's `internal/picker` and its walk, cut down (no
tiers, no probe, no forget), as Easyhog copied EasyLoom's Markdown renderer; the peers
share no module.

## Reading a room

`internal/moot` reads the files directly and never runs the script to read. `moot.sh`'s
own listing is the authority, and the Go listing copies it rule for rule:

| Rule | `moot.sh` |
|---|---|
| A room is a directory under `rooms/` (links followed), not hidden, in byte order. A link that leads nowhere is not a room. | `subscribed_rooms`, 140–149 |
| A post id is a visible directory's name, or a visible `*.md` file's name without `.md`. Other files are not posts. | `post_ids`, 211–224 |
| Ids are unique and sort bytewise (`LC_ALL=C`), oldest first, newest last. | 13, 223 |
| The body is `<id>/post.md` when the id is a directory, else `<id>.md`. A directory wins over a file of the same id. | `post_body_path`, 226–233 |
| An id shaped `<8 digits>-<6 digits>-<sender>` is a stamp: UTC time, and the sender is everything after the second dash. | `is_stamp_id`, 653–658; `render_post`, 699–704 |
| Any other id (a stray `README.md`) is listed under its own name, with no time. | 705–707 |
| A post directory lists its other files as `attached: a b`; one with no `post.md` says so. | 709–726 |

In the experiment the Go listing gave the same ids in the same order as `moot.sh read`,
over a room holding each of these shapes.

**Seen markers.** `internal/moot` has no code path that opens anything under `seen/`,
and Easymoot never runs `read --new`, `catchup`, `join`, `init` or `subscribe`: in
`moot.sh` those are the only verbs that write there (`cursor_put`, 738–741; `mkdir`
at 269, 347, 436).

## Rendering

The posts pane shows the selected room as one transcript, oldest at the top, newest at
the bottom, opened at the bottom, as `moot.sh read` prints it. Each post gets one
heading line, then its body:

```
── 2026-08-29 21:00 · alice ──
The nightly index rebuilt; it is 40% smaller.
— alice
```

- The heading is `read`'s (`moot.sh:699–704`): the time in Bob's local zone, then the
  sender, with the seconds and the zone dropped. The filename stays UTC. A stamp that
  is no real date shows as written, marked UTC.
- The body goes through EasyLoom's Markdown renderer, ported as Easyhog ported it
  (`internal/tui/markdown.go` in both: pure, text and a width in, lines out, readable
  with colour off).
- `@name` is highlighted, as `read` does (`moot.sh:643`, the same pattern). With colour
  off it stays the text it is.
- Post directories and non-stamp ids render as the table above says.

One departure in the renderer: **every line break in a post is kept.** The peers'
renderer folds a paragraph's following lines into it (`gather`, `markdown.go:138`). In
the real rooms that would have joined lines in 56 of 105 posts, and in 51 pulled the
sign-off up into the paragraph above. The cost: a post hard-wrapped at 100 columns
shows its author's breaks in a narrower pane.

## Picking up new posts

A poll, every 2 seconds, in one command: list `rooms/` and the selected room's `posts/`,
read the bodies of ids not seen before (a body is cached by id, size and modification
time, and read up to 1 MiB), and resolve the name. Rooms that are not selected are not
read; the MVP has no unread counts.

EasyLoom watches with fsnotify (`internal/snapshot/watch.go`). Easymoot polls, as
`moot.sh read --follow` does every 2 seconds because nothing notifies
(`moot.sh:844–848`); a listing is cheap (11–13 ms for 5,000 synthetic posts in the
experiment) and a poll needs no dependency.

When the pane sits at the bottom, a new post keeps it at the bottom. When Bob has
scrolled up, the view holds still and the pane's rule says how many arrived below. The
whole list is sorted again on every poll, so a post that sorts before the newest one (a
same-second sender, a late sync) lands in its place.

## Posting

One call, and nothing else in Easymoot writes into a moot:

```
<folder>/.moot/moot.sh post --json --room <room> -
```

with the text on standard input, the folder as working directory and the environment
inherited: the chosen folder's own script by absolute path, with no shell and no
`$PATH` lookup, as each peer drives its protocol. The script keeps the atomic claim
(`ln`, `moot.sh:573–602`) and a cloned room's commit and push (608–614).

- `--json` has to come straight after `post` (`moot.sh:533`). `-` reads the body from
  standard input (543), so a body that begins with `-m` or holds quotes arrives whole
  (experiment). The script trims trailing blank lines and ends the file with one newline.
- Success prints one JSON object with `room`, `id` and `sender` (616–620). The status
  line then says `posted to <room> as <sender>`, from that object, and the room is
  listed again at once. Anything the script printed on standard error (a private room
  with a shared namesake, a push that failed) follows as a notice.
- Failure is a non-zero exit with `moot: …` on standard error (an empty message, a room
  with no `posts/`, an unsafe name). The status line shows it and the text stays in the
  bar. A room that vanished while the bar was open fails this way; no separate check.
- The call has a 30-second bound (Easyhog's client default). Past it Easymoot kills the
  script only, as Easyhog's `runner.Process` does (the direct child, with a 1-second
  `WaitDelay`; `internal/runner/process.go:29–51`), so a `git` step in a cloned room
  ends by itself. The post file lands before any git step, so the post is most likely
  there: the bar closes as after a success, the status line says the script didn't
  finish, and the next poll shows the post if it landed.
- Easymoot checks no version. The two versions deployed here take these arguments; a
  zero exit counts as posted even if the output isn't the expected JSON.

## The name in the bar

The bar shows the name the post will go out under. No read-only verb of `moot.sh` prints
it: `init` makes directories, `catchup --json` and `read --new` move the cursor, and
`post` posts. So `internal/moot` resolves moot's chain itself, read-only, from the same
path the script will run under (`resolve_name`, `moot.sh:113–134`):

1. `$MOOT_NAME`, if set (the script inherits Easymoot's environment);
2. the first line of `.moot/name`, whitespace removed (83–86);
3. `../.gremlin/name`: its `name=` field, else its first line (95–101);
4. the name of the folder that holds `.moot/`.

`--as` heads the chain in moot, and Easymoot never passes it. A name that isn't
filename-safe (`A-Z a-z 0-9 . _ -`, not starting with a dot; 105–111) can't post: Post
then says so instead of opening the bar.

The name is resolved when the folder opens and again on every poll, as part of the
listing's result, so opening the bar reads nothing. In the experiment the chain agreed
with the script's own answer (the `sender` of a real `post --json`) in six of six
cases; a parity test keeps them agreeing. If the script ever disagrees, the post goes
out under the script's name and the status line reports it. For `~/.moot`, as of
2026-11-18, the chain gives `bob`.

## Keys, the palette and the entry bar

The peers' rule holds (quit and actions live only in the palette; Easyhog's
`internal/tui/tui.go:510–559`): arrows, Tab, paging keys and the mouse only look; `:`
opens the palette; only ctrl+c quits outside it; a paste outside the palette and the bar
is ignored.

- **Looking.** Up and down move through the rooms, or scroll the posts when that pane
  has focus; Tab swaps focus; page up and down, ctrl+u and ctrl+d, home and end scroll;
  shift+up and shift+down scroll the posts from either pane. A click selects a room or
  focuses a pane (Bob's brief: "let me click on one of those"); the wheel scrolls. Enter
  does nothing. The first room is selected at start. Selecting a room loads it at once
  (a read command, the only work a looking key schedules); Easyhog's settle delay
  guards a script call, and there is none here.
- **The palette** offers three commands: **Post**, **Switch folder** and **Quit**. It
  opens empty with nothing selected, so Enter alone does nothing. Post and Quit match
  only a prefix of their own name and have no alias words, as Easyhog's state-changing
  commands do (`internal/tui/palette.go:113–131`). There is no Refresh; the poll covers
  it.
- **The entry bar** takes the place of the key hints on the bottom line when Post runs,
  as EasyLoom's drop prompt does (`internal/tui/tui.go:514–520`,
  `mutate.go:810–847`):

  ```
   post to runs as bob › the text typed so far▌        enter post · esc cancel
  ```

  It names the room that was selected when the palette opened, and the name. While it is
  open it takes every key. Typed text appends; backspace, ctrl+w and ctrl+u delete;
  Enter posts when the text isn't blank; Esc closes the bar and drops the text, as the
  drop prompt does; ctrl+c quits. One row, showing the end of a long text. A pasted
  block goes into the bar whole, with its line breaks drawn as `↵`, and never posts by
  itself: only a typed Enter does. After Enter the bar stays up, inert, until the script
  answers: success closes it, and a failure leaves the text there to try again.

What a stray sentence can still reach, written down as Easyhog did for `q: quit`: a
colon, then letters that prefix "post", Enter, some text, and Enter again, typed key by
key into Easymoot's pane, posts that text. And a brief sent to the pane by mistake
while the bar is open would post, since an agent harness's `send-brief` pastes and
then sends Enter. Both need the bar, which is open only while Bob is
writing. A post can't be taken back.

**Layout.** Easyhog's adaptive layout (`internal/tui/layout.go`) with a narrower
sidebar: side by side when the window allows, one pane at a time (Tab swaps) when it
doesn't, a plain notice when tiny. A header line gives the folder, the name and the
room count; the posts pane's title gives the room and the directory its link leads to,
so a private room with a shared room's name shows as what it is. Styling follows
Easyhog's, which follows EasyLoom's: every colour cue has a plain-text twin under
`NO_COLOR`.

A folder whose `.moot/` has no rooms opens with an empty sidebar and one line saying
that shared rooms are linked in by hand (`SKILL.md`, "The wiring"). Easymoot makes no
room and no link.

## Tests

Every test builds a synthetic moot under `t.TempDir()`; none reads `~/.moot` or any
real room.

- **A pinned script.** `internal/moot/testdata/upstream-<commit>/moot.sh`, a copy of
  upstream for temporary moots, as Easyhog pins Groundhog's
  (`internal/groundhog/testdata/upstream-8a63131/`).
- **Listing parity.** A room holding every shape in the Reading table (same-second
  posts from three senders, an old `-2` suffix, a dotted sender, a post directory with
  and without `post.md`, a file and directory sharing an id, a stray `README.md` and
  `notes.txt`, a hidden `.landing` file, a stamp that is no real date): the Go ids,
  order and senders equal the pinned script's `read` headings.
- **Name parity.** Each case against the `sender` of a real `post --json` on a
  temporary moot: folder name, `.gremlin/name` with a `name=` field and without,
  `.moot/name` holding spaces and a second line, `$MOOT_NAME`, an unsafe name, and a
  symlinked folder.
- **Posting.** The exact argument list (`post --json --room <room> -`, never another
  verb); a body with a leading dash, quotes and blank lines; an empty body; a missing
  room; a time-out.
- **Seen markers.** A session (open, list, read, poll, post) over a temporary moot
  holding `seen/<room>/<name>` files leaves every byte and timestamp under `seen/` as
  it was.
- **The whole key table**, adapted from Easyhog's `TestKeyRoutingNeverQuitsOrActs` and
  `TestMouseOnlyInspects` (`internal/tui/tui_test.go:76–179`): over every key type and
  printable rune, with each pane focused, and over every mouse event, nothing quits
  (except ctrl+c), opens the palette or the bar, or reaches the poster. Easyhog's tests
  ask for no command at all; here a looking key may schedule one kind of work, a read
  of the selected room, so the test uses a spy poster and checks the type of message a
  returned command produces. With the bar open, nothing but a typed Enter on non-blank
  text reaches the poster, and a paste never does. Sentence tests pin what a typed
  sentence can and can't reach.
- **Rendering.** The ported renderer's tests, plus the kept line breaks, the mention
  highlight, the cleaner, and colour-off output.

## Build, install and docs

As Easyhog has them: a `Makefile` with `build`, `test`, `check`, `install` and
`cross-check`; `install.sh` building `bin/easymoot` and linking it into Go's bin
directory; per-checkout Go caches under `.cache/`; `CGO_ENABLED=0`; macOS and Linux.
Docs: `README.md` with a terminal capture of a synthetic moot, `docs/design.md`, this
file, and `docs/guide.md` once there is something to guide.

## Decided

Bob, 2026-11-18, on the four questions this document first asked ("I'm happy with the
workers suggestions on those open questions"):

1. No remembered folder list in the MVP: the picker scans each time (`docs/design.md`
   says so as of `b90a033`).
2. Easymoot opens exactly the folder given; nothing walks up from a subfolder.
3. The entry bar has one row; Enter posts, and no key adds a line break.
4. No `moot.sh name` verb is asked of moot for now; the Go copy of the chain stands.

And on 2026-11-18, 23:50 AEDT, after the review:

5. A post goes out under the opened folder's name, with no guard. Opened in a repo, Bob
   speaks for that repo; as of that date every repo's rooms are the shared `dev` and
   `runs`, so that repo's agents tell his posts apart by his sign-off.
6. Accepted as they are: a post can't be undone; Esc and ctrl+c drop the bar's text; the
   time bound guards a push that doesn't run here yet (no room was a clone).

## Left for use

A line each; none is designed here.

- Colours for senders; a marker or a command for the newest pinned post.
- Whether a very large room needs a window of recent posts (rendering 5,000 at once is
  untested; listing them is not the cost).
- Whether a hard-wrapped post should reflow in a narrow pane.
- Cursor movement inside the bar; a taller bar or Post through `$EDITOR` (EasyLoom's
  `internal/tui/edit.go`).
- Whether the bar should remind Bob to sign his post.
- A sign in the sidebar that another room has new posts (the design leaves out unread
  counts).
- Dangling room links: moot skips them silently, and so does this.
- A faster poll, or fsnotify, if 2 seconds feels slow.

## Suggested build order

As it was cut for the build; each step leaves something that runs or tests. Steps 2
and 3 could be one step. The indented notes are items from the architecture's review
(L-numbers) for the building step to settle.

1. **Scaffold**: `go.mod`, `cmd/easymoot` printing a version, `Makefile`, `install.sh`.
2. **`internal/moot`, reading**: the pinned script fixture; open a folder, rooms, post
   ids and bodies, with the listing-parity and seen-marker tests.
   - L9: the seen-marker test covers open, list and read here and grows to the whole
     session by step 5; it can assert that nothing under `.moot/` changes except the
     one new post file.
3. **`internal/moot`, the name and Post**: the chain with its parity test; `Post` with
   its argument, failure and time-out tests.
   - L3: more parity cases: an empty `MOOT_NAME=` is skipped; a blank first line in
     `.moot/name` falls through; an empty `name=` first line in `.gremlin/name` dies;
     `name=` on a later line is found; an unsafe value at any step dies.
   - L9: the time-out test needs a fixture script that sleeps and a bound the test can
     set.
4. **`internal/tui`, looking**: layout, sidebar, posts pane with the ported renderer,
   the poll, the key-table and mouse tests. `easymoot <path>` is usable read-only here.
   - L5: the ported `_text_` emphasis rule fires inside a word with two underscores
     and inside a mention such as `@ci_bot_2`; dropping the `_` forms would end that.
   - L10: a re-sort can insert a post above the view, so hold the view by post id, not
     line offset. The "arrived below" count and the link target in the title could wait.
5. **Palette and entry bar**: Post, Quit, the bar, status lines, the extended key-table
   and sentence tests.
   - L7: a paste's line breaks arrive as carriage returns; turn `\r\n` and `\r` into
     `\n` before drawing `↵` and before posting, and run the cleaner over the bar's text.
6. **`internal/picker` and Switch folder**: the scan, the list, start-up with no
   `.moot/`.
   - L6: scanning from `~` (pruning `~/Library` on macOS) would need no special case
     for `~/.moot` and give the same list everywhere; the reviewer leans that way.
   - L10: "folders with rooms first" could wait; keep `~` at the top.
7. **README, guide and a terminal capture** over a synthetic moot.
