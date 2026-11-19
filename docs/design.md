# Easymoot design

What Easymoot does and leaves out, as Bob decided it: on 2026-11-18, before any
code, from his brief and his feedback on the first design drafts; and on
2026-11-19, after he tried the first build and after the first release. Each
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

Easymoot works out of one folder's `.moot/` at a time.

- `easymoot [path]` opens the `.moot/` in that folder (the current folder by
  default). `easymoot` in `~` opens `~/.moot`.
- Where there is none, it says so, naming the folder, and exits.
- The sidebar lists that moot's rooms (`.moot/rooms/*`, following the links that
  point into a shared store).

So a session has one participant and one identity, and a room linked into many
repos shows once, under the folder Bob chose.

**No folder picker** (Bob, 2026-11-19). On 2026-11-18 Bob had decided that,
where a folder has no moot, Easymoot would scan for folders that have one and
let him pick, the way EasyLoom does, with a palette command to switch folders
later. After trying the first build he dropped both: "For the moment, I don't
think we even need a folder picker. EasyLoom has one, but EasyHog doesn't. …
However, I do think we can drop the folder picker in easymoot." Ctrl+c at
start-up is "fine for the moment". The one folder a run works in is the one it
is given or started in.

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
participant's `read --new` and `catchup` exactly as they were.

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

Choosing or setting the posting name; several moots at once; a folder picker;
unread counts, and unread marks that outlast a session; anything that edits or
deletes a post (moot has neither); joining or creating rooms (Bob, 2026-11-19:
"If we do add more features later, I can see like joining rooms, creating rooms
might be useful."); a browser UI.
