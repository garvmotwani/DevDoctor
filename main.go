// Command devdoctor reports environment and project problems.
//
// The pipeline is: parse arguments → collect evidence → run rules →
// render (text or JSON) → exit with a code that reflects the result.
// Each step lives in its own small function so tests can exercise them
// without a real machine, and so the diagnostics package stays the
// only place that decides what counts as a problem.
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"devdoctor/internal/diagnostics"
	"devdoctor/internal/process"
	"devdoctor/internal/project"
	"devdoctor/internal/system"
	"devdoctor/internal/tools"
)

// Version is the single source of truth for the reported version,
// used by both the text header and the JSON document.
const Version = "0.1"

// Documented exit codes. Keeping them named makes the mapping in
// exitCodeFor read like the README instead of like magic numbers.
const (
	exitOK       = 0 // scan completed, nothing to fix
	exitProblems = 1 // scan completed, at least one warning or error
	exitFailure  = 2 // bad usage, bad path, or the scan itself failed
)

// result is everything one run gathered, ready to be rendered. Both
// renderers read from this value and nothing else, so text and JSON
// can never disagree.
type result struct {
	System      system.Info
	Tools       []tools.Result
	Report      project.Report
	Ports       process.Snapshot
	Diagnostics []diagnostics.Diagnostic
}

// checkTools is a variable so tests can capture rendering and exit
// codes without spawning real version commands.
var checkTools = func() []tools.Result {
	return tools.CheckAll(tools.Defaults(), 3*time.Second)
}

// checkPorts is a variable for the same reason: tests must not depend
// on which ports happen to be busy on the machine running them.
var checkPorts = func() process.Snapshot {
	return process.SnapshotOf(process.DevPorts)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program as a function of its arguments and output
// streams. Returning the exit code instead of calling os.Exit keeps
// every outcome testable.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "devdoctor: %v\n\n%s", err, usageText)
		return exitFailure
	}
	if opts.help {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}

	root, err := resolveRoot(opts.dir)
	if err != nil {
		fmt.Fprintf(stderr, "devdoctor: %v\n", err)
		return exitFailure
	}
	if opts.serve {
		if err := serveDashboard(root, func(addr string) {
			fmt.Fprintf(stdout, "DevDoctor dashboard: http://%s\nScanning: %s\nPress Ctrl+C to stop.\n", addr, root)
		}); err != nil {
			fmt.Fprintf(stderr, "devdoctor: dashboard stopped: %v\n", err)
			return exitFailure
		}
		return exitOK
	}

	res, err := collect(root)
	if err != nil {
		fmt.Fprintf(stderr, "devdoctor: %v\n", err)
		return exitFailure
	}

	if opts.json {
		if err := renderJSON(stdout, res); err != nil {
			fmt.Fprintf(stderr, "devdoctor: could not write JSON: %v\n", err)
			return exitFailure
		}
	} else {
		renderHuman(stdout, res)
	}

	return exitCodeFor(res.Diagnostics)
}

// collect gathers machine and project evidence and runs the rules.
// Rules are pure functions over the collected evidence, so nothing
// here touches the disk a second time.
//
// Project rules and port rules have different inputs, so they are run
// by two small engines and concatenated in a fixed order: project
// problems first, then machine observations.
func collect(root string) (result, error) {
	rep, err := project.ScanPath(root)
	if err != nil {
		return result{}, err
	}
	ports := checkPorts()

	found := diagnostics.Run(rep, diagnostics.DefaultRules())
	found = append(found, diagnostics.RunPorts(ports, diagnostics.PortRules())...)

	return result{
		System:      system.GetInfo(),
		Tools:       checkTools(),
		Report:      rep,
		Ports:       ports,
		Diagnostics: found,
	}, nil
}
