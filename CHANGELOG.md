# Changelog

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
