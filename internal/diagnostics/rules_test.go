package diagnostics

import (
	"strings"
	"testing"

	"devdoctor/internal/project"
)

// report builds a Report carrying only the evidence rules read.
// No filesystem, no scanner: these tests exercise the rules alone.
func report(present ...string) project.Report {
	m := map[string]bool{}
	for _, p := range present {
		m[p] = true
	}
	return project.Report{Present: m}
}

// wantOne asserts exactly one diagnostic was produced and that it
// carries the expected identity (ID, severity, fix).
func wantOne(t *testing.T, got []Diagnostic, id string, sev Severity, fix string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %+v", len(got), got)
	}
	d := got[0]
	if d.ID != id {
		t.Errorf("ID = %q, want %q", d.ID, id)
	}
	if d.Severity != sev {
		t.Errorf("Severity = %v, want %v", d.Severity, sev)
	}
	if d.SuggestedFix != fix {
		t.Errorf("SuggestedFix = %q, want %q", d.SuggestedFix, fix)
	}
	if d.Title == "" || d.Description == "" || d.Evidence == "" {
		t.Errorf("diagnostic has empty text fields: %+v", d)
	}
}

func wantNone(t *testing.T, got []Diagnostic) {
	t.Helper()
	if len(got) != 0 {
		t.Fatalf("got %d diagnostics, want 0: %+v", len(got), got)
	}
}

func TestMissingNodeModules(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool // expect the diagnostic
	}{
		{"package.json without node_modules", report("package.json"), true},
		{"package.json with node_modules", report("package.json", "node_modules"), false},
		{"no package.json at all", report(), false},
		{"lockfile alone is weak evidence", report("package-lock.json"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingNodeModules(tc.rep)
			if tc.want {
				wantOne(t, got, "DEP-001", SeverityWarning, "npm install")
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestMissingPythonEnvironment(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool
	}{
		{"requirements.txt without venv", report("requirements.txt"), true},
		{"pyproject.toml without venv", report("pyproject.toml"), true},
		{"setup.py without venv", report("setup.py"), true},
		{".venv present", report("requirements.txt", ".venv"), false},
		{"venv present", report("requirements.txt", "venv"), false},
		{"no python files", report(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingPythonEnvironment(tc.rep)
			if tc.want {
				// Info severity, no fix: this rule reports, it does not prescribe.
				wantOne(t, got, "PY-001", SeverityInfo, "")
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestMissingGitRepository(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool
	}{
		{"node project without git", report("package.json"), true},
		{"python project without git", report("requirements.txt"), true},
		{"docker project without git", report("Dockerfile"), true},
		{"git present", report("package.json", ".git"), false},
		{"plain directory", report(), false},
		{".env alone is weak evidence", report(".env"), false},
		{".gitignore alone is weak evidence", report(".gitignore"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingGitRepository(tc.rep)
			if tc.want {
				wantOne(t, got, "GIT-001", SeverityInfo, "git init")
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestRunAppliesRulesInOrder(t *testing.T) {
	// Fires all three rules; output order must be rule order.
	rep := report("package.json", "requirements.txt")
	got := Run(rep, DefaultRules())
	ids := make([]string, 0, len(got))
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	if want := "DEP-001,PY-001,GIT-001"; strings.Join(ids, ",") != want {
		t.Errorf("order = %v, want %s", ids, want)
	}
}

func TestRunOnZeroReportFindsNothing(t *testing.T) {
	// A zero Report has a nil Present map; reading it must not panic
	// and no rule may fire on an evidence-free report.
	got := Run(project.Report{}, DefaultRules())
	wantNone(t, got)
}
