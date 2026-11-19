// Command easymoot is a terminal interface over the rooms of one folder's
// .moot/. `easymoot [path]` opens that folder's moot read-only: a sidebar of its
// rooms and the selected room's posts. With no path it uses the working directory.
// A folder with no .moot/ is an error that names the folder; nothing is drawn.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/x/term"

	"github.com/zealtv/easymoot/internal/moot"
	"github.com/zealtv/easymoot/internal/tui"
)

// version is overridable at link time; otherwise it comes from the build info
// the toolchain stamps in (as EasyLoom's does).
var version = ""

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "easymoot: %v\n", err)
		}
		os.Exit(1)
	}
}

type options struct {
	path    string
	version bool
}

// run does the whole command. The terminal interface runs only when stdin and
// stdout are both terminals; otherwise it reads the moot once and prints one frame.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}
	if opts.version {
		fmt.Fprintln(stdout, buildVersion())
		return nil
	}
	path := opts.path
	if path == "" {
		path = "."
	}
	src, err := moot.Open(path)
	if err != nil {
		return err
	}
	model := tui.New(src)
	in, inOK := stdin.(*os.File)
	out, outOK := stdout.(*os.File)
	interactive := inOK && outOK && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
	if forceInteractive {
		interactive = true
	}
	line, err := tui.Run(context.Background(), model, stdin, stdout, interactive)
	if line != "" {
		// The screen is gone by now: one line on standard error, after ctrl+c stopped
		// a post, saying whether it went out.
		fmt.Fprintln(stderr, line)
	}
	return err
}

// forceInteractive lets a test run the terminal interface over pipes.
var forceInteractive = false

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("easymoot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: easymoot [flags] [path]")
		fmt.Fprintln(stderr, "\npath is a folder holding a .moot directory, or that .moot directory itself.")
		fmt.Fprintln(stderr, "With no path, the working directory is used.")
		fmt.Fprintln(stderr, "\nflags:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	switch flags.NArg() {
	case 0:
	case 1:
		opts.path = flags.Arg(0)
	default:
		flags.Usage()
		return opts, fmt.Errorf("one folder at a time: got %d paths", flags.NArg())
	}
	return opts, nil
}

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return "dev+" + setting.Value
		}
	}
	return "dev"
}
