package diagnostics

import (
	"fmt"
	"strings"

	"devdoctor/internal/project"
)

// DefaultRules returns the current rule set, in display order.
// Adding a rule later means appending here — the engine does not care.
func DefaultRules() []Rule {
	return []Rule{
		missingNodeModules,
		missingPythonEnvironment,
		missingGitRepository,
		missingEnvFile,
		missingEnvKeys,
		emptyEnvVariables,
		malformedEnvLines,
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

// missingEnvFile implements ENV-001: an example file exists but the
// real .env does not. The example is a template; the app usually
// needs a filled-in copy.
func missingEnvFile(rep project.Report) []Diagnostic {
	if rep.Present[".env"] || !rep.Present[".env.example"] {
		return nil
	}
	return []Diagnostic{{
		ID:          "ENV-001",
		Severity:    SeverityWarning,
		Title:       "No .env file found",
		Description: "This project includes a .env.example file but no .env file. The example is a template: applications typically expect a real .env with local values, and may fail at startup without one.",
		Evidence:    ".env.example exists; .env does not exist.",
		// No SuggestedFix: copying a file is spelled differently per
		// platform and shell, and DevDoctor never does it for the user.
	}}
}

// missingEnvKeys implements ENV-002: names declared in .env.example
// with no counterpart in .env. Only names are reported — a variable
// name is configuration, its value is a secret.
func missingEnvKeys(rep project.Report) []Diagnostic {
	if rep.Env == nil || rep.EnvExample == nil {
		return nil
	}
	var missing []string
	for _, name := range rep.EnvExample.Keys {
		if !rep.Env.Has(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []Diagnostic{{
		ID:          "ENV-002",
		Severity:    SeverityWarning,
		Title:       "Environment variables missing from .env",
		Description: "These variables are declared in .env.example but are not set in .env. If the application reads them at startup, it may fail or behave unexpectedly.",
		Evidence:    fmt.Sprintf("Declared in .env.example, absent from .env: %s", strings.Join(missing, ", ")),
	}}
}

// emptyEnvVariables implements ENV-003: KEY= with nothing after it.
// Most loaders treat an empty value as unset, which is rarely what the
// author intended.
func emptyEnvVariables(rep project.Report) []Diagnostic {
	if rep.Env == nil || len(rep.Env.Empty) == 0 {
		return nil
	}
	return []Diagnostic{{
		ID:          "ENV-003",
		Severity:    SeverityWarning,
		Title:       "Environment variables have no value",
		Description: "These variables are declared in .env but assigned nothing, which most loaders treat as unset. If the application requires them, startup will likely fail.",
		Evidence:    fmt.Sprintf("Declared with an empty value: %s", strings.Join(rep.Env.Empty, ", ")),
	}}
}

// malformedEnvLines implements ENV-004: a line that is not
// NAME=value. Only line numbers are reported. Echoing the line itself
// would risk printing a secret, since a malformed line is often a
// pasted value or a password with a typo.
func malformedEnvLines(rep project.Report) []Diagnostic {
	if rep.Env == nil || len(rep.Env.Malformed) == 0 {
		return nil
	}
	lines := make([]string, len(rep.Env.Malformed))
	for i, n := range rep.Env.Malformed {
		lines[i] = fmt.Sprintf("line %d", n)
	}
	return []Diagnostic{{
		ID:          "ENV-004",
		Severity:    SeverityWarning,
		Title:       "Malformed lines in .env",
		Description: "These lines are not NAME=value pairs, so most loaders ignore them and the variables they were meant to set stay unset. Line contents are not shown because a malformed line may contain a secret.",
		Evidence:    fmt.Sprintf("Could not parse: %s", strings.Join(lines, ", ")),
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
