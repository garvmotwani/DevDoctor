package diagnostics

import (
	"strings"
	"testing"

	"devdoctor/internal/process"
)

func runPortRules(s process.Snapshot) []Diagnostic {
	return RunPorts(s, PortRules())
}

func TestPortOccupiedReportsNameAndPID(t *testing.T) {
	got := runPortRules(process.Snapshot{Ports: []process.Listener{
		{Port: 8000, Protocol: "tcp", PID: 14232, Process: "python.exe"},
	}})

	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %+v", len(got), got)
	}
	d := got[0]
	if d.ID != "PORT-001" {
		t.Errorf("ID = %q, want PORT-001", d.ID)
	}
	if d.Severity != SeverityInfo {
		t.Errorf("Severity = %v, want info (a busy port is not a fault)", d.Severity)
	}
	if d.Title != "Port 8000 is in use" {
		t.Errorf("Title = %q", d.Title)
	}
	for _, want := range []string{"port 8000", "python.exe", "PID: 14232"} {
		if !strings.Contains(d.Evidence, want) {
			t.Errorf("Evidence %q should contain %q", d.Evidence, want)
		}
	}
	if d.SuggestedFix != "" {
		t.Errorf("SuggestedFix = %q, want empty: DevDoctor never suggests stopping a process", d.SuggestedFix)
	}
}

func TestPortOccupiedSaysWhenNameIsUnavailable(t *testing.T) {
	got := runPortRules(process.Snapshot{Ports: []process.Listener{
		{Port: 5432, Protocol: "tcp", PID: 998},
	}})

	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	if !strings.Contains(got[0].Evidence, "PID: 998") {
		t.Errorf("Evidence should keep the PID it does know: %q", got[0].Evidence)
	}
	if !strings.Contains(got[0].Evidence, "not readable") {
		t.Errorf("Evidence should admit the name is missing: %q", got[0].Evidence)
	}
	if strings.Contains(got[0].Evidence, "process: ") {
		t.Errorf("Evidence must not invent a process name: %q", got[0].Evidence)
	}
}

func TestPortOccupiedWithoutAnyOwner(t *testing.T) {
	got := runPortRules(process.Snapshot{Ports: []process.Listener{{Port: 3000, Protocol: "tcp"}}})

	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	if !strings.Contains(got[0].Evidence, "not identified") {
		t.Errorf("Evidence should say the owner is unknown: %q", got[0].Evidence)
	}
}

func TestPortOccupiedIsOneDiagnosticPerPort(t *testing.T) {
	got := runPortRules(process.Snapshot{Ports: []process.Listener{
		{Port: 3000, Protocol: "tcp", PID: 1, Process: "node"},
		{Port: 5432, Protocol: "tcp", PID: 2, Process: "postgres"},
	}})
	if len(got) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(got))
	}
	if got[0].Title != "Port 3000 is in use" || got[1].Title != "Port 5432 is in use" {
		t.Errorf("titles = %q, %q", got[0].Title, got[1].Title)
	}
}

func TestPortOccupiedQuietWhenNothingIsListening(t *testing.T) {
	if got := runPortRules(process.Snapshot{}); len(got) != 0 {
		t.Errorf("got %+v, want no diagnostics for a machine with no dev ports in use", got)
	}
}

func TestPortInspectionLimitedExplainsTheGap(t *testing.T) {
	note := "Reading listening ports and their owning processes is not implemented for this platform yet."
	got := runPortRules(process.Snapshot{Note: note})

	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %+v", len(got), got)
	}
	d := got[0]
	if d.ID != "PROC-001" || d.Severity != SeverityInfo {
		t.Errorf("got %q/%v, want PROC-001/info", d.ID, d.Severity)
	}
	if !strings.Contains(d.Description, note) {
		t.Errorf("Description should carry the explanation: %q", d.Description)
	}
}

// TestPortRulesNeverSuggestKilling is the safety invariant of this
// phase: no configuration of evidence may produce a fix command.
func TestPortRulesNeverSuggestKilling(t *testing.T) {
	snapshots := []process.Snapshot{
		{Ports: []process.Listener{
			{Port: 3000, Protocol: "tcp", PID: 10, Process: "node"},
			{Port: 8080, Protocol: "tcp", PID: 11},
			{Port: 6379, Protocol: "tcp"},
		}},
		{Note: "unavailable"},
	}
	for _, s := range snapshots {
		for _, d := range runPortRules(s) {
			if d.SuggestedFix != "" {
				t.Errorf("%s suggests a fix %q; process and port rules must never act or advise stopping a process", d.ID, d.SuggestedFix)
			}
		}
	}
}
