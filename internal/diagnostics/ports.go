package diagnostics

import (
	"fmt"

	"devdoctor/internal/process"
)

// PortRule is the process counterpart of Rule: a pure function of what
// the machine inspection observed.
//
// It is a separate type because the evidence is separate. Project rules
// read a project.Report; port rules read a process.Snapshot. Keeping
// the two inputs apart is what stops a project fact from being
// mistaken for machine state. Diagnostics remain the only place that
// decides what counts as a problem.
type PortRule func(process.Snapshot) []Diagnostic

// PortRules returns the process and port rules in display order.
func PortRules() []PortRule {
	return []PortRule{
		portOccupied,
		portInspectionLimited,
	}
}

// RunPorts applies each port rule in order and concatenates the
// results, mirroring Run.
func RunPorts(s process.Snapshot, rules []PortRule) []Diagnostic {
	var out []Diagnostic
	for _, rule := range rules {
		out = append(out, rule(s)...)
	}
	return out
}

// portOccupied implements PORT-001: a development port has a listener.
//
// Severity is info, on purpose. A busy port is not a fault: it may be
// the developer's own server, a database they want running, or an
// unrelated service. It only becomes a problem when something fails to
// bind, which DevDoctor cannot know in advance. Warning here would turn
// a healthy machine into exit code 1 and make the tool cry wolf.
func portOccupied(s process.Snapshot) []Diagnostic {
	out := make([]Diagnostic, 0, len(s.Ports))
	for _, l := range s.Ports {
		out = append(out, Diagnostic{
			ID:          "PORT-001",
			Severity:    SeverityInfo,
			Title:       fmt.Sprintf("Port %d is in use", l.Port),
			Description: "A program is already listening on this development port. If a server you start tries to use the same port, binding will fail or the server will choose a different one. DevDoctor only reports this; it never stops a process.",
			Evidence:    listenerEvidence(l),
			// No SuggestedFix: the safe action depends on whether the
			// process is yours and wanted. Both possible answers
			// ("stop it" / "keep it") are guesses, and stopping a
			// process is exactly what this tool must not suggest
			// blindly.
		})
	}
	return out
}

// listenerEvidence describes what we know, and says so plainly when
// part of it could not be read. A missing process name is never
// replaced with a guess.
func listenerEvidence(l process.Listener) string {
	switch {
	case l.Process != "" && l.PID > 0:
		return fmt.Sprintf("TCP listening socket on port %d; process: %s; PID: %d", l.Port, l.Process, l.PID)
	case l.PID > 0:
		return fmt.Sprintf("TCP listening socket on port %d; PID: %d; process name not readable at this permission level", l.Port, l.PID)
	default:
		return fmt.Sprintf("TCP listening socket on port %d; owning process not identified", l.Port)
	}
}

// portInspectionLimited implements PROC-001: the machine inspection
// itself was limited (unsupported platform or a permission boundary).
// It is information, not a problem: the project half of the scan is
// unaffected.
func portInspectionLimited(s process.Snapshot) []Diagnostic {
	if s.Note == "" {
		return nil
	}
	return []Diagnostic{{
		ID:           "PROC-001",
		Severity:     SeverityInfo,
		Title:        "Process information is limited",
		Description:  s.Note + " DevDoctor reports what it can read and treats the rest as a known limitation rather than a fault.",
		Evidence:     "No listening-socket data was available from this system.",
		SuggestedFix: "",
	}}
}
