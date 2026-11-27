// Package project inspects the directory DevDoctor was run from and
// reports what kind of project it looks like.
//
// Detection is deliberately conservative: marker files establish project
// types, and the small set of manifests we parse produces facts only from
// explicit, well-known metadata. For example, a React claim requires React
// to appear in package.json dependencies.
//
// All filesystem access goes through the fs.FS interface (os.DirFS for
// the real disk, fstest.MapFS in tests) so detection logic is testable
// without touching the user's disk. Inside an fs.FS, paths are always
// slash-separated, even on Windows — which is why this file uses
// path.Join, never filepath.Join, for paths *inside* the FS.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"devdoctor/internal/env"
)

// Fact is one thing we believe about the project, with optional detail.
type Fact struct {
	Label  string // short claim, e.g. "Node.js project"
	Detail string // optional extra information
}

// Report is the result of scanning one project directory.
//
// Facts are for humans; Present, Env and EnvExample are for rules.
// Rules ask about raw paths ("does package.json exist?") instead of
// matching on display labels, so rewording a Fact can never silently
// break a rule.
type Report struct {
	Root       string           // the directory that was scanned
	Facts      []Fact           // every claim made about the project
	Present    map[string]bool  // marker paths (slash-separated, relative to Root) that exist
	Env        *env.File        // parsed .env; nil when the file does not exist
	EnvExample *env.File        // parsed .env.example; nil when the file does not exist
	Package    *PackageManifest // parsed package.json; nil when absent or invalid
	PackageErr bool             // package.json exists but is not valid JSON
}

// PackageManifest is the safe subset of package.json that DevDoctor needs for
// diagnostics. It deliberately excludes package scripts, repository URLs,
// author fields and arbitrary config, which are irrelevant to diagnosis and
// would needlessly enlarge the report surface.
type PackageManifest struct {
	NodeEngine     string
	PackageManager string
	Frameworks     []string
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

// PackageManager returns the package manager indicated by exactly one
// Node.js lockfile. The boolean is false when no lockfile is present or when
// the project has conflicting lockfiles; in both cases a diagnostic must not
// guess which install command is safe to recommend.
func (r Report) PackageManager() (string, bool) {
	managers := r.PackageManagers()
	if len(managers) != 1 {
		return "", false
	}
	return managers[0], true
}

// PackageManagers returns every manager indicated by a lockfile, in stable
// order. More than one value is evidence of an ambiguous install workflow.
func (r Report) PackageManagers() []string {
	return packageManagers(r.Present)
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
	{relPath: "node_modules", label: "node_modules"},
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
	found := map[string]bool{}   // label -> seen (drives Facts)
	present := map[string]bool{} // raw path -> exists (drives rules)

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
	exists, err := statAny(fsys, path.Join(root, ".git"))
	if err != nil {
		return Report{Root: root}, err
	}
	if exists {
		present[".git"] = true
		found["Git repository"] = true
	}

	for _, m := range markers {
		exists, err := statAny(fsys, path.Join(root, m.relPath))
		if err != nil {
			return Report{Root: root}, err
		}
		if exists {
			present[m.relPath] = true
			found[m.label] = true
		}
	}

	// Nothing matched is itself a useful finding, not an error.
	if len(found) == 0 {
		return Report{
			Root:    root,
			Present: present,
			Facts:   []Fact{{Label: "plain directory", Detail: "no recognizable project files"}},
		}, nil
	}

	// Environment files are parsed here, in the evidence stage, so rules
	// stay pure functions of the report. env.Parse keeps only names and
	// shapes — never values — so nothing secret can reach a report, and
	// therefore nothing secret can reach the terminal.
	var envFile, envExample *env.File
	if present[".env"] {
		f, err := env.ParseFS(fsys, path.Join(root, ".env"))
		if err != nil {
			return Report{Root: root}, fmt.Errorf("read .env: %w", err)
		}
		envFile = &f
	}

	// package.json is parsed only for explicit stack metadata. An invalid file
	// is useful diagnostic evidence but must not prevent the rest of the scan
	// from completing.
	var pkg *PackageManifest
	var packageErr bool
	if present["package.json"] {
		parsed, err := parsePackageManifest(fsys, path.Join(root, "package.json"))
		if err != nil {
			packageErr = true
		} else {
			pkg = &parsed
		}
	}
	if present[".env.example"] {
		f, err := env.ParseFS(fsys, path.Join(root, ".env.example"))
		if err != nil {
			return Report{Root: root}, fmt.Errorf("read .env.example: %w", err)
		}
		envExample = &f
	}

	// Emit raw facts in a stable order (sorted), then derived
	// conclusions that only make sense given earlier evidence.
	labels := make([]string, 0, len(found))
	for label := range found {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	facts := make([]Fact, 0, len(labels)+5)
	for _, label := range labels {
		facts = append(facts, Fact{Label: label})
	}

	if found["Node.js project"] {
		if detail := lockfileSummary(found); detail != "" {
			facts = append(facts, Fact{Label: "package manager", Detail: detail})
		}
		if pkg != nil {
			for _, framework := range pkg.Frameworks {
				facts = append(facts, Fact{Label: "framework", Detail: framework})
			}
			if pkg.NodeEngine != "" {
				facts = append(facts, Fact{Label: "Node.js version", Detail: pkg.NodeEngine})
			}
			if pkg.PackageManager != "" {
				facts = append(facts, Fact{Label: "declared package manager", Detail: pkg.PackageManager})
			}
		}
	}
	if found["Python project"] {
		if found[".venv"] || found["venv"] {
			facts = append(facts, Fact{Label: "Python environment", Detail: "virtual environment directory present"})
		} else {
			facts = append(facts, Fact{Label: "Python environment", Detail: "not detected"})
		}
	}

	return Report{
		Root:       root,
		Present:    present,
		Env:        envFile,
		EnvExample: envExample,
		Package:    pkg,
		PackageErr: packageErr,
		Facts:      facts,
	}, nil
}

// parsePackageManifest reads a deliberately narrow, safe package.json view.
// The dependency maps are used only to test well-known package names; version
// strings are discarded immediately.
func parsePackageManifest(fsys fs.FS, name string) (PackageManifest, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return PackageManifest{}, err
	}
	var raw struct {
		Engines         map[string]string `json:"engines"`
		PackageManager  string            `json:"packageManager"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return PackageManifest{}, err
	}

	return PackageManifest{
		NodeEngine:     raw.Engines["node"],
		PackageManager: raw.PackageManager,
		Frameworks:     frameworks(raw.Dependencies, raw.DevDependencies),
	}, nil
}

// frameworks recognizes a small, explicit set of ecosystem markers. The
// order is intentional so facts and JSON output are deterministic.
func frameworks(deps, devDeps map[string]string) []string {
	has := func(name string) bool {
		_, inDeps := deps[name]
		_, inDevDeps := devDeps[name]
		return inDeps || inDevDeps
	}
	var out []string
	switch {
	case has("next"):
		out = append(out, "Next.js")
	case has("@vitejs/plugin-react") || has("@vitejs/plugin-vue") || has("vite"):
		out = append(out, "Vite")
	}
	if has("react") {
		out = append(out, "React")
	}
	if has("vue") {
		out = append(out, "Vue")
	}
	if has("@angular/core") {
		out = append(out, "Angular")
	}
	if has("svelte") {
		out = append(out, "Svelte")
	}
	return out
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
	managers := packageManagers(found)
	switch len(managers) {
	case 0:
		return ""
	case 1:
		return managers[0]
	default:
		return strings.Join(managers, " + ") + " (multiple lockfiles)"
	}
}

// packageManagers maps lockfile evidence to package-manager names in a stable
// order. It is shared by display facts and diagnostics so the tool never says
// one thing about a project while recommending another.
func packageManagers(present map[string]bool) []string {
	var managers []string
	if present["package-lock.json"] || present["npm-shrinkwrap.json"] {
		managers = append(managers, "npm")
	}
	if present["pnpm-lock.yaml"] {
		managers = append(managers, "pnpm")
	}
	if present["yarn.lock"] {
		managers = append(managers, "yarn")
	}
	if present["bun.lockb"] {
		managers = append(managers, "bun")
	}
	return managers
}
