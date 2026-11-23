# moot.sh and its README, pinned at ce10b46

`moot.sh` here is a byte-for-byte copy of `.moot/moot.sh` from
[moot](https://github.com/zealtv/moot) at commit `ce10b46`, the last commit of its
`moot-archive-dm-presence` branch, and `MOOT-README.md` is a byte-for-byte copy of
that commit's `.moot/README.md`. (It carries another name because this file is the
pin's own note.) The script's verbs are `create`, `archive`, `leave`, `join`, `dm`,
`status` and `rooms`, besides `post`, `read` and `watch`; every command except
`help` starts with link repair. `MOOT-README.md` is moot's own contract for them.

The tests copy the script into a temporary `.moot/` and run it only there, so they
need no network and no moot installed. Nothing reads `MOOT-README.md`; it sits
beside the script for whoever checks a detail against the pin.

To follow a newer moot, copy its script and README into a new directory named for
the commit, point the tests at it (`internal/moot/helpers_test.go`,
`internal/tui/helpers_test.go`, `internal/acceptance/world_test.go` and
`cmd/easymoot/exit_test.go`), write this note again there, and remove this
directory:

    git -C <moot checkout> show <commit>:.moot/moot.sh > internal/moot/testdata/upstream-<commit>/moot.sh
    git -C <moot checkout> show <commit>:.moot/README.md > internal/moot/testdata/upstream-<commit>/MOOT-README.md
