// Command easymoot is a terminal interface over the rooms of one folder's
// .moot/. `easymoot [path]` opens that folder's moot: a sidebar of its rooms and the
// selected room's posts, which it only reads; writing goes through the folder's own
// moot.sh. With no path it uses the working directory.
// A folder with no .moot/ is an error that names the folder; nothing is drawn.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	root    string
	depth   int
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
	var modelOpts []tui.Option
	if opts.root == "" {
		opts.root = defaultRoot(src.Folder())
	}
	if opts.root != "" {
		root, err := filepath.Abs(opts.root)
		if err == nil {
			root, err = filepath.EvalSymlinks(root)
		}
		if err != nil {
			return fmt.Errorf("--root %q: %w", opts.root, err)
		}
		info, err := os.Stat(root)
		if err != nil {
			return fmt.Errorf("--root %q: %w", opts.root, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("--root %q: not a directory", opts.root)
		}
		modelOpts = append(modelOpts, tui.WithRoot(root, opts.depth))
	}
	model := tui.New(src, modelOpts...)
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

// defaultRoot is the root a run without --root walks: the opened folder's parent, when
// a sibling folder there holds a .moot/ (Bob, 2026-11-22: "go with the parent default
// for --root"). That is the neighbourhood moot.sh dm <name> assumes. Otherwise "", and
// no walk runs.
func defaultRoot(folder string) string {
	parent := filepath.Dir(folder)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	self, err := os.Stat(folder)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		dir := filepath.Join(parent, e.Name())
		// The opened folder, however its name is spelled here (case, a link), is no sibling.
		if info, err := os.Stat(dir); err != nil || os.SameFile(self, info) {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, ".moot")); err == nil && info.IsDir() {
			return parent
		}
	}
	return ""
}

// forceInteractive lets a test run the terminal interface over pipes.
var forceInteractive = false

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("easymoot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.root, "root", "", "also discover rooms and participants below this directory\n(default: the folder's parent, when a folder beside it has a .moot)")
	flags.IntVar(&opts.depth, "depth", moot.DefaultScanDepth, "discovery depth below root (root is 0)")
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: easymoot [flags] [path]")
		fmt.Fprintln(stderr, "\npath is a folder holding a .moot directory, or that .moot directory itself.")
		fmt.Fprintln(stderr, "With no path, the working directory is used.")
		fmt.Fprintln(stderr, "\nflags:")
		flags.PrintDefaults()
	}
	// Accept flags after the opened folder as well as before it.
	var ordered, paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			ordered = append(ordered, a)
			if a == "--root" || a == "-root" || a == "--depth" || a == "-depth" {
				if i+1 == len(args) {
					return opts, fmt.Errorf("flag needs an argument: %s", a)
				}
				i++
				ordered = append(ordered, args[i])
			}
		} else {
			paths = append(paths, a)
		}
	}
	ordered = append(ordered, "--")
	ordered = append(ordered, paths...)
	if err := flags.Parse(ordered); err != nil {
		return opts, err
	}
	if opts.depth < 0 {
		return opts, fmt.Errorf("--depth must be non-negative")
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
