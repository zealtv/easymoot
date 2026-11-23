package moot

import (
	"context"
	"errors"
)

// RunResult is what a successful Run reports.
type RunResult struct {
	// Lines are the non-blank lines the script wrote to standard output: the room
	// verbs print one ("created rooms/ops"); rooms prints a listing.
	Lines []string
	// Notices are the lines it wrote to standard error ("moot: " prefixes removed):
	// link repair reports what it mended there, even when the command succeeds.
	Notices []string
}

// Run runs one moot.sh verb in the opened folder, the way Post runs post (see
// execScript): the folder's own script, no shell, the same bound (WithTimeout), the
// exit status, and the lines on standard output and error. It asks for no --json.
//
//	m.Run(ctx, []string{"archive", "ops"})
//
// A script that exits non-zero is a *ScriptError holding its message, a missing one
// is ErrNoScript, and one that outlasts the bound is killed and gives ErrTimeout. Run
// decides nothing about the verb: the script says whether it runs.
func (m *Moot) Run(ctx context.Context, args []string, opts ...PostOption) (RunResult, error) {
	cfg := postConfig{timeout: DefaultPostTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	if len(args) == 0 || args[0] == "" {
		return RunResult{}, errors.New("no verb to run")
	}
	stdout, notices, err := m.execScript(ctx, cfg, "", args...)
	return RunResult{Lines: lines(stdout), Notices: notices}, err
}
