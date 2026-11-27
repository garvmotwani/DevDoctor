package project

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

// mapFS builds an in-memory filesystem from a plain map. Tests declare
// what a project looks like; no disk is involved anywhere.
func mapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for p, content := range files {
		fsys[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

// nodeProject is the shared fixture: a minimal Node.js project.
var nodeProject = map[string]string{
	"fakeDir/package.json": `{ "name": "demo", "version": "1.0.0" }`,
}

func merge(base, extra map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range base {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// scan is the tiny adapter from fixture to scanFS.
func scan(t *testing.T, files map[string]string) Report {
	t.Helper()
	fsys := mapFS(files)
	// fstest.TestFS shakes the implementation of an fs.FS: Open, Stat,
	// ReadDir and globbing must agree with each other. We require every
	// path declared in the fixture to be reachable — validating MapFS
	// usage and scanFS assumptions together.
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	if err := fstest.TestFS(fsys, paths...); err != nil {
		t.Fatalf("fixture is not a valid fs.FS: %v", err)
	}
	rep, err := scanFS(fsys, "fakeDir")
	if err != nil {
		t.Fatalf("scanFS returned unexpected error: %v", err)
	}
	return rep
}

func wantLabels(t *testing.T, rep Report, present, absent []string) {
	t.Helper()
	joined := make([]string, 0, len(rep.Facts))
	for _, f := range rep.Facts {
		joined = append(joined, f.Label+" | "+f.Detail)
	}
	got := strings.Join(joined, "\n")
	for _, want := range present {
		if !strings.Contains(got, want) {
			t.Errorf("expected fact containing %q, got:\n%s", want, got)
		}
	}
	for _, no := range absent {
		if strings.Contains(got, no) {
			t.Errorf("expected NO fact containing %q, got:\n%s", no, got)
		}
	}
}

func TestScanNodeProject(t *testing.T) {
	rep := scan(t, nodeProject)
	wantLabels(t, rep,
		[]string{"Node.js project"},
		[]string{"Python project", "plain directory"},
	)
	if !rep.IsNodeProject() {
		t.Errorf("IsNodeProject() = false, want true")
	}
}

func TestScanPackageManagerFromLockfile(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"npm", merge(nodeProject, map[string]string{"fakeDir/package-lock.json": ""}), "npm"},
		{"pnpm", merge(nodeProject, map[string]string{"fakeDir/pnpm-lock.yaml": ""}), "pnpm"},
		{"yarn", merge(nodeProject, map[string]string{"fakeDir/yarn.lock": ""}), "yarn"},
		{
			"mixed lockfiles are surfaced, not hidden",
			merge(nodeProject, map[string]string{
				"fakeDir/package-lock.json": "",
				"fakeDir/pnpm-lock.yaml":    "",
			}),
			"multiple lockfiles",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := scan(t, tc.files)
			wantLabels(t, rep, []string{tc.want}, nil)
		})
	}
}

func TestScanNodeWithoutLockfileHasNoManager(t *testing.T) {
	rep := scan(t, nodeProject)
	wantLabels(t, rep, nil, []string{"package manager"})
}

func TestScanPackageManifestFacts(t *testing.T) {
	rep := scan(t, map[string]string{
		"fakeDir/package.json": `{
          "engines":{"node":">=20"},
          "packageManager":"pnpm@9.15.0",
          "dependencies":{"next":"15.0.0","react":"19.0.0"},
          "devDependencies":{"typescript":"5.0.0"}
        }`,
	})
	wantLabels(t, rep, []string{"Next.js", "React", ">=20", "pnpm@9.15.0"}, nil)
	if rep.Package == nil {
		t.Fatal("expected package manifest")
	}
	if got := strings.Join(rep.Package.Frameworks, ","); got != "Next.js,React" {
		t.Errorf("Frameworks = %q", got)
	}
}

func TestScanInvalidPackageManifestDoesNotAbort(t *testing.T) {
	rep := scan(t, map[string]string{"fakeDir/package.json": `{"name":`})
	if !rep.PackageErr || rep.Package != nil {
		t.Errorf("PackageErr/Package = %v/%+v, want true/nil", rep.PackageErr, rep.Package)
	}
}

func TestScanPythonProject(t *testing.T) {
	files := map[string]string{
		"fakeDir/requirements.txt": "requests==2.31.0\n",
	}
	rep := scan(t, files)
	wantLabels(t, rep,
		[]string{"Python project", "not detected"},
		[]string{"Node.js project"},
	)
	if !rep.IsPythonProject() {
		t.Errorf("IsPythonProject() = false, want true")
	}
}

func TestScanPythonWithVenv(t *testing.T) {
	files := map[string]string{
		"fakeDir/requirements.txt": "requests==2.31.0\n",
		"fakeDir/.venv/pyvenv.cfg": "home = /usr/bin\n",
	}
	rep := scan(t, files)
	wantLabels(t, rep,
		[]string{"virtual environment directory present"},
		[]string{"not detected"},
	)
}

func TestScanDockerAndEnvFiles(t *testing.T) {
	files := map[string]string{
		"fakeDir/Dockerfile":        "FROM node:22\n",
		"fakeDir/compose.yml":       "services: {}\n",
		"fakeDir/.env":              "SECRET_TOKEN=abc123\n", // parsed for names only
		"fakeDir/.env.example":      "SECRET_TOKEN=\n",
		"fakeDir/package.json":      "{}",
		"fakeDir/package-lock.json": "{}",
	}
	rep := scan(t, files)
	wantLabels(t, rep,
		[]string{"Docker configuration", ".env", ".env.example", "npm"},
		nil,
	)
}

// TestScanParsesEnvNamesOnly is the scanner half of the "never print a
// secret" guarantee: names and emptiness are kept, values are not.
func TestScanParsesEnvNamesOnly(t *testing.T) {
	const secret = "supersecret123"
	files := map[string]string{
		"fakeDir/.env":         "DATABASE_PASSWORD=" + secret + "\nPORT=\n",
		"fakeDir/.env.example": "DATABASE_PASSWORD=\nPORT=8000\n",
	}
	rep := scan(t, files)

	if rep.Env == nil || rep.EnvExample == nil {
		t.Fatalf("expected both env files to be parsed: %+v", rep)
	}
	if !rep.Env.Has("DATABASE_PASSWORD") {
		t.Error("expected DATABASE_PASSWORD to be recorded by name")
	}
	if got := strings.Join(rep.Env.Empty, ","); got != "PORT" {
		t.Errorf("Empty = %q, want \"PORT\"", got)
	}
	if got := strings.Join(rep.EnvExample.Keys, ","); got != "DATABASE_PASSWORD,PORT" {
		t.Errorf("example Keys = %q", got)
	}
	// The report is what later stages (and the terminal) see; no value
	// text may survive inside it.
	dump := fmt.Sprintf("%+v", rep)
	for _, value := range []string{secret, "8000"} {
		if strings.Contains(dump, value) {
			t.Fatalf("report still contains value %q: %s", value, dump)
		}
	}
}

func TestScanEnvExampleWithoutEnvFile(t *testing.T) {
	files := map[string]string{
		"fakeDir/.env.example": "API_KEY=\n",
	}
	rep := scan(t, files)
	if rep.Env != nil {
		t.Errorf(".env should be nil when absent, got %+v", rep.Env)
	}
	if rep.EnvExample == nil || !rep.EnvExample.Has("API_KEY") {
		t.Errorf("expected .env.example to be parsed, got %+v", rep.EnvExample)
	}
}

func TestScanGitRepository(t *testing.T) {
	files := map[string]string{
		"fakeDir/.git/HEAD": "ref: refs/heads/main\n",
		"fakeDir/main.go":   "package main\n",
	}
	rep := scan(t, files)
	wantLabels(t, rep, []string{"Git repository"}, nil)
}

func TestScanGitignoreIsNotAGitRepository(t *testing.T) {
	files := map[string]string{
		"fakeDir/.gitignore": "node_modules/\n",
	}
	rep := scan(t, files)
	wantLabels(t, rep,
		[]string{"plain directory"},
		[]string{"Git repository"},
	)
}

func TestScanEmptyDirectory(t *testing.T) {
	files := map[string]string{
		"fakeDir/notes.txt": "not a project\n",
	}
	rep := scan(t, files)
	wantLabels(t, rep,
		[]string{"plain directory"},
		[]string{"Node.js project", "Python project", "Git repository"},
	)
}

func TestScanRejectsMissingRoot(t *testing.T) {
	// A root that does not exist at all is an error — distinct from a
	// root that exists but contains no recognizable project files.
	fsys := mapFS(map[string]string{"file.txt": "x"})
	_, err := scanFS(fsys, "nowhere")
	if err == nil {
		t.Fatal("expected an error for a missing root, got nil")
	}
}

func TestScanEmptyRootIsFSRoot(t *testing.T) {
	// root "" means: the project is the FS root itself. This is how
	// ScanPath wires the real disk (os.DirFS), so it must work here too.
	rep, err := scanFS(mapFS(map[string]string{
		"package.json":      "{}",
		"package-lock.json": "{}",
	}), "")
	if err != nil {
		t.Fatalf("scanFS with empty root returned error: %v", err)
	}
	wantLabels(t, rep, []string{"Node.js project", "npm"}, nil)
}
