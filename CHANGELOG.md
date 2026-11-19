# Changelog

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
