package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// options is the parsed command line.
type options struct {
	json  bool   // machine-readable output instead of text
	help  bool   // print usage and stop
	serve bool   // host the local browser dashboard
	dir   string // directory to scan; "" means the current directory
}

// usageText is shown for --help and appended to every usage error.
const usageText = `DevDoctor — local-first developer diagnostics

Usage:
  devdoctor [--json]                      scan the current directory
  devdoctor scan [--json] [directory]     scan a directory
  devdoctor serve [directory]             host a local browser dashboard

Flags:
  --json    print one JSON document instead of text
  --help    show this message

Dashboard:
  The serve command listens only on the first available address from
  http://127.0.0.1:8080 through :8089 and never uploads project data.
  Press Ctrl+C to stop it.

Exit codes:
  0  scan completed, no warnings or errors
  1  scan completed, at least one warning or error
  2  invalid usage, invalid path, or scan failure
`

// parseArgs turns raw arguments into options.
//
// Parsing is deliberately strict: anything that is not a flag or a
// subcommand we understand is an error. DevDoctor reports on whatever
// the user pointed it at, so guessing what a typo meant would be worse
// than refusing to run.
func parseArgs(args []string) (options, error) {
	var opts options
	var positional []string

	for _, arg := range args {
		switch {
		case arg == "--json" || arg == "-json":
			opts.json = true
		case arg == "--help" || arg == "-h" || arg == "help":
			opts.help = true
		case strings.HasPrefix(arg, "-"):
			// An unrecognized flag is never a directory name.
			return options{}, fmt.Errorf("unknown flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if opts.help {
		return opts, nil
	}

	// No arguments at all keeps the original behaviour: scan here.
	if len(positional) == 0 {
		return opts, nil
	}
	if positional[0] != "scan" && positional[0] != "serve" {
		return options{}, fmt.Errorf("unknown command %q", positional[0])
	}
	if positional[0] == "serve" {
		if opts.json {
			return options{}, errors.New("--json cannot be used with the serve command")
		}
		opts.serve = true
	}

	switch len(positional) - 1 {
	case 0:
		return opts, nil
	case 1:
		opts.dir = positional[1]
		return opts, nil
	default:
		return options{}, fmt.Errorf("too many arguments for %q (expected at most one directory)", positional[0])
	}
}

// resolveRoot turns the requested directory into an absolute path and
// proves it is a directory before anything scans it. An empty dir
// means "wherever the user is", which is the default behaviour.
func resolveRoot(dir string) (string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not determine working directory: %w", err)
		}
		return wd, nil
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid path \"%s\": %w", dir, err)
	}
	info, err := os.Stat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("invalid path \"%s\": does not exist", dir)
	}
	if err != nil {
		return "", fmt.Errorf("invalid path \"%s\": %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("invalid path \"%s\": not a directory", dir)
	}
	return abs, nil
}
