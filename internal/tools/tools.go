// Package tools detects developer tools installed on the machine.
//
// The mechanism is always the same: resolve the command on PATH,
// run it with arguments that make it print its version, parse the
// version out of the output. No shell is involved at any point.
package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"time"
)

// Status is the outcome of checking a single tool.
type Status int

const (
	StatusOK       Status = iota // found and version understood
	StatusNotFound               // executable not on PATH
	StatusError                  // found, but it could not be run or the version output was unrecognized
)

// String makes Status printable. fmt calls this automatically
// whenever a Status is printed with %s or %v.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusNotFound:
		return "not found"
	case StatusError:
		return "error"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// Tool describes one developer tool to check.
type Tool struct {
	Name           string   // human-readable name, e.g. "Node.js"
	Command        string   // executable to look up on PATH, e.g. "node"
	VersionArgs    []string // arguments that make it print its version, e.g. {"--version"}
	VersionPattern string   // regex; first capture group must be the version string
}

// Result is what we learned about one tool.
type Result struct {
	Tool    Tool
	Status  Status
	Version string // e.g. "2.51.0"; only meaningful when Status == StatusOK
	Path    string // full path of the executable, if it was found
	Err     error  // underlying failure, only meaningful when Status == StatusError
}

// Check looks up one tool and, if present, asks it for its version.
// timeout bounds how long the child process may run.
func Check(tool Tool, timeout time.Duration) Result {
	res := Result{Tool: tool}

	// Step 1: does the executable exist on PATH at all?
	path, err := exec.LookPath(tool.Command)
	if err != nil {
		res.Status = StatusNotFound
		return res
	}
	res.Path = path

	// Step 2: run "<path> <versionArgs>" with a deadline.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// We pass the resolved path, not the bare command name, so the
	// child runs exactly the program we found.
	cmd := exec.CommandContext(ctx, path, tool.VersionArgs...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out // some tools print their version to stderr

	if err := cmd.Run(); err != nil {
		res.Status = StatusError
		res.Err = err
		return res
	}

	// Step 3: pull the version out of the output.
	res.Version = extractVersion(tool.VersionPattern, out.String())
	if res.Version == "" {
		res.Status = StatusError
		res.Err = fmt.Errorf("version output did not match pattern %q", tool.VersionPattern)
		return res
	}

	res.Status = StatusOK
	return res
}

// CheckAll checks each tool in turn. Sequential on purpose: output order
// is stable and it is easy to reason about. Parallelizing is a later,
// deliberate optimization.
func CheckAll(list []Tool, timeout time.Duration) []Result {
	results := make([]Result, 0, len(list))
	for _, t := range list {
		results = append(results, Check(t, timeout))
	}
	return results
}

// extractVersion returns the first capture group of the first match of
// pattern in output, or "" if the output does not match.
func extractVersion(pattern, output string) string {
	// MustCompile panics on an invalid pattern. That is acceptable here:
	// patterns come from our own source code, never from user input.
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
