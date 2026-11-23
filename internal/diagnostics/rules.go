package diagnostics

import (
	"fmt"
	"strings"

	"devdoctor/internal/project"
)

// DefaultRules returns the Phase 4 rule set, in display order.
// Adding a rule later means appending here — the engine does not care.
func DefaultRules() []Rule {
	return []Rule{
		missingNodeModules,
		missingPythonEnvironment,
		missingGitRepository,
	}
}

// missingNodeModules implements Rule 1: package.json exists but
// node_modules does not.
func missingNodeModules(rep project.Report) []Diagnostic {
	if !rep.Present["package.json"] || rep.Present["node_modules"] {
		return nil
	}
	return []Diagnostic{{
		ID:           "DEP-001",
		Severity:     SeverityWarning,
		Title:        "Node dependencies may not be installed",
		Description:  "Your project declares Node.js dependencies in package.json, but node_modules was not found. The application will not start until dependencies are installed.",
		Evidence:     "package.json exists; node_modules does not exist.",
		SuggestedFix: "npm install",
	}}
}

// missingPythonEnvironment implements Rule 2: a Python dependency
// file is present but no virtual environment directory was detected.
// Severity is info on purpose — a project without a local venv is not
// necessarily broken (system-wide installs are legitimate), so this
// reports what we saw instead of declaring a fault.
func missingPythonEnvironment(rep project.Report) []Diagnostic {
	files := presentOf(rep, "requirements.txt", "pyproject.toml", "setup.py")
	if len(files) == 0 || rep.Present[".venv"] || rep.Present["venv"] {
		return nil
	}
	return []Diagnostic{{
		ID:          "PY-001",
		Severity:    SeverityInfo,
		Title:       "Python environment not detected",
		Description: "A Python dependency file was found, but no virtual environment directory (.venv or venv) is present. This may be intentional if dependencies are installed system-wide.",
		Evidence:    fmt.Sprintf("%s present; .venv and venv do not exist.", strings.Join(files, ", ")),
		// No SuggestedFix: we deliberately do not assume a venv is
		// required, so recommending one would be over-eager.
	}}
}

// missingGitRepository implements Rule 3: project files but no .git.
// Informational, not an error: many directories legitimately are not
// repositories (scratch dirs, extracted archives).
func missingGitRepository(rep project.Report) []Diagnostic {
	if rep.Present[".git"] {
		return nil
	}
	// Only strong indicators count. A lone .env or .gitignore is not
	// enough to claim "this is a project that should be tracked".
	files := presentOf(rep, sourceIndicators...)
	if len(files) == 0 {
		return nil
	}
	return []Diagnostic{{
		ID:           "GIT-001",
		Severity:     SeverityInfo,
		Title:        "No Git repository detected",
		Description:  "This directory contains project files, but no .git directory was found, so changes here are not under version control.",
		Evidence:     fmt.Sprintf("%s found; .git does not exist.", strings.Join(files, ", ")),
		SuggestedFix: "git init",
	}}
}

// sourceIndicators are the files whose presence means "this looks like
// a project worth tracking in Git". Deliberately conservative: weak
// evidence (.env alone, .gitignore alone) must not trigger the rule.
var sourceIndicators = []string{
	"package.json",
	"requirements.txt",
	"pyproject.toml",
	"setup.py",
	"Dockerfile",
	"docker-compose.yml",
	"compose.yml",
	"compose.yaml",
}

// presentOf returns the subset of paths that exist, in the order given.
// Reading a missing key (or a nil map) is legal in Go and yields the
// zero value, so a zero Report simply means "nothing present" — no nil
// check needed at call sites.
func presentOf(rep project.Report, paths ...string) []string {
	var out []string
	for _, p := range paths {
		if rep.Present[p] {
			out = append(out, p)
		}
	}
	return out
}
