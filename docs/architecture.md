# Easymoot architecture

How Easymoot is built, for the MVP in `docs/design.md` and its follow-ups. Drafted on
2026-11-18, before the build, by Claude Fable 5.2, and brought up to the build on
2026-11-19 (Claude Sonnet 5.5) after the first release and the changes that followed
it; recursive room discovery was added on 2026-11-20, when upstream references
were refreshed as of moot `ed13a2a`. Where this
text and the code differ, the code is the authority; the text cites the file or
function a claim rests on so a reader can check.

It starts from Easyhog's and EasyLoom's shapes and says where Easymoot departs. The MVP
stays simple: a question that use can answer sits under [Left for use](#left-for-use).

## What this rests on

The peers were read on 2026-11-18; moot was re-read at `ed13a2a` on 2026-11-20, and
the test pin moved to `ce10b46` on 2026-11-22 (the commit that adds `create`,
`archive`, `leave`, `dm`, `status`, archived rooms and link repair). Check a detail
again before building on it.

| Source | Commit |
|---|---|
| [moot](https://github.com/zealtv/moot) (`.moot/moot.sh`, `README.md`, `SKILL.md`) | `ed13a2a` read; `ce10b46` pinned |
| EasyLoom | `ca2f8f6` |
| Easyhog | `9d82857` |

"Experiment" below refers to the throwaway Go programs run against synthetic moots
on 2026-11-18. The listing and name parity tests were re-run against moot `ed13a2a`
on 2026-11-20. The design expects moots shaped like this:

- Rooms live with their keeper, as real folders under that participant's
  `.moot/rooms/`; other participants hear them through links. `join` writes relative
  targets as of moot `ed13a2a`, 2026-11-20. A folder may have no rooms at all.
- A name usually comes from the folder, with no `name` file or `.gremlin/name`.
- Posts are single `.md` files of prose with little Markdown, usually several long
  lines, with a sign-off line directly under the text. Rooms are small: hundreds of
  posts rather than thousands, each a few KB at most.

## Shape

```
cmd/easymoot/        arguments, start-up, the terminal, the line printed on exit
internal/moot/       one folder's .moot/, headless: rooms, posts, bodies, the name, post
internal/tui/        Bubble Tea shell: sidebar, posts pane, palette, entry bar
internal/acceptance/ test files only: one end-to-end session (see Tests)
```

Go 1.24 (`go 1.24.0`), Bubble Tea v1.3.10, Lip Gloss v1.1.0, `charmbracelet/x/term`
v0.2.1 and `charmbracelet/x/ansi` v0.10.1 (both peers' palettes import it), as the
peers' `go.mod` files have them. No other dependency. Module path
`github.com/zealtv/easymoot`.

`internal/moot` is the headless core. Easyhog splits a core from a script client
(`internal/core`, `internal/groundhog`); a moot keeps no state beyond its files and
every script call is one `moot.sh` verb, so one package holds both.

Two rules from the peers carry over unchanged:

- **I/O stays out of `Update` and `View`.** Listing, reading bodies, resolving the name
  and posting run in Bubble Tea commands and come back as messages; an answer for a room
  that is no longer selected is dropped (Easyhog, `docs/architecture.md`,
  `internal/tui`). The one exception is the exit line, which `main` asks for after the
  program has ended (see [Posting](#posting)).
- **Text from a moot is inert.** Room names, sender names and post bodies pass through
  one cleaner before they are drawn, which turns every control character, escape
  included, into a visible middle dot (Easyhog, `internal/tui/state.go:272`;
  here `internal/tui/clean.go`).

## Opening a folder

Easymoot works out of one folder's `.moot/` for the whole run, and has no folder picker.
Bob decided that on 2026-11-19 (`docs/design.md`); the first release, v0.1.0, had a
picker that scanned for folders and a palette command to switch, and both are gone.
The opt-in `--root` (see [Recursive rooms](#recursive-rooms---root))
adds rooms to the sidebar and doesn't change this: still one folder, one name, and
every `moot.sh` call runs in the opened folder.

- `easymoot [path]` calls `moot.Open(path)` (`cmd/easymoot/main.go`), with `.` when no
  path is given. `path` may also name the `.moot/` directory itself. A second path is an
  error.
- A folder with no `.moot/` is `moot.ErrNoMoot`, wrapped with the absolute folder:
  `easymoot: no .moot directory in <folder>`, on standard error, exit status 1, and
  nothing drawn. A typed path that does not exist reads the same.
- When standard input and output are not both terminals, `tui.Run` reads the moot once
  and prints one 80×24 frame instead of starting the interface.

Departures from EasyLoom:

1. **No walk up from the working directory** (EasyLoom: `internal/target/resolve.go:102`).
   The folder decides the posting name, so Easymoot opens exactly the folder it was
   given, rather than choosing an ancestor's moot and identity.
2. **The path is kept as typed, made absolute, with symlinks left alone** (EasyLoom
   resolves to the real path, `resolve.go:139`). `moot.sh` names the sender after the
   folder it was invoked through (`moot.sh:15`, `122`); in the experiment a post through
   a symlinked folder went out under the link's name.
3. **A match is a directory named `.moot`**, with or without a script. Reading needs no
   tool; where `moot.sh` is missing, Post says so when tried (`moot.ErrNoScript`).

## Reading a room

`internal/moot` reads the files directly and never runs the script to read. `moot.sh`'s
own listing is the authority, and the Go listing (`Rooms`, `List` in `moot.go`) copies it
rule for rule:

| Rule | `moot.sh` |
|---|---|
| A room is a directory under `rooms/` (links followed), not hidden, in byte order. A link that leads nowhere is not a room. | `subscribed_rooms`, 136–145; 13 |
| A post id is a visible directory's name, or a visible `*.md` file's name without `.md`. Other files are not posts. | `post_ids`, 195–208 |
| Ids are unique and sort bytewise (`LC_ALL=C`), oldest first, newest last. | 13, 207 |
| The body is `<id>/post.md` when the id is a directory, else `<id>.md`. A directory wins over a file of the same id. | `post_body_path`, 210–217 |
| An id shaped `<8 digits>-<6 digits>-<sender>` is a stamp: UTC time, and the sender is everything after the second dash. | `is_stamp_id`, 397–402; `render_post`, 443–448 |
| Any other id (a stray `README.md`) is listed under its own name, with no time. | 449–451 |
| A post directory lists its other files as `attached: a b`; one with no `post.md` says so. | 453–470 |

In the experiment the Go listing gave the same ids in the same order as `moot.sh read`,
over a room holding each of these shapes.

**Bodies.** `Moot.Body` (`body.go`) reads at most 1 MiB of a post (`MaxBody`; a longer
post shows a "cut" line) and caches the result by room and id, reusing it while the body
file's size and modification time stay the same (a post directory's own time too, for
its `attached:` line). The poll adds a second cache on top of this one, in the model:
a body that was read without error is not asked for again by id until the room is
selected afresh (`pollCmd` in `tui.go`). So a post that is edited in place after it was
first read keeps showing its first reading until then; see [Left for use](#left-for-use).

**Seen markers.** `internal/moot` has no code path that opens anything under `seen/`,
and Easymoot never runs `read --new`, which moves the cursor (`cursor_put`, 482–485;
`read_new`, 541–544). `post --json` writes a post and leaves `seen/` alone. Two of the
script's other verbs touch this folder's own cursor folders by name: `rooms` (link
repair) renames a cursor folder with its room's link, or removes it with an archived
link whose room has gone, and `leave` removes the folder of the link it removes
(`moot.sh` `cmd_leave`). A name changes or a folder goes; no
post id a cursor holds ever moves. The unread marks below are Easymoot's own and
unrelated to them.

## Rendering

The posts pane shows the selected room as one transcript, oldest at the top, newest at
the bottom, opened at the bottom, as `moot.sh read` prints it. `Model.transcript`
(`transcript.go`) lays it out: each post gets one heading line, then its body, and
`postGap` (2) blank lines sit between one post and the next heading, so each post reads
as its own block:

```
── alice ─────────────────────────────────────────────── 2026-08-29 21:00 ──
The nightly index rebuilt; it is 40% smaller.
— alice


── bob ───────────────────────────────────────────────── 2026-08-30 08:12 ──
…
```

- The heading (`heading` in `transcript.go`) puts the sender at the left and the time
  at the right, joined by the rule and right-aligned to the measure (below), or to the
  pane when that is narrower, so the time sits over the prose's right edge (Bob,
  2026-11-19). `moot.sh read` puts the time first (`moot.sh:443–448`); Easymoot keeps
  its time in Bob's local zone, with the seconds and the zone dropped. The filename
  stays UTC. A stamp that is no real date shows as written, marked UTC. An id that is
  no stamp shows alone, `── README ──`. A heading wider than the pane is cut.
- The body goes through a Markdown renderer ported from EasyLoom's as Easyhog ported it
  (`internal/tui/markdown.go` in all three: pure, text and widths in, lines out,
  readable with colour off).
- `@name` is highlighted, as `read` does (`moot.sh:388`, the same pattern). With colour
  off it stays the text it is.
- A post's body is drawn from the cleaned text; a post directory also gets its
  `attached:` line, and a directory with no `post.md` says so.

Two departures in the renderer:

- **Every line break in a post is kept.** The peers' renderer folds a paragraph's
  following lines into it (`gather`, `markdown.go:138`). For posts written as several
  long lines with a sign-off directly under the text, that joins lines meant to stand
  apart and pulls the sign-off up into the paragraph above. The cost: a post
  hard-wrapped at 100 columns shows its author's breaks, and a source line longer
  than the measure below wraps at it, leaving a short remnant.
- **The `_` forms of strong and emphasis are gone**; `**` and `*` remain. The `_` forms
  fired inside identifiers such as `read_new_ids` and inside a mention such as
  `@ci_bot_2`.

**The measure.** Bob asked on 2026-11-19 for posts that don't "spread across the whole
window". Prose wraps at `measure` (76 columns, a constant in `transcript.go`, chosen by
eye between 72 and 80), or at the pane's text width when that is narrower (the pane less
a one-column margin each side). Prose means paragraphs, `#` headings, list items,
quotes and rules. Left at the pane's full width, because cutting them short destroys
what they are:

- fenced code, its fence lines included;
- table rows: a plain line whose first non-blank character is `|` (the renderer parses
  no tables; rows stay as written);
- a plain line indented four or more spaces (`codeIndent`): indented code, a file tree.
  Spaces only, so a tab-indented line still wraps as prose.

In a pane narrower than such a line, it wraps to the pane. A single token too long for
its line, a long URL for example, goes on a line of its own and breaks only at the pane's
width, so a URL that fits the pane stays whole. Prose sits against the left edge; it is
not centred.

The held view needs no special case for the gap or the measure: `starts`, `blockLen`,
`settle` and `scrollTo` all derive from the transcript's line numbers. A resize across the
measure can shift a held view within its post, as any re-wrap does.

## Picking up new posts

The poll is a chain, not a timer. `Init` starts the first read, and each chained answer
(`applyPoll` in `tui.go`) starts a 2-second wait (`DefaultPollInterval`) that ends in the
next read. The interval therefore counts from the answer, and a slow read stretches it.
As of moot `ed13a2a`, 2026-11-20, `read` has no follow mode; `watch` waits for new
posts and exits. Easymoot keeps its own poll rather than watch the file system as
EasyLoom does with fsnotify (`internal/snapshot/watch.go`), which keeps a dependency out.

One poll, in one command (`pollCmd`):

1. resolve the name, list `rooms/`;
2. list the selected room's `posts/`;
3. read the body of each id the model doesn't already hold (see Bodies above);
4. work out the other rooms' tips for the unread marks (below);
5. read each post that arrived in another heard room, once, for a mention of the reader
   (below, "On the map").

Selecting a room, and the read after a post, are *unchained* reads of the selected room
alone: they list no other room and start no timer. Answers for a room that is no longer
the selected one are dropped, but a chained answer still starts the next wait, so the loop
never stops. If the selected room itself disappears, the neighbour at the same position is
selected and read.

The whole list is sorted again on every poll, so a post that sorts before the newest one
(a same-second sender, a late sync) lands in its place. The view is held by post id and
an offset into that post, not by line number, so such a post above the view moves
nothing. When the pane sits at the bottom, a new post keeps it at the bottom. When Bob has
scrolled up, the view holds still, and the pane's title says `scrolled up, end follows`
while the post count in it rises; the `end` key returns to the bottom.

**Unread marks.** A room other than the selected one that gets a post while Easymoot runs
shows `• ` and its name in bold cyan until Bob selects it (`roomRow` in `render.go`; the
`unread` style in `styles.go`). Bob decided this on 2026-11-19 (`docs/design.md`): a
mark, not a count, this session only, nothing stored. The mechanics, in `tui.go`:

- A *tip* (`roomTip`) is what one listing said of a room: its post count and its newest
  id. `Model.latest` holds the last tip of each room, `Model.seen` the tip each room had
  at launch or when it was last on show, and `Model.unread` the marks.
- `track` derives the marks from those two maps on every chained answer. The selected
  room never takes a mark and its `seen` follows its `latest`. A room met for the first
  time, at launch or because it was linked in while Easymoot runs, starts read. Another
  room is unread when its count is past its `seen` count *or* its newest id is past its
  `seen` id: the count catches a late post that sorts before the newest, the id catches
  one that replaces another. When posts are deleted, `seen` comes down with them, so a
  deleted post cannot leave a stuck mark. A room that cannot be listed keeps its old
  state.
- Selecting a room (`jump`, or the neighbour after a room goes) takes its tip as seen at
  once. A read outside the chain also marks what it shows as seen (`see`), so Bob's own
  post, or one he watched arrive, does not mark the room when he leaves it.
- The chained poll takes the selected room's tip from the listing it already has. For
  each other room it makes one `stat` of its `posts/` directory (`Moot.PostsModTime`) and
  lists the room again, names only and no bodies, only if that stamp differs from the one
  recorded at the last listing, or the stamp is less than `stampGrain` (2 seconds) older
  than that listing (a coarse file system can stamp two changes alike within a tick). If
  the `stat` fails, the room is listed every poll.
- Known gaps: one post deleted and another added between two polls, the new one
  sorting before the old newest, leaves count and newest alike and gives no mark; and a
  dangling link in `posts/` that starts to resolve does not change the directory's stamp,
  so its post shows when the directory next changes.
- In the one-pane layout, with only the posts pane showing, the marks can't be seen; Tab
  shows the sidebar.

**On the map** (since 2026-11-27, `glance-plain`; `glance.go`). Inside tmux Easymoot tells
tireless-watcher what waits for the reader, on its own pane, through `internal/twpane`
(the same file as EasyLoom's; it writes only changes, in one tmux call, under a lock, and
nothing after `Clear`, which `Run` calls on the way out):

- `@tw-idle`, from the unread marks (since 2026-11-28 it replaces `@tw-next`: tireless-watcher a8af009 retired that one, and the map adds no label): `3 new`, the sum of each heard room's listing
  count past its `seen` count (at least 1 a room). No rooms and no mentions (the wait
  has those): Bob, 2026-11-27, "easymoot probably doesn't need a status, maybe n new".
  The map draws it as written, with no label. Unset when nothing is unread.
- `@tw_status` `wait`, `@tw_status_text` (`@<name> ← <sender> · <room>`, the newest,
  with ` +N`: the mention first and short enough for a title bar) and `@tw_status_at` (when the poll first found that post), while an unread
  post counts as a mention of the opened folder's own resolved name, as moot.sh's
  `counts_for_me` reads it: an `@<name>` or `@all` outside code, or any post from the
  other side of a DM; never the reader's own post. A mention between other participants
  doesn't wait (the coordinator, settling it on 2026-11-27: "The point is \"someone needs
  Bob\""). Selecting the room clears it. A mention in the room on show never waits: it is
  read as it arrives, as for the unread marks. With a participant selected no room is on
  show, so the room behind it counts and waits like any other.
- "Heard" means kept or heard by the opened folder and not archived: a room found only by
  `--root` neither counts nor waits.
- The check (`checkMentions`) runs in the chained poll, for each heard room not on show
  whose tip is past `seen`. It takes the posts `tipsOf` listed this poll and reads the
  body of each post past `seen`'s newest that it hasn't checked before; a room `tipsOf`
  didn't list (the stamp gate kept it) keeps its earlier findings. A body that can't be
  read isn't recorded, so the next listing tries it again. So the poll reads another
  room's body once per arriving post, where before this it read none, and lists nothing
  more.

**The glance** (since 2026-11-27, `publish-glance`; `glance.go`). On the same pane Easymoot
publishes tireless-watcher's glance, the badge line and the list its map draws inside
Easymoot's own box (tireless-watcher's `docs/interface.md`, "Glance"), through
`internal/twglance` (the same file as EasyLoom's and Easyhog's: entries encoded as
`role:text`, cleaned, cut, written on change in one call, and cleared by `Run` on exit):

- `@tw-glance-far`, the badge line, marks and counts only so that it fits one far line:
  `:moot`, then `:•N` (the sidebar's unread mark; the unread posts, counted as
  for `@tw-idle`), `hold:@N` (the mentions that wait), `ok:●N` (listeners whose status is
  current) and `quiet:●?N` (listeners with no status or an old one), each left out at
  zero.
- `@tw-glance-mid`, the list: one row per listener, `ok` for the current ones, then
  `quiet` for the rest, each by name (the map draws the role's mark, `●` or `○`), as
  `<name> › <status>`, a head and a body (tireless-watcher's `docs/glance.md`, "A head
  and a body"): the map sets the names in a column and wraps each status under itself.
  Only the first ` › ` splits, so a status may hold one. A listener with no status is
  its name alone. One row a listener means the map's 16 rows hold 16 listeners, and its
  `+N` counts the rest. No posts.
- `@tw_status_text` (above) names a room `run-YYYYMMDD-<name>` as `<name>` (`short`): a
  run room's date says little on the map.
- A **listener** is a participant whose `.moot/watching` names a live pid and has no
  `watch.paused` (the sidebar's `●`). Its status is **current** when its stamp was under
  12 hours old at the poll (`currentFor`); a status with no stamp, or one stamped more
  than a minute ahead of the clock, is never current.
- So that the list is there with the participants out of sight, a poll reads presence
  whenever Easymoot runs inside tmux (`peopleOnShow`, on the glance's pane id), map or
  no map, not only while the participants show.
  With no walk there are no participants, and the glance is the badge line alone.

**Measured costs.** On a development Mac, 2026-11-19, under load, so roughly, with bodies
cached and synthetic moots; the figures are medians or ranges and will differ elsewhere.
A steady chained poll, with the stamp gate and before it (when every other room was listed
each time):

| Moot | With the gate | Before the gate |
|---|---|---|
| 10 rooms × 100 posts | 0.4 ms | 4–5 ms |
| 5 rooms × 1,000 posts | 4 ms | 20–25 ms |
| 20 rooms × 1,000 posts | 4 ms | 80–125 ms |

With the gate the poll costs about what the unchained read of the selected room alone
costs; a room changed in the last two seconds is still listed each poll until it ages.
The layout is the other cost: `applyPoll` calls `rebuild` on every answer, changed or not.
One room, at 120×40, an answer including that layout took about 5 ms at 100 posts, 55 ms at
1,000 and 240 ms at 5,000, while drawing a frame or handling a key stayed near 0.15 ms or
less. All of the file-system work runs inside a command, off the interface's thread.

**Open: how much the poll should cost.** Bob remarked on 2026-11-19 that 56 ms at 1,000
posts "sounds a bit heavy" and that it "doesn't need to be polling if it's not focused",
and asked for an architecture or design review of it with a strong model, not
immediately. This document doesn't answer it. Ideas on the table, none adopted: pause
polling while the terminal has no focus (Bubble Tea reports focus), skip the layout when
the ids and bodies are unchanged, lay out only the new posts. Two related behaviours stay
as built until then: a chained poll that started before a post landed can answer after
the read that follows the post, so the new post can vanish from the view until the next
poll, and the same race can lower the selected room's tip; and the model's body cache
(above).

## Posting

Posting is one of the script calls Easymoot makes (the others are the room actions and
link repair, see [Room actions](#room-actions)); nothing else in Easymoot writes into a
moot:

```
<folder>/.moot/moot.sh post --json --room <room> -
```

with the text on standard input, the folder as working directory and the environment
inherited: the folder's own script by absolute path, with no shell and no `$PATH`
lookup, as each peer drives its protocol (`Moot.Post`, `internal/moot/post.go`). The
script keeps the atomic claim (`ln`, `moot.sh:331–362`). As of moot `ed13a2a`,
2026-11-20, it runs no git: syncing a cloned room belongs to a separate job outside moot.

- `--json` is a `post` option (`moot.sh:306`); Easymoot puts it straight after the verb.
  `-` reads the body from standard input (313), so a body that begins with `-m` or holds
  quotes arrives whole (experiment). The script trims trailing blank lines and ends the
  file with one newline.
- **Success** prints one JSON object with `room`, `id` and `sender` (364–368). The bar
  closes and the status line says `posted to <room> as <sender>`, from that object; any
  lines the folder's script printed on standard error follow as notices. Then, in
  `applyPost` (`bar.go`):
  - **the posts pane returns to the end**, as the `end` key does (`followAfterPost`), if
    the room the reply names is the one on show, so the new post is in view when the read
    lands. A reply naming another room leaves the view alone. Bob asked for this on
    2026-11-19;
  - the room is read again at once, outside the poll chain (`readAfterPost`).
- **Failure** is a non-zero exit with `moot: …` on standard error (an empty message, a
  room with no `posts/`, an unsafe name). The status line shows it, the text stays in
  the bar, and the view is left as it was. A room that vanished while the bar was open
  fails this way; no separate check. A script that exits 0 but leaves a child holding its
  output past one second also reports as a failure; see [Left for use](#left-for-use).
- **The 30-second bound** is Easyhog's client default (`DefaultPostTimeout`). Past it
  Easymoot kills the script only, as Easyhog's `runner.Process` does (the direct child,
  with a 1-second `WaitDelay`; `internal/runner/process.go:29–51`); any descendant ends
  by itself. The post may already have landed: the bar closes, the status line says the
  script didn't finish, the view is left alone, and the room is read at once and again
  by the next poll.
- Easymoot checks no version. The pinned script takes these arguments; a
  zero exit counts as posted even if the output isn't the expected JSON.

**Ctrl+c while posting.** Bob decided on 2026-11-19 that ctrl+c must not post. While the
bar is busy (Enter pressed, no answer yet) the bar takes no key but ctrl+c, and
`stopPost` (`bar.go`) does this:

1. cancel the post's context, which kills the script (`exec.CommandContext` sends
   SIGKILL to the direct child; the 1-second `WaitDelay` bounds the wait for its pipes);
2. show `stopping…` and start a `quitBound` (3 seconds) timer;
3. quit when the answer arrives (whatever it is), when the timer fires, or at a second
   ctrl+c, which quits at once.

With nothing in flight (no bar, the bar open and idle) ctrl+c quits at once. A stale timer
does nothing unless a stop is under way. A cancel that comes before the script has started
makes `exec` refuse to start it, so nothing is posted.

What a kill can leave, from the pinned script's `cmd_post`, which has no `trap`: it reads
standard input, resolves the name, writes the text to a hidden
`.<stamp>-<sender>.<pid>.landing` file in `posts/`, links that to the post's final name,
removes the hidden file, then prints the JSON. Killed before the write, nothing is left.
Between the write and the link, a hidden `.landing` file and no post. Between the link
and the removal, the post and a hidden file.
After the removal, the post stands. Readers skip hidden names (`post_ids` globs `posts/*`;
`List` skips names starting with a dot), so a leftover is invisible, and Easymoot never
deletes anything from a room, so it stays. The window before the link is microseconds
wide.

**The exit line.** Bob decided on 2026-11-19 that Easymoot prints a line after ctrl+c
has stopped a post. The first ctrl+c records an `exitNote` (`internal/tui/exit.go`) in the
model; the answer completes it; `Run` returns `Model.ExitLine()` to `main`, which prints it
on standard error once the terminal is restored. Any other way of quitting prints nothing.
The lines:

```
easymoot: the post had already gone out to dev (20261119-101500-ada)
easymoot: stopped; nothing was posted to dev
easymoot: stopped waiting; the post to dev may or may not have gone out
```

The first follows a success answer, or a stop whose listing shows a post from the bar's
name stamped at or after the second Enter was pressed. The second follows a stop whose
listing shows none. The third follows a second ctrl+c, the timer passing, or the poster's
own time-out. For a stop, `ExitLine` lists the room once (`Source.List`) and writes
nothing. Two limits, chosen for simplicity: a post of the same name from another process
during the few seconds of the stop reads as ours, and a script failure that arrives during
the stop takes the listing path, so a script that failed after the post landed still says
the post went out.

## The name in the bar

The bar shows the name the post will go out under. No read-only verb of `moot.sh` prints
it: `read --new` moves the cursor, and `post --json` reports the sender only after
posting. So `internal/moot` resolves moot's chain itself, read-only, from the same
path the script will run under (`resolve_name`, `moot.sh:109–130`; `name.go`):

1. `$MOOT_NAME`, if set (the script inherits Easymoot's environment);
2. the first line of `.moot/name`, whitespace removed (79–82);
3. `../.gremlin/name`: its `name=` field, else its first line (91–97);
4. the name of the folder that holds `.moot/`.

`--as` heads the chain in moot, and Easymoot never passes it. A name that isn't
filename-safe (`A-Z a-z 0-9 . _ -`, not starting with a dot; 101–107) can't post: Post
then says so instead of opening the bar.

The name is resolved when the folder opens and again on every poll, as part of the poll's
result, so opening the bar reads nothing. In the experiment the chain agreed with the
script's own answer (the `sender` of a real `post --json`) in six of six cases; a parity
test keeps them agreeing. If the script ever disagrees, the post goes out under the
script's name and the status line reports it. For `~/work/me/.moot`, with no overrides,
the chain gives `me`.

## Keys, the palette and the entry bar

The peers' rule holds (quit and actions live only in the palette; Easyhog's
`internal/tui/tui.go:510–559`; here `updateKey` in `tui.go` is the whole key table):
arrows, Tab, paging keys and the mouse only look; `>` opens the palette (`:` until 2026-11-27,
when the easy apps and tireless-watcher moved to `>` together, with no alias); only ctrl+c quits
outside it; a paste outside the palette and the bar is ignored.

- **Looking.** Up and down move through the sidebar's rooms and participants, or scroll
  the posts when that pane has focus; Tab swaps focus; page up and down, ctrl+u and
  ctrl+d, home and end scroll; shift+up and shift+down scroll the posts from either pane;
  shift+left and shift+right resize the sidebar. A click selects a room or a participant,
  or focuses a pane (Bob's brief: "let me click on one of those"); a press on the divider
  column starts a drag that resizes the sidebar; the wheel scrolls. Enter
  does nothing. The first room is selected at start. Selecting a room loads it at once
  (a read command, the only work a looking key schedules); Easyhog's settle delay
  guards a script call, and there is none here.
- **The palette** is one table, `allCommands` (`internal/tui/palette.go`), each row with
  the test that says when it is listed (`Model.commands`; what is listed when is in
  `docs/design.md`). It opens empty with nothing selected, so Enter alone does nothing.
  The commands that act match only a prefix of their own name and have no alias words, as
  Easyhog's state-changing commands do; the two that only look (Show and Hide archived,
  Pinned post) match as Easyhog's toggles do, by scattered letters too. Refresh walks for
  rooms at once; the regular poll keeps checking posts. An Esc fused with the next key
  (below) is appended to the query as that key; the palette still needs an Enter.
- **The entry bar** takes the place of the key hints on the bottom line when Post runs,
  as EasyLoom's drop prompt does (`internal/tui/tui.go:514–520`,
  `mutate.go:810–847`):

  ```
   post to runs as bob › the text typed so far▌        enter post · esc cancel
  ```

  It names the room that was selected when the palette opened, and the name. While it is
  open it takes every key; the mouse does nothing. Typed text appends; backspace, ctrl+w
  and ctrl+u delete; Enter posts when the text isn't blank; Esc closes the bar and drops
  the text, as the drop prompt does; ctrl+c quits, or stops a post in flight (above). One
  row, showing the end of a long text. After Enter the bar stays up, inert, until the
  script answers: success closes it, and a failure leaves the text there to try again.

  Two rules come from the byte stream a terminal sends, which key-by-key tests cannot
  see:

  - **A fused Esc.** A terminal sends Alt+key as Esc followed by the key, and tmux (within
    its `escape-time`), ssh and Option-as-Meta deliver an Esc and the next key together as
    one Alt-modified key. In the bar, an Alt-modified key acts as Esc: the bar closes
    and the text is dropped. Otherwise the Esc would be lost, the draft would stay, and a
    later Enter would post it. The cost: Option+arrow drops a draft instead of moving
    in it.
  - **Paste.** A bracketed paste goes into the bar whole, its line breaks (`\r\n` or
    `\r`, which terminals send) turned into `\n` and drawn as `↵`, and never posts by
    itself: only a typed Enter does. A paste sent *without* brackets (bracketed paste
    turned off, tmux's `paste-buffer` without `-p`, tmux's Alt+right-click *Paste*) arrives
    as typing, each line break as an Enter, so it posts at its first line break. The model
    cannot tell it from typing, and a timing guess would be the kind of safeguard the MVP
    leaves out; the README says so.

What a stray sentence can still reach, written down as Easyhog did for `q: quit`: a
`>`, then letters that prefix "post", Enter, some text, and Enter again, typed key by
key into Easymoot's pane, posts that text. Since the key became `>`, a shell line such as
`foo > q` typed into the pane and ended with Enter quits Easymoot (`>` opens the palette,
`q` is a prefix of Quit); before, only a colon did that. And a brief sent to the pane by mistake
while the bar is open would post, since an agent harness pastes a brief and then sends
Enter. Both need the bar, which is open only while Bob is writing. A post can't be taken
back.

**Layout.** Easyhog's adaptive layout (`internal/tui/layout.go`) with a narrower
sidebar (a quarter of the window, 18 to 32 columns, until a resize sets `Model.sideWidth`
for the session, clamped by `sideBounds` to 12 columns and half the window; the drag
follows `tea.MouseActionMotion`, which `WithMouseCellMotion` reports while a button is
held): side by side from 64 columns, one
pane at a time (Tab swaps) when narrower, a plain notice when under 20 columns or 3 rows.
A header line gives the room count, the name a post would go out under and the folder;
the posts pane's title gives the room, its post count and whether the view is scrolled up.
The footer has a status line (from 10 rows) above the key hints, and the bar takes the
hints' row. Styling follows Easyhog's, which follows EasyLoom's: every colour cue has a
plain-text twin under `NO_COLOR`, which is why an unread room carries a bullet as well as
its colour.

A folder whose `.moot/` has no rooms opens with an empty sidebar and one line saying
that shared rooms are linked in by hand (`SKILL.md`, "The wiring"). Easymoot makes no
room and no link.

## Tests

Every test builds a synthetic moot under `t.TempDir()`; none reads any real room.
The acceptance session points `HOME` at a synthetic home, so it builds the
binary first: Go writes telemetry into whatever home it finds.

- **A pinned script.** `internal/moot/testdata/upstream-<commit>/moot.sh` (now
  `upstream-ce10b46`, with moot's README beside it as `MOOT-README.md`), a copy of
  upstream for temporary moots, as Easyhog pins Groundhog's
  (`internal/groundhog/testdata/upstream-8a63131/`). The directory's own `README.md`
  names the source repo, the commit and how to update the pin.
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
- **Ctrl+c while posting.** Model-level tests of `stopPost` (the context is cancelled,
  keys stay inert, a second ctrl+c quits, the timer quits, a landed answer still quits),
  and one through the pinned script behind a wrapper that holds on a pipe: ctrl+c while
  it holds leaves the room byte-for-byte as it was. `cmd/easymoot/exit_test.go` drives the
  real program over pipes (`forceInteractive`) and pins each exit line, and the silence of
  every other quit. The windows after the link, in which a `.landing` file survives, are
  too narrow to hit on purpose and are untested.
- **Seen markers.** A session (open, list, read, poll, post) over a temporary moot
  holding `seen/<room>/<name>` files leaves every byte and timestamp under `seen/` as
  it was.
- **The whole key table** (`TestKeyRoutingNeverQuitsOrActs` and `TestMouseOnlyLooks` in
  `internal/tui/tui_test.go`, adapted from Easyhog's tests of the same names and kind): over every key type and
  printable rune, with each pane focused, and over every mouse event, nothing quits
  (except ctrl+c), opens the palette or the bar, or reaches the poster. Easyhog's tests
  ask for no command at all; here a looking key may schedule one kind of work, a read of
  the selected room (which may list other rooms by name and `stat` them, and read no body
  of theirs; a chained poll reads a newly arrived post of theirs once, for mentions), so the test uses a spy poster and checks the type of message a returned
  command produces. With the bar open, nothing but a typed Enter on non-blank text
  reaches the poster, a paste never does, and an Alt-modified key closes the bar. Sentence
  tests pin what a typed sentence can and can't reach.
- **Rendering.** The ported renderer's tests, plus the kept line breaks, the mention
  highlight, the cleaner, colour-off output, the two-line gap, the measure in a wide and a
  narrow pane, code, table rows and indented lines keeping the pane width, and a long
  token breaking at the pane.
- **Unread marks.** No marks at launch; a post in an unselected room marks it and
  selecting clears it; the selected room never takes one; a deleted post leaves none; a
  room that goes and comes back starts read; Bob's own post, and a post he watched
  arrive, don't mark the room he leaves; a post that doesn't sort last still marks; an
  unchanged room is not listed again, a changed one is, and one changed within the last
  two seconds is listed each poll.
- **Scroll to the end.** After a success the view follows the end and shows the new post,
  through the spy and through the pinned script; a failure or time-out leaves the view; a
  reply naming another room leaves it too.
- **README captures.** `internal/tui/readme_test.go` renders the README's terminal
  captures from the model over a synthetic moot and fails if the README's blocks differ.
- **Acceptance** (`make acceptance`; `EASYMOOT_ACCEPTANCE=1` turns it on). One session
  over a synthetic home of several folders' moots, each with the pinned script: the real
  `internal/moot` under the model, driven through `Update`, then the built binary run over
  the same home. It checks, among other things, that a post made in one folder shows in
  another folder's linked copy of the shared room, with that other folder's name in the
  bar, and that the first folder's private room does not; that no `.moot/` is an error naming the folder; that every file
  under every `seen/` is byte-identical with its time unchanged; and that the only new
  paths in the home are the posts the session wrote.

## Build, install and docs

As Easyhog has them: a `Makefile` with `build`, `test`, `check`, `install`,
`cross-check` and `acceptance`; `install.sh` building `bin/easymoot` and linking it into
Go's bin directory; per-checkout Go caches under `.cache/`; `CGO_ENABLED=0`; macOS and
Linux. Every build path stamps `vX.Y.Z` from the root `VERSION` file, as
`go install …@vX.Y.Z` does. Docs: `README.md` with a terminal capture of a synthetic moot,
`docs/design.md`, this file, and `docs/guide.md` once there is something to guide.

## Recursive rooms (`--root`)

Built on 2026-11-20 to the design in `docs/design.md`, "Rooms below a root".

**Arguments.** `parseFlags` in `cmd/easymoot/main.go` accepts `--root <dir>` and
`--depth <n>` before or after the opened folder. Depth defaults to 4; zero searches
only the root itself, and negative or non-numeric values are errors. With no `--root`,
`defaultRoot` takes the opened folder's parent when a folder beside it holds a `.moot/`;
otherwise no walk runs, and `moot.Open`, `Moot.Rooms` and the flat sidebar keep their
existing behavior.
The opened folder still supplies the name and the only script used for posting.

**The exception to "copies `moot.sh` rule for rule".** `Moot.Rooms` continues to
copy `subscribed_rooms`: a link leading to a directory is a room. Recursive
`moot.Discover` (`internal/moot/discover.go`) instead finds real room directories
under each keeper's `.moot/rooms/`, using `Lstat`. It never adds the links found
in other folders. It independently resolves the opened folder's own rooms and
hearing links with `EvalSymlinks`, adding heard rooms even outside the root or
beyond the depth limit. Dangling links are omitted. A room linked from several
folders shows once under its keeper. Older links to arbitrary room stores remain
readable, with the target's parent as the keeper header.

**Walking.** `Discover` uses `filepath.WalkDir`, never descends through a directory
symlink. Before starting the TUI, `run` resolves the root the user typed with
`EvalSymlinks`. Missing roots and non-directories exit with a clear `--root` error
and no frame. `Discover` receives the resolved directory. It skips unreadable
subtrees, `node_modules`, `vendor`, hidden folders (including `.git` and `.cache`),
and worktrees whose `.git` is a regular file. The `.moot` and `rooms` directories
are inspected directly from their parent, with `Lstat`, rather than descended
into. The opened folder's own hearing is included independently of these rules.
Keepers and heard room targets are matched with `os.SameFile`, so differing path
case on case-insensitive disks cannot duplicate them or lose posting access. One
path spelling represents each keeper. Keepers sort by path, with the opened
folder first; rooms sort by name.
The depth default and walk shape follow EasyLoom's `internal/target/scan.go`.

**Identity and reading.** `moot.Room` adds `Keeper` and `PostName`. `Room.Key` is
keeper plus name in recursive mode, and remains just the local name in flat
mode. Selection, post reads and session-only unread maps use that key, keeping
same-named rooms distinct. A `moot.Discovery` is an immutable room set with readers
for the resolved room paths. `discoveredSource` (`internal/tui/discovery.go`) routes
`List`, `Body` and `PostsModTime` through those readers while delegating identity
to the opened source. Reading never runs a script or touches seen markers.

**The sidebar.** `Model.sidebar` projects the rooms into keeper headers and
indented room rows. `Model.sel` still indexes rooms, so arrow keys skip headers.
`sideTop`, `sideIndex`, mouse hit testing and `sidePane` share the displayed row
projection. Headers show the keeper's folder name. `roomRow` makes read-only rooms
italic, retaining the session unread bullet. With styles disabled the label is
plain; there is no extra decoration. `openBar` refuses Post on these rooms with a
short status line. For a heard room, the bar captures `PostName`, the opened
folder's local link name. Posting still calls that folder's own `moot.sh`.

**Refresh.** `Model.poll` supplies an immutable discovery request to `pollCmd`.
The first command walks; `Update` schedules another on every 15th poll tick
(about 30 seconds), and the palette's `:refresh` schedules one immediately.
All walking runs inside the command, never in `Update` or `View`. `applyPoll`
retains the existing room slice and discovery when the sorted room set is
unchanged, including hearing names. A changed tree keeps selection by room key,
or chooses the nearest surviving neighbor if the selected room disappeared.
A discovery result can update the tree after selection moved, without replacing
that selection's posts. A read using an older discovery cannot restore its tree.
Walk requests also carry a sequence number; an answer older than the newest
successful walk applied
is ignored, while a chained answer still schedules the next poll.
No file-system watcher or configuration file is needed.

## Room kinds and the archived view

Built on 2026-11-22 to the design in `docs/design.md`, "Room kinds".

- **Kinds.** `moot.Room` gains `Kind` (`internal/moot/moot.go`): `Standing` (`Kept`, the
  zero value; `Heard`; `Elsewhere`, for a room found below `--root` that the opened
  folder neither keeps nor hears) and the booleans `Archived` and `DM`.
  `moot.KindOf(folder, standing)` reads the name only. `Moot.Rooms` takes the standing from
  whether the entry is a link, and the name from the folder a link leads to; `Discover`
  takes the name from the room folder. An archived room gets an empty `PostName`, the path
  a room the opened folder doesn't hear already takes, so Post's refusal and the italics
  need no new case. `Room.Name` stays the real folder name; the suffix drops only when drawing.
- **The projection** (`internal/tui/kinds.go`). `Model.all` holds every room the last
  listing found and `Model.rooms` the ones on show in sidebar order. `arrange` drops archived
  rooms unless shown and sorts stably by group rank; `direct` goes by the newest post id of
  each DM, from the unread tracker's tips (`Model.latest`). `Model.sidebar` adds a header
  where a group starts; `sel` indexes `rooms`, so the arrows skip headers. `withRooms`
  recomputes `rooms` from `all` and keeps the selection by key.
- **Hidden means not polled.** `pollCmd` takes `show` and runs `arrange` before it chooses
  the first room or builds the unread tips, so a hidden archived room is never listed,
  stat'd or read; `track` never marks an archived room. Toggling happens in `Update` on `all`
  with no read. When the open room is archived and gets hidden, `takeNeighbour` selects
  its neighbour, as it does for a room that disappears.
- **Labels.** `Model.label(room)` is what the sidebar and pane title call a room, from the
  room and `Model.ownName`, which `pollCmd` fetches with `OwnName` (`internal/moot/name.go`;
  `Name` without `$MOOT_NAME`), so `View` reads no clock or file. `dmLabel` takes the other
  half off the folder's own name or, for a room kept elsewhere, its keeper's.
- **Pinned post.** `Model.pinned` finds the newest post whose sender is `pinned`; the pane
  title shows its date, and Pinned post calls `scrollTo` on its line, so the held view and
  `End` do the rest. The one-pane layout also focuses the posts.

## Room actions

Built on 2026-11-22 to the design in `docs/design.md`, "Room actions".

- **The runner.** `Moot.Run(ctx, args)` (`internal/moot/run.go`) and `Post` both call
  `execScript` (`post.go`): the folder's own script, no shell, one bound, exit status plus
  the stdout and stderr lines. `Run` returns them as a `RunResult`; a non-zero exit is a
  `*ScriptError` whose message is the script's own `moot: ` lines (an older script's usage
  text rides on stderr beside its "unknown command", and is left out). `Source` gains `Run`.
- **The verbs** (`internal/tui/actions.go`). `canArchive`, `canJoin` and `canLeave` read
  the selected room's `Kind` and are the palette's `when` tests. A command opens a `bar`
  in `barConfirm` or `barCreate` mode (`bar.go`), which fixes the verb, its target and its
  label; a typed Enter in either calls `startAction`, which sets `Model.acting` and
  returns a command that answers with an `actionMsg`. `applyAction` writes the status line
  and starts a read. Only that command and `postCmd` reach a script.
- **Selecting after.** The read after an action is `pollAs` with `pollOpts{repair, pick}`:
  `repair` runs `rooms` first, on the original source, so the listing sees mended links
  (the open and Refresh polls do too); `pick` names a room to select from those listed,
  which `applyPoll` takes in place of the one asked for. A room that left the list is
  handled as any vanished room is. The poll's `repairs` lines reach the status line in
  `noteRepairs`.

## Participants

Built on 2026-11-22 to the design in `docs/design.md`, "Participants".

- **Readers** (`internal/moot/presence.go`). `ReadPresence(folder)` is a plain function, no
  `Source` method: the name from `OwnName`'s chain (the folder's name if that fails), line
  one of `.moot/status` split by `statusLine` into text and stamp, and `livePid` (signal 0
  to `watching`'s `pid=`; `EPERM` counts as alive) with `watch.paused`. `Discover` also
  returns `Participants`, the opened folder first, then each walked keeper.
- **The poll** (`internal/tui/participants.go`). `pollOpts.people`, set by `peopleOnShow`
  (a participant selected, the divider within the sidebar's window, or inside tmux, for
  the glance), makes `pollCmd`
  call `peopleOf`, which reads each folder and turns stamps into age strings at
  `time.Now()`; `pollMsg.people` carries the finished rows, and `applyPoll` keeps the
  selection by folder (`Model.person`).
- **The sidebar** (moved there on 2026-11-23). `sidebar()`, the one projection that
  drawing, `settle`, the arrows and the mouse share, appends a `divider` row and one
  `person` row per participant after the rooms whenever there is a walk; a moot with no
  rooms puts its notice lines in as `line` rows. `selectedRow` finds the selection, a
  room's row or, with `Model.onPerson`, the participant's; `arrow` steps to the next row
  that is either, so `jump` selects a room and `pickPerson` a participant. The room left
  keeps `Model.room` and goes on being polled; `track` may mark it unread meanwhile, and
  `jump` back to it shows the posts it kept. `body` draws `personPane` (the strip) in
  place of `postsPane` while `onPerson`, and `commands` lists only DM \<name\>, Refresh
  and Quit.

## DMs

Built on 2026-11-23 to the design in `docs/design.md`, "DMs".

- **The bar.** `openDM` opens a post's `bar` with `dm` set to the selected participant's
  folder and `room` empty; `openBar` and `openDM` share `openPostBar`, which checks the
  name. `bar.to()` names the target in notices (`@name` for a DM).
- **The post command.** A typed Enter calls `postCmd` as for any post; with `dm` set it
  first runs `Source.Run(ctx, dm <folder>)` under the same context, reads the room from
  its one line (`dmRoom`), then calls `Post`. So a DM has no in-flight state of its own:
  `postMsg` (with `dm`), `applyPost` and `stopPost` serve both. On success `applyPost`
  clears `onPerson` and lists again with `pollOpts.pick` (`pickOwn`: kept or heard),
  which selects the DM.

## Decided

Bob, 2026-11-18, on the four questions this document first asked ("I'm happy with the
workers suggestions on those open questions"):

1. No remembered folder list in the MVP. (Moot since 2026-11-19: no picker, item 7.)
2. Easymoot opens exactly the folder given; nothing walks up from a subfolder.
3. The entry bar has one row; Enter posts, and no key adds a line break.
4. No `moot.sh name` verb is asked of moot for now; the Go copy of the chain stands.

And on 2026-11-18, 23:50 AEDT, after a review of the first draft:

5. A post goes out under the opened folder's name, with no guard. Opened in a repo, Bob
   speaks for that repo; as of that date every repo's rooms are the shared `dev` and
   `runs`, so that repo's agents tell his posts apart by his sign-off.
6. Accepted as they are: a post can't be undone; Esc drops the bar's text; the time bound
   guards a push that doesn't run here yet (as of that date no room was a clone). Ctrl+c
   no longer drops the text of a post in flight: see item 9.

Bob, 2026-11-19, after trying the build and after the first release (his words are in
`docs/design.md`):

7. The folder picker and the Switch folder command are dropped; a folder with no `.moot/`
   is an error.
8. After a post goes out, the posts pane scrolls to the end.
9. Ctrl+c while posting must not post: it stops the script, and the program prints a line
   about what happened to the post.
10. Posts sit two blank lines apart, and their prose is kept to a readable width (76
    columns, set by eye).
11. Rooms with unread posts get a mark, for the session only. He passed over reading the
    seen markers to decide it. Its style (bold cyan with a bullet, also with colour off),
    and rooms linked in while running starting unmarked, are "fine for now".

## Left for use

A line each; none is designed here.

- Colours for senders; a marker or a command for the newest pinned post.
- How much the poll should cost, and whether it should pause when the terminal has no
  focus (see [Picking up new posts](#picking-up-new-posts)); whether a very large room
  needs a window of recent posts (rendering 5,000 at once is untested; listing them is
  not the cost).
- Whether a hard-wrapped post should reflow in a narrow pane or under the measure.
- Whether a tab-indented line should count as indented code (it wraps as prose now).
- Cursor movement inside the bar; a taller bar or Post through `$EDITOR` (EasyLoom's
  `internal/tui/edit.go`).
- Whether the bar should remind Bob to sign his post.
- Unread marks that outlast a session, or counts; seeing a mark when only the posts pane
  shows.
- A post written in pieces by hand (a post directory made first and filled later) can
  keep showing its first reading until the room is selected again. `moot.sh post` writes
  atomically, so this needs a hand-made post.
- A post that stops reporting success because a child of the script holds its output past
  the 1-second wait reads as a failure, and the text stays in the bar, where another Enter
  would post it twice. Easyhog treats that case as success (`internal/groundhog/client.go`).
  The pinned script starts no long-lived child after posting as of moot `ed13a2a`,
  2026-11-20, so this concerns other deployed scripts.
- A hidden `.landing` file left by a killed post: Easymoot leaves it, as it never deletes
  from a room.
- Dangling room links are skipped silently in both modes. Join links a room kept in
  another folder; a dangling link stays for `moot.sh leave` or the keeper.
- A faster poll, or fsnotify, if 2 seconds feels slow.

## How it was built

The MVP was built in the order this document first cut it: scaffold (`go.mod`,
`cmd/easymoot`, `Makefile`, `install.sh`); `internal/moot` reading, with the listing-parity
and seen-marker tests; the name and Post, with their parity, argument and time-out tests;
the shell, looking (layout, sidebar, posts pane, the poll, the key and mouse tests); the
palette and the entry bar; a folder picker; the README with a capture; and an end-to-end
acceptance session. The picker step was undone on 2026-11-19. The changes since, described
above, came from Bob's feedback on the build and from reviews of it, not from a plan made
in advance.
