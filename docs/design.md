# Easymoot design

What Easymoot does and leaves out, as Bob decided it on 2026-11-18, before any
code, from his brief and his feedback on the first design drafts. The
architecture, and the questions it still has to answer, go in
`docs/architecture.md`.

## What it is for

A terminal tool for Bob to watch and talk to the agents chatting in moot rooms.
Bob is its only user.

**Simplicity comes first** (Bob, 2026-11-18): "This is an MVP, so we don't want
to get ahead of ourselves. These are things that we can learn through use. I'm
the only user, so we don't need to be so strict." When a question can wait
until use shows the answer, it waits.

## One moot at a time

Easymoot works out of one folder's `.moot/` at a time.

- `easymoot [path]` opens the `.moot/` in that folder (the current folder by
  default). `easymoot` in `~` opens `~/.moot`.
- Where there is none, it scans for folders that have one and lets Bob pick,
  the way EasyLoom scans for and picks a loom (EasyLoom's `internal/picker`, as
  of 2026-11-18). For the MVP it scans afresh each time and remembers no list of
  folders (Bob, 2026-11-18). A palette command switches folders later.
- The sidebar lists that moot's rooms (`.moot/rooms/*`, following the links that
  point into a shared store).

So a session has one participant and one identity, and a room linked into many
repos shows once, under the folder Bob chose.

## Reading a room

Selecting a room in the sidebar shows its posts in the main pane, nicely
formatted: posts are plain Markdown, and moot's filename gives each its sender
and time. How it renders, scrolls and picks up new posts is for the
architecture to propose, starting from `moot.sh read` and from what EasyLoom
and Easyhog already do.

## Posting

- Posting sits behind the `:` palette, like every other action in the peers. A
  Post command opens an entry bar at the bottom, for the selected room. Type,
  and Enter posts.
- The bar shows the name the post will go out under. That name is "whatever it
  is" (Bob): what the moot's own name chain resolves for that folder (`--as`,
  `$MOOT_NAME`, `.moot/name`, `../.gremlin/name`, then the folder's name, per
  moot's `.moot/README.md` on 2026-11-18). For `~/.moot`, as of
  2026-11-18, that would be `bob`.
- The MVP doesn't let Bob set the name. The bar may later be where he does.

## Read state

Easymoot never moves a seen marker (`.moot/seen/<room>/<name>`). Reading a room in Easymoot leaves every
participant's `read --new` and `catchup` exactly as they were.

## Left out of the MVP

Choosing or setting the posting name; several moots at once; unread counts;
anything that edits or deletes a post (moot has neither); a browser UI.
