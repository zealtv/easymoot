// Package acceptance holds the MVP's end-to-end acceptance test. It has only
// test files, and they run only when EASYMOOT_ACCEPTANCE=1 (see `make
// acceptance`), as Easyhog's internal/acceptance does.
//
// The test builds a synthetic home directory under t.TempDir() with several
// folders holding moots, each with a copy of the pinned upstream moot.sh, and
// drives the real internal/moot beneath the Bubble Tea model
// through its Update method, with no terminal: the model's commands run for real,
// and their answers come back as messages, as Bubble Tea's own loop would send
// them. A second step builds the easymoot binary and runs it over the same home,
// with standard output redirected, to check how start-up finds the one folder to
// open, and that a folder with no .moot/ is an error.
//
// It never touches this repository's own .moot/, ~/.moot or any
// other real room: HOME is the synthetic home for the whole session. At the end it
// checks that every file under every seen/ directory is byte-identical, with its
// modification time unchanged, and that the only new paths in the home are the
// three posts the session wrote.
package acceptance
