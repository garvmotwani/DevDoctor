// Package project inspects the directory DevDoctor was run from and
// reports what kind of project it looks like.
//
// Detection is deliberately conservative: a claim is made only when the
// file or directory naming it exists. File *contents* are not read yet,
// so nothing here may say "React project" just because package.json
// exists — that would be weak evidence.
//
// All filesystem access goes through the fs.FS interface (os.DirFS for
// the real disk, fstest.MapFS in tests) so detection logic is testable
// without touching the user's disk. Inside an fs.FS, paths are always
// slash-separated, even on Windows — which is why this file uses
// path.Join, never filepath.Join, for paths *inside* the FS.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// Fact is one thing we believe about the project, with optional detail.
type Fact struct {
	Label  string // short claim, e.g. "Node.js project"
	Detail string // optional extra information
}

// Report is the result of scanning one project directory.
type Report struct {
	Root  string // the directory that was scanned
	Facts []Fact // every claim made about the project
}

// IsNodeProject reports whether the scan found Node.js indicators.
func (r Report) IsNodeProject() bool {
	for _, f := range r.Facts {
		if f.Label == "Node.js project" {
			return true
		}
	}
	return false
}

// IsPythonProject reports whether the scan found Python indicators.
func (r Report) IsPythonProject() bool {
	for _, f := range r.Facts {
		if f.Label == "Python project" {
			return true
		}
	}
	return false
}

// marker is a path whose existence lets us claim label.
type marker struct {
	relPath string // relative to the project root, slash-separated
	label   string // what presence lets us say
}

// markers is the evidence table. One row per indicator; adding support
// for a new project type means adding rows here, nothing else.
var markers = []marker{
	// Node.js
	{relPath: "package.json", label: "Node.js project"},
	{relPath: "package-lock.json", label: "package-lock.json"},
	{relPath: "npm-shrinkwrap.json", label: "npm-shrinkwrap.json"},
	{relPath: "pnpm-lock.yaml", label: "pnpm-lock.yaml"},
	{relPath: "yarn.lock", label: "yarn.lock"},
	{relPath: "bun.lockb", label: "bun.lockb"},
	// Python
	{relPath: "requirements.txt", label: "Python project"},
	{relPath: "pyproject.toml", label: "Python project"},
	{relPath: "setup.py", label: "Python project"},
	{relPath: "Pipfile", label: "Pipfile"},
	{relPath: "poetry.lock", label: "poetry.lock"},
	// Docker
	{relPath: "Dockerfile", label: "Docker configuration"},
	{relPath: "docker-compose.yml", label: "Docker configuration"},
	{relPath: "compose.yml", label: "Docker configuration"},
	{relPath: "compose.yaml", label: "Docker configuration"},
	// Environment files
	{relPath: ".env", label: ".env"},
	{relPath: ".env.example", label: ".env.example"},
	// Python virtual environment directories
	{relPath: ".venv", label: ".venv"},
	{relPath: "venv", label: "venv"},
}

// Scan inspects the given filesystem path. It is the disk adapter:
// everything interesting happens in scanFS.
func ScanPath(dir string) (Report, error) {
	// os.DirFS takes an OS-form directory path and exposes it as an
	// fs.FS whose internal paths are relative to that directory.
	fsys := os.DirFS(dir)
	rep, err := scanFS(fsys, "")
	if err != nil {
		return rep, err
	}
	rep.Root = dir
	return rep, nil
}

// scanFS implements detection against any fs.FS. root is the prefix
// inside fsys under which the project lives ("" when fsys is rooted at
// the project directory itself, as os.DirFS and our tests arrange).
func scanFS(fsys fs.FS, root string) (Report, error) {
	found := map[string]bool{}

	// The root itself must exist and be a directory. A missing root is
	// a real error (imagine running devdoctor from a directory that was
	// just deleted) and must be reported, not scanned as empty. The
	// Join with "." turns root "" into ".", the FS root itself.
	rootPath := path.Join(root, ".")
	if rootInfo, err := fs.Stat(fsys, rootPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Report{Root: root}, fmt.Errorf("project root %q does not exist", root)
		}
		return Report{Root: root}, fmt.Errorf("stat project root %q: %w", root, err)
	} else if !rootInfo.IsDir() {
		return Report{Root: root}, fmt.Errorf("project root %q is not a directory", root)
	}

	// Git is special-cased: the marker is the .git entry itself (a
	// directory in normal repos, a plain file in worktrees). A file
	// that merely *contains* "git" in its name, like .gitignore, is
	// weak evidence and must not count as a repository.
	present, err := statAny(fsys, path.Join(root, ".git"))
	if err != nil {
		return Report{Root: root}, err
	}
	if present {
		found["Git repository"] = true
	}

	for _, m := range markers {
		present, err := statAny(fsys, path.Join(root, m.relPath))
		if err != nil {
			return Report{Root: root}, err
		}
		if present {
			found[m.label] = true
		}
	}

	// Nothing matched is itself a useful finding, not an error.
	if len(found) == 0 {
		return Report{
			Root:  root,
			Facts: []Fact{{Label: "plain directory", Detail: "no recognizable project files"}},
		}, nil
	}

	// Emit raw facts in a stable order (sorted), then derived
	// conclusions that only make sense given earlier evidence.
	labels := make([]string, 0, len(found))
	for label := range found {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	facts := make([]Fact, 0, len(labels)+2)
	for _, label := range labels {
		facts = append(facts, Fact{Label: label})
	}

	if found["Node.js project"] {
		if detail := lockfileSummary(found); detail != "" {
			facts = append(facts, Fact{Label: "package manager", Detail: detail})
		}
	}
	if found["Python project"] {
		if found[".venv"] || found["venv"] {
			facts = append(facts, Fact{Label: "Python environment", Detail: "virtual environment directory present"})
		} else {
			facts = append(facts, Fact{Label: "Python environment", Detail: "not detected"})
		}
	}

	return Report{Root: root, Facts: facts}, nil
}

// statAny answers "does this path exist?" while preserving the
// three-way outcome: yes, definitely no (ErrNotExist), or unknown
// (any other error — propagate it, don't silently treat it as "no").
func statAny(fsys fs.FS, name string) (bool, error) {
	_, err := fs.Stat(fsys, name)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
}

// lockfileSummary maps lockfile evidence to package manager names.
// More than one lockfile is worth surfacing, not hiding.
func lockfileSummary(found map[string]bool) string {
	var managers []string
	if found["package-lock.json"] || found["npm-shrinkwrap.json"] {
		managers = append(managers, "npm")
	}
	if found["pnpm-lock.yaml"] {
		managers = append(managers, "pnpm")
	}
	if found["yarn.lock"] {
		managers = append(managers, "yarn")
	}
	if found["bun.lockb"] {
		managers = append(managers, "bun")
	}
	switch len(managers) {
	case 0:
		return ""
	case 1:
		return managers[0]
	default:
		return strings.Join(managers, " + ") + " (multiple lockfiles)"
	}
}
