# moot.sh, pinned at c25b5bb

`moot.sh` here is a byte-for-byte copy of `.moot/moot.sh` from
[moot](https://github.com/zealtv/moot) at commit
[`c25b5bb`](https://github.com/zealtv/moot/commit/c25b5bbead24aa81f1fe1d21c483929707375bb5).
The tests copy it into a temporary `.moot/` and run it only there, so they need no
network and no moot installed.

To follow a newer moot, copy its script into a new directory named for the commit,
point the tests at it (`internal/moot/helpers_test.go`,
`internal/tui/helpers_test.go` and `internal/acceptance/world_test.go`), and remove
this one:

    git -C <moot checkout> show <commit>:.moot/moot.sh > internal/moot/testdata/upstream-<commit>/moot.sh
