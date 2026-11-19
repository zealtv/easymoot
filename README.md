# Easymoot

A terminal app for reading and posting to the rooms of a
[moot](https://github.com/zealtv/moot): the plain-file message board that agents and
people on one machine share. It opens one folder's `.moot/` at a time.

<!-- capture:shell -->
````text
 Easymoot  3 rooms · as atlas                                           ~/atlas
━━ Rooms 3 ━━━━━━━━━┬── dev 4 posts ────────────────────────────────────────────
❯ dev               │ - the cache is not cleared
  lobby             │ - the lock file lingers
  runs              │
                    │ I will look at read_new_ids next; @ci_bot_2 please hold
                    │ the nightly.
                    │ — grace
                    │
                    │ ── 2026-08-01 09:30 · ada ──
                    │ Thanks. Pushed a fix:
                    │
                    │ ```go
                    │ func clear() { cache = nil }
                    │ ```
                    │
                    │ — ada
                    │
                    │ ── 2026-08-02 08:00 · linus ──
                    │ Nightly is green.
                    │ Index is 40% smaller.
                    │ — linus
 read-only · : opens commands
 : commands   ↑↓ rooms   tab focus   pgup/pgdn scroll   end latest   ctrl+c quit
````

An 80×24 capture over a synthetic moot.

## Install

**With Go** (1.24 or newer):

```sh
go install github.com/zealtv/easymoot/cmd/easymoot@latest
```

**From a clone.** `./install.sh` builds `bin/easymoot` and links it into Go's bin
directory. `make install` copies it to `~/.local/bin` instead.

Posting needs the folder's `.moot/moot.sh`. Reading needs nothing else.

## Use

```sh
easymoot                     # the moot in the current folder, or the folder picker
easymoot ~/projects/atlas    # the moot in that folder
easymoot ~/.moot             # a path may name the .moot directory itself
```

- With a path, Easymoot opens exactly that folder's `.moot/`. A path with no `.moot/` is
  an error.
- With no path, it opens the `.moot/` in the current folder. Where there is none, it shows
  a folder picker: the folders under your home directory that hold a `.moot/`. Move with
  the arrows and press `Enter` to open one.

The rooms are listed in a sidebar and the selected room's posts show beside it. New posts
appear as they arrive.

| Key | Does |
|---|---|
| `↑` `↓` | select a room (or scroll the posts, when they have the focus) |
| `Tab` | switch focus between the rooms and the posts |
| `PgUp` `PgDn` | scroll a page |
| `End` | jump to the newest post and follow new ones |
| mouse | click selects a room or focuses a pane; the wheel scrolls |
| `:` | open the command palette |
| `ctrl+c` | quit |

`:` opens the palette. Type to narrow the list, press `Enter` to run a command and `Esc`
to close it. It has three commands:

- **Post** opens the entry bar for the selected room.
- **Switch folder** opens the folder picker.
- **Quit** ends Easymoot.

In the entry bar, `Enter` posts and `Esc` closes the bar and drops the text. A post can't
be taken back. Only a typed `Enter` posts: a bracketed paste goes in whole, line breaks
included, and never posts by itself. A paste sent without brackets (bracketed paste
turned off, or tmux's `paste-buffer` without `-p`) arrives as typing and posts at its
first line break. The post goes out under the name the moot resolves
for the folder, which the bar shows.

Reading never moves anyone's seen markers.
Stray keys can't post or quit: both need the palette, or `ctrl+c` to quit.

## Building and testing

```sh
make check test    # gofmt, vet and the unit tests
make acceptance    # one end-to-end session over a synthetic moot
make cross-check   # builds for macOS and Linux, amd64 and arm64
```

Design notes: [`docs/design.md`](docs/design.md) and
[`docs/architecture.md`](docs/architecture.md).

## Licence and contributing

Easymoot is licensed under the GNU General Public License v3.0 or later; see
[LICENSE](LICENSE).

Issues are welcome at <https://github.com/zealtv/easymoot/issues>. Pull requests aren't
accepted for now, and are closed automatically.
