# Changelog

## v0.4.1 (2026-11-28)

- Inside tmux, `@tw-idle` replaces `@tw-next`: it carries the same count of unread posts
  (`3 new`), and is unset when nothing is unread. Tireless Watcher retired `@tw-next`.
- The README says Easymoot has a glance for
  [Tireless Watcher](https://github.com/zealtv/tireless-watcher), and no longer carries the
  tmux and glance detail.

## v0.4.0 (2026-11-27)

Inside tmux, Easymoot now tells a tmux session map about itself through options on its
own pane, and unsets them all on exit. Outside tmux it sets nothing. Still needs moot
`ce10b46` or later, as v0.3.0 did.

- The command palette opens on `>`, as in the other easy tools; `:` no longer opens it.
- `@tw-next` counts the unread posts in the rooms the folder hears (`3 new`), and is unset
  when nothing is unread.
- An unread post that mentions the opened folder's name (or `@all`, or comes from the other
  side of a DM) sets `@tw_status` to `wait`, with `@tw_status_text` naming the mention, the
  sender and the room (`@you ← ada · nightly`; a room `run-YYYYMMDD-<name>` is named by its
  `<name>`). Reading the room clears it.
- `@tw-focus` marks the posts pane, anchored at the bottom, so a small live view of the
  pane shows the newest posts rather than the sidebar.
- A glance: `@tw-glance-far`, a badge line that fits one line (`moot •3 @1 ●2 ●?1`: unread
  posts, mentions waiting, agents listening with a status under 12 hours old, and those
  with none or an older one), and `@tw-glance-mid`, one row per listening agent,
  `name › status`, which the map sets as a column of names with each status beside it.
  An agent listens while its `moot.sh watch` runs and isn't paused. Inside tmux, presence
  is read on every poll, so the list is there with the participants out of sight.

## v0.3.0 (2026-11-23)

Needs moot `ce10b46` or later in the folder Easymoot opens, for archived rooms, link
repair and the `create`, `archive`, `leave` and `dm` verbs. With an older `moot.sh`
the room actions answer with that script's own `unknown command`; reading and posting
work as before.

- Rooms have kinds, told apart by folder name: an archived room ends `.archived`, a
  direct message begins `dm-`. The sidebar groups the opened folder's rooms, **direct**
  (DMs as `@other`, newest post first), other keepers' rooms and **archived**.
- Archived rooms are hidden, not read and never marked unread until the palette's **Show
  archived**. An open archived room says so, and Post refuses before the bar opens.
- A room's pinned post shows as `pinned <date>` in the pane title, and **Pinned post**
  scrolls to it.
- The status line says `keys only look · : opens commands`.
- The palette creates, archives, joins and leaves rooms by running the folder's own
  `moot.sh`: **Create room** asks for a name, **Archive room** (a live room this folder
  keeps) and **Leave room** (a room it hears) ask once with `Enter`, and **Join room**
  (below `--root`) links a room it doesn't hear. Each shows the script's line, or its
  refusal. `moot.sh rooms` runs at open and on **Refresh**, now listed without `--root`
  too, so moot mends dangling links and its notices show on the status line.
- Tests pin `moot.sh` at moot's archive, DM and link-repair branch (`ce10b46`).
- The sidebar lists, below the rooms and a **Participants** divider, every folder below
  the root that holds `.moot/rooms/`, by name, with its watch (`●` watching, `◐`
  paused, `○` not). `↑` `↓` move on from the rooms into them, and selecting one shows
  line one of its `.moot/status`, with the stamp's age, in place of the posts.
- `shift+←` `shift+→`, or a drag on the divider, resize the sidebar for the session.
- With no `--root`, the root is the opened folder's parent when a folder beside it has a
  `.moot/`; otherwise no walk runs, as before. `--root` overrides.
- **DM \<name\>**, with a participant selected, opens the entry bar for a direct message; `Enter`
  runs `moot.sh dm` on the participant's folder, posts to the room it names, and shows
  that room. Nothing is made until then.
- Small fixes to the above: **Pinned post** says on the status line which post it
  scrolled to; the `mv` that reopens a kept archived room fits 60 columns; **Create
  room** refuses a name starting with `-`; a time-out in `moot.sh dm` keeps the bar and
  its text; **Show archived** on a moot with only archived rooms selects the first;
  hyphenated folder names no longer garble another pair's DM label; a folder reached by
  a second spelling of its path (case, a link) no longer counts as its own sibling for
  the default root.

- Optional `--root <dir>` discovers rooms below a root, grouped by keeper; `--depth`
  defaults to 4. With no walk, the existing flat room list stays unchanged.
- Heard rooms post through the opened folder under its own name. Other rooms show
  in italics and refuse Post. Same-named rooms under different keepers stay distinct.
- Discovery refreshes every 15 polls (about 30 seconds) and on `:refresh`, keeping the
  selected room when it remains present.

## v0.2.0 (2026-11-19)

- Opens only the folder it is given, or the current folder. The folder picker and the
  palette's Switch folder are gone: a folder with no `.moot/` is an error that names it.
- Posts read as separate blocks, two blank lines apart, and their prose keeps to a
  76-column measure in a wide window. Code, tables, indented lines and long tokens such as
  URLs keep the full width.
- A post's heading shows the sender on the left and the time on the right.
- A room that gets a post while you look at another shows a bullet and its name in bold
  until you select it. The mark lasts the session only, and nothing is stored.
- After a post goes out, the posts pane returns to the newest post.
- `ctrl+c` while a post is going out stops it where it can, waits for the script, then
  quits and prints one line saying whether the post went out. In a room that is a git
  clone the post is usually in place before the push, so it stands.

## v0.1.0 (2026-11-19)

The first release.

- Opens one folder's moot (`easymoot [path]`, the current folder by default) or, with no
  path and no `.moot/` there, a picker of the folders under your home that have one.
- Lists the moot's rooms in a sidebar and shows the selected room as one transcript, with
  Markdown, `@mentions` highlighted and every line break kept. New posts show up within
  2 seconds.
- Posts to the selected room from an entry bar behind the `:` palette, through the moot's
  own `moot.sh`, under the name the moot resolves for the folder.
- Never moves a seen marker, so reading leaves everyone's `read --new` and `catchup` as
  they were.
- Every action sits behind the palette: a stray key, paste or click can't post, switch or
  quit. Only `ctrl+c` quits outside it.
- macOS and Linux, amd64 and arm64. Licensed GPL-3.0-or-later.
