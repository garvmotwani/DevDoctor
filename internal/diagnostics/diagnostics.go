// Package diagnostics turns scanner evidence into human-readable
// problems.
//
// The pipeline is intentionally plain:
//
//	scanner → project.Report (evidence) → rules → []Diagnostic → report
//
// A rule is a pure function of the evidence: same report in, same
// diagnostics out. Nothing here reads the disk, runs commands, or
// contacts the network. That is what makes the engine deterministic,
// fast, and testable without touching a real machine.
package diagnostics

import (
	"fmt"

	"devdoctor/internal/project"
)

// Severity ranks how serious a diagnostic is. The constants are
// ordered (iota increments one per line), so SeverityError >
// SeverityWarning > SeverityInfo and callers can compare them with
// > or >=.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// String makes Severity printable; fmt uses it for %s and %v.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// Diagnostic is one problem — or noteworthy observation — produced by
// a rule. SuggestedFix is text to show the user; this package never
// executes it. Executing fixes is a separate, explicit future feature.
type Diagnostic struct {
	ID           string // stable identifier, e.g. "DEP-001"
	Severity     Severity
	Title        string // one-line summary
	Description  string // what it means and why it matters
	Evidence     string // the observed facts that triggered the rule
	SuggestedFix string // command to consider; "" when we have nothing useful to suggest
}

// Rule inspects evidence and returns every diagnostic it finds.
// A rule that does not apply returns nil — "no opinion" is a normal
// outcome, not an error.
type Rule func(project.Report) []Diagnostic

// Run applies each rule in order and concatenates the results.
// Deterministic by construction: output order is rule order, then the
// order each rule chose to return.
func Run(rep project.Report, rules []Rule) []Diagnostic {
	var out []Diagnostic
	for _, rule := range rules {
		out = append(out, rule(rep)...)
	}
	return out
}
