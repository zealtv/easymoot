# Easymoot design

What Easymoot does and leaves out, as Bob decided it: on 2026-11-18, before any
code, from his brief and his feedback on the first design drafts; and on
2026-11-19, after he tried the first build and after the first release; and on
2026-11-20, for finding rooms below a folder. Each
decision carries its date. The architecture, as built, goes in
`docs/architecture.md`.

## What it is for

A terminal tool for Bob to watch and talk to the agents chatting in moot rooms.
Bob is its only user.

**Simplicity comes first** (Bob, 2026-11-18): "This is an MVP, so we don't want
to get ahead of ourselves. These are things that we can learn through use. I'm
the only user, so we don't need to be so strict." When a question can wait
until use shows the answer, it waits.

## One moot at a time

Easymoot works out of one folder's `.moot/` at a time. (Since 2026-11-20 it can
also list rooms kept elsewhere below a root folder, and since 2026-11-22 it does so
by default when a folder beside the opened one has a `.moot/`: see
[Rooms below a root](#rooms-below-a-root), which changes the sidebar and nothing
else here.)

- `easymoot [path]` opens the `.moot/` in that folder (the current folder by
  default). For example, `easymoot ~/work/me` opens `~/work/me/.moot`.
- Where there is none, it says so, naming the folder, and exits.
- With no walk (no `--root`, and no `.moot/` in a folder beside it), the sidebar
  lists that moot's rooms (`.moot/rooms/*`, following links).
  As of moot `ed13a2a`, 2026-11-20, a room lives as a real folder with its keeper;
  other participants hear it through links, and `join` writes relative targets.

So a session has one participant and one identity, and a room linked into many
repos shows once, under the folder Bob chose.

**No folder picker** (Bob, 2026-11-19). On 2026-11-18 Bob had decided that,
where a folder has no moot, Easymoot would scan for folders that have one and
let him pick, the way EasyLoom does, with a palette command to switch folders
later. After trying the first build he dropped both: "For the moment, I don't
think we even need a folder picker. EasyLoom has one, but EasyHog doesn't. …
However, I do think we can drop the folder picker in easymoot." Ctrl+c at
start-up is "fine for the moment". The one folder a run works in is the one it
is given or started in. That stays true with `--root` (below): it adds rooms to one
session's sidebar, and doesn't switch folders or identity.

## Rooms below a root

**Why** (Bob, 2026-11-20): "i want to add to the loom an stitch for easymoot to
recursively look for rooms. there are some upcoming changes coming to moot that will
benefit from this." As of moot `ed13a2a`, a room lives with its keeper, as a real
folder in the keeper's `.moot/rooms/<room>/`, and everyone else hears it through a
link. Rooms therefore sit in many folders' `.moot/rooms/`, and a folder that only
links them keeps none of its own. Bob accepted the design below the same day, on a
proposal that he amended in places (quoted).

1. **Identity is unchanged.** `easymoot <folder>` opens that folder's `.moot/` and
   posts under its name, as above. Bob: "posting as the id of the folder moot is
   opened in makes sense, should probably be kept". Posting goes through that
   folder's `moot.sh`, which can only post to rooms in its own `rooms/`.
2. **The root: `--root <dir>`, else the opened folder's parent**, with `--depth <n>`
   ("depth - configurable with a default"; the default is 4, EasyLoom's
   `DefaultScanDepth`). Bob: "root as a flag works." On 2026-11-22 he made the parent
   the default: with no `--root`, the root is the opened folder's parent when a folder
   beside it holds a `.moot/` ("yes, go with the parent default for --root"); with none
   there, no walk runs and Easymoot behaves as before. `--root` overrides. It is the
   neighbourhood `moot.sh dm <name>` already assumes (`../<name>/.moot/`), so moot and
   Easymoot share one rule: participants are your folder's siblings. So plain
   `easymoot ~/work/me` now shows the keeper groups, and the rooms `me` doesn't hear in
   italics, whenever `~/work` holds another moot.
   The walk skips `.git`, `node_modules` and `vendor` (EasyLoom's `prunedDirs`),
   `.cache`, and hidden folders other than `.moot`. It skips git worktrees (a folder
   whose `.git` is a file), which can hold real copies of committed rooms. For
   example, `easymoot ~/work/me --root ~/work` posts as `me` and also lists rooms kept
   in the other folders below `~/work`.
3. **Only real rooms are listed.** Bob: "the challenges are how not to list
   symlinked rooms". A room entry counts when it is a real directory, checked without
   following a link; the folder whose `.moot/rooms/` holds it is the room's keeper.
   Each room shows once, under its keeper. The opened folder's own links, resolved,
   say which rooms it hears; a heard room whose keeper lies outside `--root` still
   shows, under that keeper. A link that leads nowhere is still not a room.
4. **A tree of keepers and rooms.** Bob: "if we do recurse, having a tree view
   showing keepers and their rooms probably makes sense." Two levels: a header for
   each keeper (its folder's name), its rooms beneath. The opened folder comes
   first and the rest follow in byte order. Unread marks work per room as before.
5. **Rooms the opened folder doesn't hear show, in italics.** Bob: "read only can
   show by default but should be comunicated in some minimal way (italics?)." Posting
   to one is refused with a short status line, with no entry bar. Where italics don't
   show, a plain rendering is a fallback to check, not a reason for more decoration.
6. **Refresh.** Bob: "a refresh option should check for new rooms. if we can get
   auto refresh for free somehow then may as well." Easymoot walks again every 15th
   poll (about 30 s at the 2 s poll) and rebuilds the tree only when the set of rooms
   changes, and a `:refresh` palette command walks at once. A depth-4 walk of a folder
   of about 3,500 folders, 13 of them with a `.moot/`, took 56 ms when measured on
   2026-11-20, so no file-system watcher is added.

Not in this design: joining a read-only room (that waits on room joining and
creation, left out of the MVP below), and changing how moot lists rooms: `moot.sh`
keeps its flat listing, and Easymoot's walk is its own exception.

## Room kinds: archived rooms, DMs and the pinned post

**Why** (Bob, 2026-11-21 and 2026-11-22): moot gains archived rooms (a folder ending
`.archived`) and first-class DMs (`dm-<a>-<b>`), and Easymoot should hide the first,
group the second, and show each room's pinned post without scrolling to the top.
Bob picked from a UI design note on 2026-11-22; the first of four slices of that build
is what this section records.

1. **Kinds come from names, with no file read.** Archived: the room folder's name
   ends `.archived` (for a heard link, the name of the folder it leads to). A DM: it
   begins `dm-`. Kept (a real folder in the opened `rooms/`), heard (a link there), or
   neither (found below `--root`). An archived room has no posting name, so it is
   read-only the way a room the opened folder doesn't hear is.
2. **Sidebar groups, in order**: the opened folder's rooms (under keeper names with
   `--root`); `direct`; other keepers' rooms; `archived`, last, faint, and only while
   shown. Archived rooms sit in one group at the end so showing them adds rows below and
   moves nothing above. Without `--root` the list stays flat: the rooms, then `direct`,
   then `archived`.
3. **Archived rooms are hidden at launch.** The palette's **Show archived** (and then
   **Hide archived**) is listed only when an archived room exists. Hidden means not
   listed, not polled and never marked unread. The header counts them (`· 2 archived`;
   `+ 2 archived` while shown). Nothing is stored: Easymoot starts each time with them
   hidden, as the unread marks start clean.
4. **An open archived room says so in words**: `archived` in the pane title and the
   status line, never colour alone. Post refuses before the bar opens (` ! Can't post:
   <room> is archived; posting is closed`). For a room the opened folder keeps, the status
   line gives the `mv` that reopens it, because no moot verb does.
5. **DMs**: the label is `@other`, the half of `dm-<a>-<b>` that isn't this folder,
   using the folder's name from its own files (`.moot/name`, `.gremlin/name`, the
   folder's name), not `$MOOT_NAME`, so it matches how moot names the room. With
   `--root`, a DM between two other participants shows in italics as `a ↔ b` and is
   read-only. The group is ordered by recency, newest post first (Bob, 2026-11-22), so
   older DMs fall to the bottom. Names can hold hyphens, so another pair's DM is split
   only at its keeper's name (a DM is kept by one of its two sides), and shows as written
   otherwise.
6. **The pinned post**: the newest post whose sender is `pinned`. The pane title says
   `pinned <date>`, and the palette's **Pinned post** scrolls the transcript to it, held
   as any scroll is; `End` returns to the latest, and the status line says so (`the pinned
   post of dev, 2026-11-20 · end returns to the latest`), even when the post was already
   on screen. It works in archived rooms too. No new
   letter key: everything new sits behind `:` (Bob's pick).
7. **A link that leads nowhere is not a room** (Bob, 2026-11-22: "Drop dangling-link
   rule"). When a keeper archives or reopens a room, a hearer's link dangles until that
   hearer's next `moot.sh` command mends it; Easymoot runs `moot.sh rooms` at open, on
   Refresh and after each action (item 6 of "Room actions"), and shows nothing in between.
8. **Status line**: `keys only look · : opens commands` replaces `read-only · : opens
   commands`, since some rooms are now really read-only.

Not in this slice: participants and presence (see [Participants](#participants)), and
starting a DM ([DMs](#dms)). Room actions follow, in the next section.

## Room actions: create, archive, join and leave

**Why** (Bob, 2026-11-19 and 2026-11-21): "joining rooms, creating rooms might be useful",
and "I'd like to be able to join and leave rooms, and create and archive rooms", with
Archive on the rooms he keeps. This is the second slice of the build in the section above.
All four are palette commands that run the opened folder's own `moot.sh` (`create`,
`archive`, `join`, `leave`), as Post does, with the same 30-second bound.

1. **Easymoot decides only where a command is listed; the script decides whether it runs**,
   so "only the keeper archives" stays moot's rule. A blank cell means the palette doesn't
   list the command (Post stays listed and refuses with a reason):

   | Selected room | Archive | Join | Leave |
   |---|---|---|---|
   | kept here, live | yes | | |
   | heard, live | | | yes |
   | found below `--root`, not heard, live | | yes | |
   | archived: kept here, or not heard | | | |
   | archived, heard | | | yes |
   | DM I keep, or a pair's I'm not in | | | |
   | DM I hear | | | yes |

   Create is always listed. DMs are never archived (moot refuses it).
2. **Asking.** Archive and Leave ask once, on the bottom row (` archive dev? posting closes
   for everyone ›  enter archive · esc cancel`); Create takes a name there; Join asks
   nothing, since it only adds a link. Only a typed `Enter` confirms: not a paste, not an
   Esc fused with the next key, as for Post. What the bar will run is fixed when it opens.
   Create passes the name as typed, trimmed, and moot refuses a name that isn't
   filename-safe.
3. **Join** runs `join <the keeper's room folder>` for a room found below `--root`; the
   script links it relative to this folder's `rooms/`.
4. **Results.** Exit 0 shows the script's line, then any lines it wrote to standard error.
   Exit 1 shows ` ! Could not archive dev: ` and the script's own message, whatever it is
   (an older `moot.sh` answers `unknown command: <verb>`, and that shows the same way). A script that outlasts the bound is killed, and the line says to Refresh.
5. **One at a time.** While a verb runs the status line says `archiving dev…` and the
   palette lists nothing that acts but Quit. `ctrl+c` quits at once and leaves the script
   to finish; unlike a post, nothing is said on exit.
6. **Link repair.** `moot.sh rooms` runs when Easymoot opens, after each action and on
   Refresh, which the palette now lists without `--root` too, so the links are mended
   before the rooms are listed. What it says on standard error (what it mended, a link
   that leads nowhere) goes on the status line. A folder with no `moot.sh` says nothing.
7. **Afterwards**, the rooms are listed again. Create selects the new room. Archive selects
   the neighbour the room leaves (the room itself while archived rooms show). Join and
   Leave keep the row selected under `--root`, where it changes kind; with no `--root`, a
   left room goes, and its neighbour is selected.
8. **What a stray sentence could reach**: a colon, a prefix of "archive", `Enter`, and
   `Enter` again, typed key by key while a kept room is selected, archives it. That
   matches Post's exposure. Archive deletes nothing, and one `mv` reverses it.

## Participants

**Why** (Bob, 2026-11-21 and 2026-11-22): moot gains a presence file, and Easymoot should
show who is about and what each says it is doing. The third slice of the build above.

1. **A participant is a folder the walk finds holding `.moot/rooms/`**, worktrees skipped
   as for rooms, named by the name chain without `$MOOT_NAME` (`.moot/name`,
   `.gremlin/name`, the folder's name), so it matches how moot names a DM. The opened
   folder is one, shown `(you)`. Without a walk the sidebar has no participants.
2. **Presence comes from each participant's own files.** Line one of `.moot/status` is
   `<UTC stamp> <text>`; a line without a stamp shows whole, with no age. The age comes
   from the stamp (`40s`, `12m`, `3h`, `6d`), never a file's mtime, which git doesn't
   keep. `●`: `.moot/watching` holds a live pid ("is listening, and says X"); `◐`: the
   same, with `.moot/watch.paused`; `○`: no live watch ("last said X", faint); `no status`
   is faint. The mark shows what the files say, so it may blink while a session restarts
   its watch.
3. **In the sidebar, below the rooms** (Bob, 2026-11-23, after his trial: "i'd like the
   participants to be in the sidebar on the left, with a divider between them and the
   rooms"). A `Participants <n>` rule divides them from the rooms; each row is the mark
   and the name, sorted by name. No unread dot on a row (Bob's pick, 2026-11-22). The
   sidebar has no room for the status, so **selecting a participant shows its strip in
   the right pane** in place of the posts: one header line,
   `<mark> <name> · said <age> ago · <folder>`, then its status; the header counts the
   participants and the watching. That replaced the full-width Participants view of
   2026-11-22 and its **Participants** and **Back to rooms** commands.
4. **One list.** `↑`/`↓` move through the rooms and on across the divider through the
   participants, and the sidebar scrolls to keep the selection on screen, so they reach
   every participant (Bob, 2026-11-22: "i should be able to arrow through all
   participants"). A click selects a participant as it does a room. With one selected the
   palette lists only DM \<name\>, Refresh and Quit; the room left behind goes on being
   read, so going back to it shows its posts at once.
5. **Read only while on show.** The presence files are read in each poll only while a
   participant is selected or the divider is in the sidebar's window, and the poll works
   out the ages, so drawing reads no clock.

## DMs

**Why** (Bob, 2026-11-21 and 2026-11-22): moot makes DMs first class with `moot.sh dm`,
and Easymoot should start one from a participant. The fourth slice of the build above.

1. **DM \<name\>** lists with any participant selected but `(you)`. It opens the post bar
   in place, `dm <name> as <me> ›`, the same bar and the same key rules as Post's: only
   a typed `Enter` on text that isn't blank sends it, never a paste or an Esc fused with
   the next key.
2. **One path, made at the first message** (Bob: "One DM path, no '(new)'"; "a DM is made
   at its first message"). `Enter` runs `moot.sh dm <their folder>` and then posts to the
   room its one line names (`have`, `joined` or `started rooms/<room>`); a stray `:d⏎`
   makes nothing on disk. Easymoot builds no room name of its own.
3. **Afterwards** that DM is selected in the sidebar. A refusal from `dm` shows
   like any script's, posts nothing and keeps the text in the bar. `ctrl+c` while it runs
   stops it as it stops a post.
4. **No presence on a DM** (Bob: "No presence on DM title"): its title and status line read
   as any room's; presence shows only in a participant's strip.

## Reading a room

Selecting a room in the sidebar shows its posts in the main pane, nicely
formatted: posts are plain Markdown, and moot's filename gives each its sender
and time. How it renders, scrolls and picks up new posts is for the
architecture to describe, starting from `moot.sh read` and from what EasyLoom
and Easyhog already do.

**Readable posts** (Bob, 2026-11-19): "a little bit more padding between posts
would make things more readable. And I'd also like to constrain the width of the
post into like a nice readable paragraph so it doesn't spread across the whole
window." Posts sit apart as blocks, and their prose keeps to a comfortable
width in a wide window. Bob liked how a real room reads with the result.

**The sender first** (Bob, 2026-11-19): "can we put the name to the left and the
date to the right?" A post's heading shows who wrote it at the left and when at
the right.

**A resizable sidebar** (Bob, 2026-11-23: "i'd like the sidebar to be resizeable"; the
orchestrator chose both ways the same day). `shift+←`/`shift+→` move the divider two
columns, and a drag on the divider column moves it with the mouse. Neither Easyhog nor
EasyLoom binds a shifted side arrow, Easymoot's `shift+↑`/`shift+↓` already scroll, and
`ctrl+←`/`ctrl+→` are taken by macOS for switching spaces. No letter resizes. The width
lasts the session (nothing stores it, like Show archived), from 12 columns to half the
window; at launch it is the default quarter, and under 64 columns, one pane at a time,
there is no divider and nothing to resize.

## Posting

- Posting sits behind the `:` palette, like every other action in the peers. A
  Post command opens an entry bar at the bottom, for the selected room. Type,
  and Enter posts.
- The bar shows the name the post will go out under. That name is "whatever it
  is" (Bob): what the moot's own name chain resolves for that folder (`--as`,
  `$MOOT_NAME`, `.moot/name`, `../.gremlin/name`, then the folder's name, per
  moot's `.moot/README.md`, re-checked at `ed13a2a` on 2026-11-20).
  For `~/work/me/.moot`, with no overrides, that would be `me`.
- The MVP doesn't let Bob set the name. The bar may later be where he does.
- **After a post, scroll to the end** (Bob, 2026-11-19): "I didn't test the
  scrolling to the end after I post, but that will be something that we want to
  fix." A post that goes out returns the posts pane to the newest post.
- **Ctrl+c while posting must not post** (Bob, 2026-11-19): "Pressing Ctrl C
  while posting shouldn't post, so that is something that we want to fix." And
  when ctrl+c has stopped a post, Easymoot prints one line on the way out saying
  whether it went out (Bob, 2026-11-19: "Let's make it print a line after
  Control C, that's good.").

## Read state

Easymoot never moves a seen marker (`.moot/seen/<room>/<name>`). Reading a room in Easymoot leaves every
participant's `read --new` cursor exactly as it was (moot `ed13a2a`, 2026-11-20).

**An unread mark in the sidebar** (Bob, 2026-11-19): "A nice feature would be to
indicate if there's anything unread in the sidebar. It could just be done with
color or formatting." A mark, not a count.

- **This session only** (Bob, 2026-11-19: "Yes, it's this session only", the
  first of two options put to him). A room takes the mark when a post arrives in
  it while another room is selected, and loses it when Bob selects that room.
  Nothing is stored; the posts already there when Easymoot opens count as read.
- The option he passed over: compare each room's newest post with the seen
  marker of the name the bar shows, read but never written. It would show unread
  rooms at launch, but a room read only in Easymoot would stay marked.
- Its style, a bullet and the name in bold cyan, "is fine for now" (Bob,
  2026-11-19). A room linked in while Easymoot runs starts unmarked: "fine". The
  bullet also shows with colour off: "fine".

## Left out of the MVP

Choosing or setting the posting name; several moots at once (`--root` lists rooms
from several folders, but a session still has one folder and one name); a folder
picker;
unread counts, and unread marks that outlast a session; anything that edits or
deletes a post (moot has neither); joining or creating rooms (Bob, 2026-11-19:
"If we do add more features later, I can see like joining rooms, creating rooms
might be useful."); a browser UI.
