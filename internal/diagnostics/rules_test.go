package diagnostics

import (
	"strings"
	"testing"

	"devdoctor/internal/env"
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

// envReport builds a report for the environment rules. A nil argument
// means "that file does not exist", which is exactly how the scanner
// signals absence.
func envReport(envFile, example *env.File) project.Report {
	present := map[string]bool{}
	if envFile != nil {
		present[".env"] = true
	}
	if example != nil {
		present[".env.example"] = true
	}
	return project.Report{Present: present, Env: envFile, EnvExample: example}
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
		fix  string
	}{
		{"npm lockfile", report("package.json", "package-lock.json"), true, "npm install"},
		{"pnpm lockfile", report("package.json", "pnpm-lock.yaml"), true, "pnpm install"},
		{"yarn lockfile", report("package.json", "yarn.lock"), true, "yarn install"},
		{"bun lockfile", report("package.json", "bun.lockb"), true, "bun install"},
		{"no lockfile does not guess", report("package.json"), true, ""},
		{"multiple lockfiles do not guess", report("package.json", "package-lock.json", "yarn.lock"), true, ""},
		{"package.json with node_modules", report("package.json", "node_modules"), false, ""},
		{"no package.json at all", report(), false, ""},
		{"lockfile alone is weak evidence", report("package-lock.json"), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingNodeModules(tc.rep)
			if tc.want {
				wantOne(t, got, "DEP-001", SeverityWarning, tc.fix)
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestPackageManifestDiagnostics(t *testing.T) {
	t.Run("invalid JSON is an error", func(t *testing.T) {
		got := invalidPackageManifest(project.Report{PackageErr: true})
		wantOne(t, got, "PKG-001", SeverityError, "Fix the JSON syntax in package.json")
	})
	t.Run("valid or absent manifest is quiet", func(t *testing.T) {
		wantNone(t, invalidPackageManifest(project.Report{}))
	})
	t.Run("conflicting lockfiles are a warning", func(t *testing.T) {
		got := conflictingLockfiles(report("package-lock.json", "yarn.lock"))
		wantOne(t, got, "DEP-002", SeverityWarning, "Choose one package manager and remove the obsolete lockfiles")
		if !strings.Contains(got[0].Evidence, "npm, yarn") {
			t.Errorf("Evidence = %q", got[0].Evidence)
		}
	})
	t.Run("one lockfile is quiet", func(t *testing.T) {
		wantNone(t, conflictingLockfiles(report("pnpm-lock.yaml")))
	})
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

func TestMissingEnvFile(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool
	}{
		{"example without .env", envReport(nil, &env.File{Keys: []string{"API_KEY"}}), true},
		{"both present", envReport(&env.File{Keys: []string{"API_KEY"}}, &env.File{Keys: []string{"API_KEY"}}), false},
		{".env only", envReport(&env.File{Keys: []string{"API_KEY"}}, nil), false},
		{"neither present", envReport(nil, nil), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingEnvFile(tc.rep)
			if tc.want {
				// No fix: copying a file is platform-specific and is
				// never executed by DevDoctor.
				wantOne(t, got, "ENV-001", SeverityWarning, "")
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestMissingEnvKeys(t *testing.T) {
	cases := []struct {
		name    string
		rep     project.Report
		want    bool
		wantTxt string // substring that must appear in Evidence
	}{
		{
			name:    "one key missing",
			rep:     envReport(&env.File{Keys: []string{"A"}}, &env.File{Keys: []string{"A", "B"}}),
			want:    true,
			wantTxt: "B",
		},
		{
			name:    "two keys missing",
			rep:     envReport(&env.File{Keys: []string{"A"}}, &env.File{Keys: []string{"A", "B", "C"}}),
			want:    true,
			wantTxt: "B, C",
		},
		{
			name: "all keys present",
			rep:  envReport(&env.File{Keys: []string{"A", "B"}}, &env.File{Keys: []string{"A", "B"}}),
			want: false,
		},
		{
			name: "no .env at all: ENV-001 covers that case",
			rep:  envReport(nil, &env.File{Keys: []string{"A"}}),
			want: false,
		},
		{
			name: "no example file",
			rep:  envReport(&env.File{Keys: []string{"A"}}, nil),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := missingEnvKeys(tc.rep)
			if !tc.want {
				wantNone(t, got)
				return
			}
			wantOne(t, got, "ENV-002", SeverityWarning, "")
			if !strings.Contains(got[0].Evidence, tc.wantTxt) {
				t.Errorf("Evidence = %q, want it to mention %q", got[0].Evidence, tc.wantTxt)
			}
		})
	}
}

func TestEmptyEnvVariables(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool
	}{
		{
			name: "one empty variable",
			rep:  envReport(&env.File{Keys: []string{"A"}, Empty: []string{"A"}}, nil),
			want: true,
		},
		{
			name: "no empty variables",
			rep:  envReport(&env.File{Keys: []string{"A"}}, nil),
			want: false,
		},
		{
			name: "no .env file",
			rep:  envReport(nil, nil),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := emptyEnvVariables(tc.rep)
			if tc.want {
				wantOne(t, got, "ENV-003", SeverityWarning, "")
				if !strings.Contains(got[0].Evidence, "A") {
					t.Errorf("Evidence = %q, want variable name", got[0].Evidence)
				}
			} else {
				wantNone(t, got)
			}
		})
	}
}

func TestMalformedEnvLines(t *testing.T) {
	cases := []struct {
		name string
		rep  project.Report
		want bool
	}{
		{
			name: "malformed lines",
			rep:  envReport(&env.File{Malformed: []int{3, 7}}, nil),
			want: true,
		},
		{
			name: "clean file",
			rep:  envReport(&env.File{Keys: []string{"A"}}, nil),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := malformedEnvLines(tc.rep)
			if !tc.want {
				wantNone(t, got)
				return
			}
			wantOne(t, got, "ENV-004", SeverityWarning, "")
			// Line numbers only: never the line contents, which may
			// hold a secret.
			for _, want := range []string{"line 3", "line 7"} {
				if !strings.Contains(got[0].Evidence, want) {
					t.Errorf("Evidence = %q, want it to mention %q", got[0].Evidence, want)
				}
			}
		})
	}
}

func TestRunAppliesRulesInOrder(t *testing.T) {
	// Fires all the Phase 4 rules; output order must be rule order.
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

func TestRunAppliesEnvRulesInOrder(t *testing.T) {
	rep := envReport(
		&env.File{Keys: []string{"A"}, Empty: []string{"A"}, Malformed: []int{9}},
		&env.File{Keys: []string{"A", "B"}},
	)
	got := Run(rep, DefaultRules())
	ids := make([]string, 0, len(got))
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	want := "ENV-002,ENV-003,ENV-004"
	if strings.Join(ids, ",") != want {
		t.Errorf("order = %v, want %s", ids, want)
	}
}

func TestRunOnZeroReportFindsNothing(t *testing.T) {
	// A zero Report has a nil Present map and nil Env files; reading
	// them must not panic and no rule may fire without evidence.
	got := Run(project.Report{}, DefaultRules())
	wantNone(t, got)
}
